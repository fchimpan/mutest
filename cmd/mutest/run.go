// Package mutest implements the mutest CLI entry point.
package mutest

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/fchimpan/mutest/config"
	"github.com/fchimpan/mutest/diff"
	"github.com/fchimpan/mutest/engine"
	"github.com/fchimpan/mutest/mutator"
	"github.com/fchimpan/mutest/output"
	"github.com/fchimpan/mutest/runner"
)

var (
	ErrTestsFailed     = errors.New("mutation tests failed")
	ErrInvalidConfig   = errors.New("invalid config")
	ErrDiscovery       = errors.New("error discovering mutations")
	ErrDiff            = errors.New("diff parse error")
	ErrInstrumentation = errors.New("instrumentation error")
	ErrBuild           = errors.New("build error")
	ErrBaseline        = errors.New("baseline test run failed (tests fail without mutations)")
	ErrInterrupted     = errors.New("interrupted")
)

// Set via ldflags at build time; resolveVersion falls back to debug.ReadBuildInfo for go install.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func resolveVersion() (v, c, d string) {
	v, c, d = version, commit, date
	if v != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) > 7 {
				c = s.Value[:7]
			} else {
				c = s.Value
			}
		case "vcs.time":
			d = s.Value
		}
	}
	return
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("mutest", flag.ContinueOnError)
	fs.SetOutput(stderr)

	showVersion := fs.Bool("version", false, "print version and exit")
	workers := fs.Int("workers", runtime.NumCPU(), "max parallel test processes")
	timeout := fs.Duration("timeout", 30*time.Second, "per-mutant test timeout")
	verbose := fs.Bool("v", false, "show test output for each mutant")
	runFlag := fs.String("run", "", "regexp to pass to go test -run")
	jsonOutput := fs.Bool("json", false, "emit results as JSON")
	dryRun := fs.Bool("dry-run", false, "discover mutations without running tests")
	threshold := fs.Float64("threshold", 0, "minimum kill rate percentage (0-100); exit 1 if below (0 = any survived mutant fails)")
	skipErrPropagation := fs.Bool("skip-err-propagation", true, "skip simple error propagation patterns (if err != nil { return err })")
	diffBase := fs.String("diff", "", "only mutate lines changed relative to this git ref (e.g., origin/main)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if *showVersion {
		v, c, d := resolveVersion()
		if c != "none" {
			fmt.Fprintf(stdout, "mutest %s (commit: %s, built: %s)\n", v, c, d)
		} else {
			fmt.Fprintf(stdout, "mutest %s\n", v)
		}
		return nil
	}

	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	cfg := config.Config{
		Patterns:           patterns,
		Workers:            *workers,
		Timeout:            *timeout,
		Verbose:            *verbose,
		Run:                *runFlag,
		JSON:               *jsonOutput,
		DryRun:             *dryRun,
		Threshold:          *threshold,
		SkipErrPropagation: *skipErrPropagation,
		Diff:               *diffBase,
	}

	return run(ctx, cfg, stdout, stderr)
}

// baselineErr classifies a VerifyBaseline failure. A failure observed after
// the run context was canceled (e.g. SIGINT while the baseline was running)
// is an interruption, not a broken test suite, and must not be reported as
// ErrBaseline.
func baselineErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ErrInterrupted
	}
	return fmt.Errorf("%w: %w", ErrBaseline, err)
}

func run(ctx context.Context, cfg config.Config, stdout, stderr io.Writer) error {
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}

	eng := engine.New(cfg.Patterns, &mutator.ComparisonMutator{}, &mutator.EqualityMutator{
		SkipErrPropagation: cfg.SkipErrPropagation,
	})

	points, err := eng.DiscoverAll()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDiscovery, err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}
	rep := output.NewReporter(cfg, stdout, stderr, cwd)

	if cfg.Diff != "" { //mutest:skip
		cl, err := diff.ParseGitDiff(cfg.Diff)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrDiff, err)
		}
		before := len(points)
		points = diff.FilterPoints(points, cl)
		fmt.Fprintf(rep.Info(), "mutest: diff mode: filtered to %d of %d mutation points (changed vs %s)\n", len(points), before, cfg.Diff)
	}

	if cfg.DryRun {
		rep.DryRun(points)
		return nil
	}

	if len(points) == 0 {
		rep.NoMutationPoints()
		return nil
	}

	fmt.Fprintf(rep.Info(), "mutest: discovered %d mutation points\n", len(points))
	fmt.Fprintf(rep.Info(), "mutest: instrumenting packages...\n")

	pkgs, err := eng.InstrumentAll(points)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInstrumentation, err)
	}
	// Ensure temp dirs are cleaned up even on SIGINT/SIGTERM.
	// defer alone is insufficient: os.Exit bypasses defers, and
	// a killed process never runs them at all.
	cleanupDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		engine.CleanupInstrumented(pkgs)
		close(cleanupDone)
	}()
	defer func() {
		engine.CleanupInstrumented(pkgs)
		select {
		case <-cleanupDone:
		default:
		}
	}()

	fmt.Fprintf(rep.Info(), "mutest: building test binaries...\n")
	if err := eng.BuildTestBinaries(ctx, pkgs); err != nil {
		return fmt.Errorf("%w: %w", ErrBuild, err)
	}

	// Packages with no test files (F3) build no binary; all their mutants
	// survive. Surface this so the survived count is not mysterious.
	for _, pkg := range pkgs {
		if pkg.NoTests {
			fmt.Fprintf(rep.Info(), "mutest: package %s has no test files (%d mutants will survive)\n", pkg.ImportPath, len(pkg.Mutations))
		}
	}

	runCfg := runner.Config{
		Workers: cfg.Workers,
		Timeout: cfg.Timeout,
		Run:     cfg.Run,
	}

	// Baseline (F4): tests must pass with no mutation active, otherwise every
	// mutant would be a false KILLED. Abort before running any mutant.
	fmt.Fprintf(rep.Info(), "mutest: verifying baseline (tests must pass without mutations)...\n")
	if err := runner.VerifyBaseline(ctx, pkgs, runCfg); err != nil {
		return baselineErr(ctx, err)
	}

	fmt.Fprintf(rep.Info(), "mutest: testing with %d workers, %s timeout per mutant\n\n", cfg.Workers, cfg.Timeout)

	summary := runner.RunInstrumented(ctx, pkgs, runCfg, rep.ProgressFunc())

	rep.Summary(summary)

	// Interruption (F5): if the run was canceled (e.g. SIGINT), do not judge
	// success on partial results — that would fake a passing CI job.
	if ctx.Err() != nil {
		fmt.Fprintf(rep.Info(), "mutest: interrupted; %d mutants were not tested\n", summary.Canceled)
		return ErrInterrupted
	}

	if cfg.Threshold > 0 {
		// Errors mean mutants could not be tested; never silently pass them.
		if summary.Errors > 0 {
			return ErrTestsFailed
		}
		// Compare against the same rounded value shown to the user (F11): a
		// raw-rate comparison could fail "-threshold 80" on a run that
		// displays "Score: 80.0%" whenever the unrounded rate was e.g. 79.96.
		if output.RoundedKillRate(summary) < cfg.Threshold {
			return ErrTestsFailed
		}
		return nil
	}
	if summary.Survived > 0 || summary.Errors > 0 {
		return ErrTestsFailed
	}
	return nil
}
