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
	"strings"
	"time"

	"github.com/fchimpan/mutest/diff"
	"github.com/fchimpan/mutest/engine"
	"github.com/fchimpan/mutest/mutator"
	"github.com/fchimpan/mutest/runner"
)

// Set via ldflags at build time; fallback to debug.ReadBuildInfo for go install.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func init() {
	if version != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) > 7 {
				commit = s.Value[:7]
			} else {
				commit = s.Value
			}
		case "vcs.time":
			date = s.Value
		}
	}
}

type config struct {
	Patterns           []string
	Workers            int
	Timeout            time.Duration
	Verbose            bool
	Run                string
	JSON               bool
	DryRun             bool
	Threshold          float64
	SkipErrPropagation bool
	Diff               string
}

// Run parses CLI arguments and executes the mutation testing pipeline.
// It returns an exit code suitable for os.Exit.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
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
			return 0
		}
		return 2
	}

	if *showVersion {
		if commit != "none" {
			fmt.Fprintf(stdout, "mutest %s (commit: %s, built: %s)\n", version, commit, date)
		} else {
			fmt.Fprintf(stdout, "mutest %s\n", version)
		}
		return 0
	}

	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	cfg := config{
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

func validateConfig(cfg config) error {
	if cfg.Workers <= 0 {
		return fmt.Errorf("-workers must be > 0, got %d", cfg.Workers)
	}
	if cfg.Timeout <= 0 {
		return fmt.Errorf("-timeout must be > 0, got %s", cfg.Timeout)
	}
	if cfg.Threshold < 0 || cfg.Threshold > 100 {
		return fmt.Errorf("-threshold must be between 0 and 100, got %.1f", cfg.Threshold)
	}
	return nil
}

func run(ctx context.Context, cfg config, stdout, stderr io.Writer) int {
	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(stderr, "mutest: %v\n", err)
		return 2
	}

	eng := engine.New(cfg.Patterns, &mutator.ComparisonMutator{}, &mutator.EqualityMutator{
		SkipErrPropagation: cfg.SkipErrPropagation,
	})

	points, err := eng.DiscoverAll()
	if err != nil {
		fmt.Fprintf(stderr, "mutest: error discovering mutations: %v\n", err)
		return 2
	}

	// Informational messages go to stderr in JSON mode to keep stdout machine-readable.
	info := stdout
	if cfg.JSON {
		info = stderr
	}

	if cfg.Diff != "" { //mutest:skip
		cl, err := diff.ParseGitDiff(cfg.Diff)
		if err != nil {
			fmt.Fprintf(stderr, "mutest: %v\n", err)
			return 2
		}
		before := len(points)
		points = diff.FilterPoints(points, cl)
		fmt.Fprintf(info, "mutest: diff mode: filtered to %d of %d mutation points (changed vs %s)\n", len(points), before, cfg.Diff)
	}

	cwd, _ := os.Getwd()
	rpc := newRelPathCache(cwd)

	if len(points) == 0 {
		if cfg.JSON {
			if cfg.DryRun {
				fmt.Fprintln(stdout, "[]")
			} else {
				writeJSONSummary(stdout, &runner.Summary{}, rpc, true)
			}
		} else {
			fmt.Fprintln(stdout, "mutest: no mutation points found")
		}
		return 0
	}

	// dry-run: list discovered mutations and exit
	if cfg.DryRun {
		return runDryRun(cfg, stdout, points, rpc)
	}

	fmt.Fprintf(info, "mutest: discovered %d mutation points\n", len(points))

	// Instrument all packages and build test binaries.
	fmt.Fprintf(info, "mutest: instrumenting packages...\n")
	pkgs, err := eng.InstrumentAll(points)
	if err != nil {
		fmt.Fprintf(stderr, "mutest: instrumentation error: %v\n", err)
		return 2
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

	fmt.Fprintf(info, "mutest: building test binaries...\n")
	if err := eng.BuildTestBinaries(ctx, pkgs); err != nil {
		fmt.Fprintf(stderr, "mutest: build error: %v\n", err)
		return 2
	}

	fmt.Fprintf(info, "mutest: testing with %d workers, %s timeout per mutant\n\n", cfg.Workers, cfg.Timeout)

	runCfg := runner.Config{
		Workers: cfg.Workers,
		Timeout: cfg.Timeout,
		Run:     cfg.Run,
	}

	var progress runner.ProgressFunc
	if cfg.JSON {
		if cfg.Verbose {
			enc := newJSONEncoder(stdout)
			progress = func(r runner.Result, done, total int) {
				enc.Encode(toJSONResult(r, rpc))
			}
		}
	} else {
		progress = func(r runner.Result, done, total int) {
			status := "KILLED"
			if r.Err != nil {
				status = "ERROR"
			} else if r.TimedOut {
				status = "TIMEOUT"
			} else if !r.Killed {
				status = "SURVIVED"
			}
			fmt.Fprintf(stdout, "--- %s: %s:%d:%d  %s (%.2fs)\n",
				status, rpc.get(r.Point.File), r.Point.Line, r.Point.Column, r.Point.Desc, r.Duration.Seconds())
			if cfg.Verbose && r.Output != "" {
				for line := range strings.SplitSeq(strings.TrimRight(r.Output, "\n"), "\n") {
					fmt.Fprintf(stdout, "        %s\n", line)
				}
			}
		}
	}

	summary := runner.RunInstrumented(ctx, pkgs, runCfg, progress)

	if cfg.JSON {
		// When verbose, results were already streamed as NDJSON;
		// emit summary without duplicating them.
		writeJSONSummary(stdout, summary, rpc, !cfg.Verbose)
	} else {
		printReport(stdout, summary, rpc)
	}

	if cfg.Threshold > 0 {
		killRate := calcKillRate(summary)
		if killRate < cfg.Threshold {
			return 1
		}
		return 0
	}
	if summary.Survived > 0 {
		return 1
	}
	return 0
}
