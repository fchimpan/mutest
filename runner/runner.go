package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/fchimpan/mutest/engine"
	"github.com/fchimpan/mutest/mutator"
)

// Common go test flags shared between warmup and mutation runs to ensure
// build cache consistency.
var goTestFlags = []string{"-vet=off", "-p=1", "-ldflags=-s -w"}

// Result captures the outcome of testing one mutant.
type Result struct {
	Point    mutator.MutationPoint
	Killed   bool
	Duration time.Duration
	Output   string
	TimedOut bool
	Err      error
}

// Summary aggregates all results.
type Summary struct {
	Total    int
	Killed   int
	Survived int
	Errors   int
	Duration time.Duration
	Results  []Result
}

// Config controls runner behavior.
type Config struct {
	Workers  int
	Timeout  time.Duration
	Patterns []string // package patterns passed to go test (e.g. "./...", "./pkg/calc")
	Run      string   // -run regex passed to go test
}

// ProgressFunc is called after each mutant is tested. It may be nil.
type ProgressFunc func(result Result, done, total int)

// WarmBuildCache compiles test binaries for the packages that will be mutated
// without actually running them. This populates Go's build cache so that each
// mutant only needs to recompile the single changed file.
func WarmBuildCache(ctx context.Context, points []mutator.MutationPoint) {
	pkgs := make(map[string]struct{})
	for _, pt := range points {
		if pt.ImportPath != "" {
			pkgs[pt.ImportPath] = struct{}{}
		}
	}

	var wg sync.WaitGroup
	for pkg := range pkgs {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			args := append([]string{"test", "-c"}, goTestFlags...)
			args = append(args, "-o", os.DevNull, p)
			cmd := exec.CommandContext(ctx, "go", args...)
			_ = cmd.Run()
		}(pkg)
	}
	wg.Wait()
}

// Run tests all mutants with bounded parallelism and returns a Summary.
func Run(ctx context.Context, eng *engine.Engine, points []mutator.MutationPoint, cfg Config, progress ProgressFunc) *Summary {
	start := time.Now()
	results := make([]Result, len(points))
	sem := make(chan struct{}, cfg.Workers)
	var mu sync.Mutex
	done := 0

	var wg sync.WaitGroup
	for i, point := range points {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, pt mutator.MutationPoint) {
			defer wg.Done()
			defer func() { <-sem }()

			r := testMutant(ctx, eng, pt, cfg)
			results[idx] = r

			if progress != nil {
				mu.Lock()
				done++
				d := done
				progress(r, d, len(points))
				mu.Unlock()
			}
		}(i, point)
	}
	wg.Wait()

	summary := &Summary{
		Total:    len(points),
		Duration: time.Since(start),
		Results:  results,
	}
	for _, r := range results {
		switch {
		case r.Err != nil:
			summary.Errors++
		case r.Killed:
			summary.Killed++
		default:
			summary.Survived++
		}
	}
	return summary
}

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
	sem := make(chan struct{}, cfg.Workers)
	var mu sync.Mutex
	done := 0

	var wg sync.WaitGroup
	for i, point := range allPoints {
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
		case r.Err != nil:
			summary.Errors++
		case r.Killed:
			summary.Killed++
		default:
			summary.Survived++
		}
	}
	return summary
}

func testMutantRuntime(ctx context.Context, pkg *engine.InstrumentedPackage, pt mutator.MutationPoint, cfg Config) Result {
	start := time.Now()

	testCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	args := []string{"-test.failfast", "-test.count=1", fmt.Sprintf("-test.timeout=%s", cfg.Timeout)}
	if cfg.Run != "" {
		args = append(args, "-test.run", cfg.Run)
	}

	cmd := exec.CommandContext(testCtx, pkg.BinaryPath, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("MUTEST_ID=%d", pt.MutestID))
	output, err := cmd.CombinedOutput()

	timedOut := testCtx.Err() == context.DeadlineExceeded
	killed := false
	if err != nil || timedOut {
		killed = true
	}

	return Result{
		Point:    pt,
		Killed:   killed,
		Duration: time.Since(start),
		Output:   string(output),
		TimedOut: timedOut,
	}
}

func testMutant(ctx context.Context, eng *engine.Engine, pt mutator.MutationPoint, cfg Config) Result {
	start := time.Now()

	m, err := eng.Prepare(pt)
	if err != nil {
		return Result{Point: pt, Err: err, Duration: time.Since(start)}
	}
	defer eng.Cleanup(m)

	testCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	args := append([]string{"test", "-overlay=" + m.OverlayPath, "-count=1", "-failfast"}, goTestFlags...)
	args = append(args, fmt.Sprintf("-timeout=%s", cfg.Timeout))
	if cfg.Run != "" {
		args = append(args, "-run", cfg.Run)
	}
	// Test only the package containing the mutated file instead of all patterns.
	if pt.ImportPath != "" {
		args = append(args, pt.ImportPath)
	} else {
		args = append(args, cfg.Patterns...)
	}

	cmd := exec.CommandContext(testCtx, "go", args...)
	output, err := cmd.CombinedOutput()

	timedOut := testCtx.Err() == context.DeadlineExceeded
	killed := false
	if err != nil || timedOut {
		killed = true
	}

	return Result{
		Point:    pt,
		Killed:   killed,
		Duration: time.Since(start),
		Output:   string(output),
		TimedOut: timedOut,
	}
}
