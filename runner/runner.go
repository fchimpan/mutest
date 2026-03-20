package runner

import (
	"context"
	"os/exec"
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
	BaseDir  string
	TestArgs []string
}

// ProgressFunc is called after each mutant is tested. It may be nil.
type ProgressFunc func(result Result, done, total int)

// Run tests all mutants with bounded parallelism and returns a Summary.
func Run(ctx context.Context, eng *engine.Engine, mut mutator.Mutator, points []mutator.MutationPoint, cfg Config, progress ProgressFunc) *Summary {
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

			r := testMutant(ctx, eng, mut, pt, cfg)
			results[idx] = r

			if progress != nil {
				mu.Lock()
				done++
				d := done
				mu.Unlock()
				progress(r, d, len(points))
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

func testMutant(ctx context.Context, eng *engine.Engine, mut mutator.Mutator, pt mutator.MutationPoint, cfg Config) Result {
	start := time.Now()

	m, err := eng.Prepare(mut, pt)
	if err != nil {
		return Result{Point: pt, Err: err, Duration: time.Since(start)}
	}
	defer eng.Cleanup(m)

	testCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	args := []string{"test", "-overlay=" + m.OverlayPath}
	args = append(args, cfg.TestArgs...)
	args = append(args, "./...")

	cmd := exec.CommandContext(testCtx, "go", args...)
	cmd.Dir = cfg.BaseDir
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
