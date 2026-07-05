package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/fchimpan/mutest/engine"
	"github.com/fchimpan/mutest/mutator"
)

// Result captures the outcome of testing one mutant.
type Result struct {
	Point    mutator.MutationPoint
	Killed   bool
	Duration time.Duration
	Output   string
	TimedOut bool
	Canceled bool
	Err      error
}

// Summary aggregates all results.
type Summary struct {
	Total    int
	Killed   int
	TimedOut int
	Survived int
	Errors   int
	Canceled int
	Duration time.Duration
	Results  []Result
}

// Config controls runner behavior.
type Config struct {
	Workers int
	Timeout time.Duration
	Run     string // -run regex passed to go test
}

// ProgressFunc is called after each mutant is tested. It may be nil.
type ProgressFunc func(result Result, done, total int)

// RunInstrumented tests all mutants using pre-built test binaries.
// Each mutation is activated via MUTEST_ID env var, avoiding per-mutation compilation.
func RunInstrumented(ctx context.Context, pkgs map[string]*engine.InstrumentedPackage, cfg Config, progress ProgressFunc) *Summary {
	// Flatten all mutations across packages.
	var allPoints []mutator.MutationPoint
	pointToPkg := make(map[int]*engine.InstrumentedPackage) // index → pkg
	for _, pkg := range pkgs {
		for _, pt := range pkg.Mutations {
			pointToPkg[len(allPoints)] = pkg
			allPoints = append(allPoints, pt)
		}
	}

	start := time.Now()
	results := make([]Result, len(allPoints))
	// Prefill every result as Canceled so that any mutant we never launch
	// (e.g. after ctx cancellation breaks the loop below) is counted as
	// canceled rather than as a false Survived (the zero value).
	for i := range results {
		results[i] = Result{Point: allPoints[i], Canceled: true}
	}
	sem := make(chan struct{}, cfg.Workers)
	var mu sync.Mutex
	done := 0

	var wg sync.WaitGroup
	for i, point := range allPoints {
		if ctx.Err() != nil {
			break // stop launching new mutants once the run is canceled
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, pt mutator.MutationPoint) {
			defer wg.Done()
			defer func() { <-sem }()

			pkg := pointToPkg[idx]
			r := testMutantRuntime(ctx, pkg, pt, cfg)
			results[idx] = r

			if progress != nil {
				mu.Lock()
				done++
				d := done
				progress(r, d, len(allPoints))
				mu.Unlock()
			}
		}(i, point)
	}
	wg.Wait()

	summary := &Summary{
		Total:    len(allPoints),
		Duration: time.Since(start),
		Results:  results,
	}
	for _, r := range results {
		switch {
		case r.Canceled:
			summary.Canceled++
		case r.Err != nil:
			summary.Errors++
		case r.TimedOut:
			summary.TimedOut++
		case r.Killed:
			summary.Killed++
		default:
			summary.Survived++
		}
	}
	return summary
}

// testArgs returns the go test flags shared by mutant and baseline runs.
func testArgs(cfg Config) []string {
	args := []string{"-test.failfast", "-test.count=1", fmt.Sprintf("-test.timeout=%s", cfg.Timeout)}
	if cfg.Run != "" {
		args = append(args, "-test.run", cfg.Run)
	}
	return args
}

// VerifyBaseline runs each package's test binary once WITHOUT any mutation
// activated and returns an error if any package's tests fail. This guards
// against the "all mutants killed because the tests were already broken"
// false 100% score. Packages without test files (NoTests) are skipped.
// Parallelism is bounded by cfg.Workers.
func VerifyBaseline(ctx context.Context, pkgs map[string]*engine.InstrumentedPackage, cfg Config) error {
	sem := make(chan struct{}, cfg.Workers)
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup

	for _, pkg := range pkgs {
		if pkg.NoTests {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(p *engine.InstrumentedPackage) {
			defer wg.Done()
			defer func() { <-sem }()

			testCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
			defer cancel()

			cmd := exec.CommandContext(testCtx, p.BinaryPath, testArgs(cfg)...)
			cmd.Dir = p.Dir
			// Neutralize any MUTEST_ID inherited from the caller's environment:
			// ID 0 activates no mutation (helper IDs are 1-based), and os/exec
			// keeps the last duplicate key, so this append always wins.
			cmd.Env = append(os.Environ(), "MUTEST_ID=0")
			output, err := cmd.CombinedOutput()
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("package %s: baseline tests failed: %w\n%s", p.ImportPath, err, trimBaselineOutput(output))
				}
				mu.Unlock()
			}
		}(pkg)
	}
	wg.Wait()
	return firstErr
}

// trimBaselineOutput trims a test binary's combined output to a readable tail
// so a huge failing log does not swamp the error message.
func trimBaselineOutput(out []byte) string {
	const max = 2000
	s := strings.TrimRight(string(out), "\n")
	if len(s) > max {
		s = "...(truncated)...\n" + s[len(s)-max:]
	}
	return s
}

func testMutantRuntime(ctx context.Context, pkg *engine.InstrumentedPackage, pt mutator.MutationPoint, cfg Config) Result {
	// A package with no test files has no binary to run; its mutants survive
	// (no tests is the largest possible test gap).
	if pkg.NoTests {
		return Result{Point: pt, Output: "package has no test files"}
	}

	start := time.Now()

	testCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(testCtx, pkg.BinaryPath, testArgs(cfg)...)
	cmd.Dir = pkg.Dir
	cmd.Env = append(os.Environ(), fmt.Sprintf("MUTEST_ID=%d", pt.MutestID))
	output, err := cmd.CombinedOutput()

	// Order matters: a canceled run and a timed-out run both kill the process
	// (yielding an *exec.ExitError), so they must be classified before the
	// ExitError case, otherwise they would be misreported as KILLED.
	canceled := ctx.Err() != nil // parent (e.g. SIGINT) cancellation, not the per-mutant timeout
	timedOut := !canceled && errors.Is(testCtx.Err(), context.DeadlineExceeded)

	r := Result{
		Point:    pt,
		Duration: time.Since(start),
		Output:   string(output),
		TimedOut: timedOut,
		Canceled: canceled,
	}
	var exitErr *exec.ExitError
	switch {
	case canceled:
		// Canceled: not a real result; leave Killed=false.
	case timedOut:
		// Timed out: counted as detected upstream, but not KILLED here.
	case err == nil:
		// Tests passed with the mutation active: mutant survived.
	case errors.As(err, &exitErr):
		r.Killed = true // tests ran and failed: genuine kill
	default:
		r.Err = err // fork failure, missing binary, etc.: ERROR, not KILLED
	}
	return r
}
