package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/fchimpan/mutest/engine"
	"github.com/fchimpan/mutest/mutator"
	"github.com/fchimpan/mutest/runner"
)

// Set via ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type config struct {
	Dir     string
	Workers int
	Timeout time.Duration
	Verbose bool
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	dir := flag.String("dir", ".", "root directory of the Go project to mutate")
	workers := flag.Int("workers", runtime.NumCPU(), "max parallel test processes")
	timeout := flag.Duration("timeout", 30*time.Second, "per-mutant test timeout")
	verbose := flag.Bool("v", false, "print details for each mutant")
	flag.Parse()

	if *showVersion {
		fmt.Printf("mutest %s (commit: %s, built: %s)\n", version, commit, date)
		return
	}

	cfg := config{
		Dir:     *dir,
		Workers: *workers,
		Timeout: *timeout,
		Verbose: *verbose,
	}

	code := run(cfg, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

// run executes the mutation testing pipeline, returning an exit code.
func run(cfg config, stdout, stderr io.Writer) int {
	compMut := &mutator.ComparisonMutator{}
	eng := engine.New(cfg.Dir, compMut)

	points, err := eng.DiscoverAll()
	if err != nil {
		fmt.Fprintf(stderr, "mutest: error discovering mutations: %v\n", err)
		return 2
	}

	if len(points) == 0 {
		fmt.Fprintln(stdout, "mutest: no mutation points found")
		return 0
	}

	fmt.Fprintf(stdout, "mutest: discovered %d mutation points\n", len(points))
	fmt.Fprintf(stdout, "mutest: testing with %d workers, %s timeout per mutant\n\n", cfg.Workers, cfg.Timeout)

	runCfg := runner.Config{
		Workers: cfg.Workers,
		Timeout: cfg.Timeout,
		BaseDir: cfg.Dir,
	}

	absDir, _ := filepath.Abs(cfg.Dir)

	var progress runner.ProgressFunc
	if cfg.Verbose {
		progress = func(r runner.Result, done, total int) {
			status := "SURVIVED"
			if r.Err != nil {
				status = "ERROR"
			} else if r.Killed {
				status = "KILLED"
			}
			fmt.Fprintf(stdout, "[%-8s] %s:%d:%d  %s  (%s)\n",
				status, relPath(absDir, r.Point.File), r.Point.Line, r.Point.Column, r.Point.Desc, r.Duration.Round(time.Millisecond))
		}
	}

	summary := runner.Run(context.Background(), eng, compMut, points, runCfg, progress)

	printReport(stdout, summary, absDir)

	if summary.Survived > 0 {
		return 1
	}
	return 0
}

func relPath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

func printReport(w io.Writer, s *runner.Summary, baseDir string) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "===== Mutation Testing Summary =====")
	fmt.Fprintf(w, "Total:     %d\n", s.Total)

	killRate := 0.0
	if s.Total-s.Errors > 0 {
		killRate = float64(s.Killed) / float64(s.Total-s.Errors) * 100
	}

	fmt.Fprintf(w, "Killed:    %d (%.1f%%)\n", s.Killed, killRate)
	fmt.Fprintf(w, "Survived:  %d\n", s.Survived)
	if s.Errors > 0 {
		fmt.Fprintf(w, "Errors:    %d\n", s.Errors)
	}
	fmt.Fprintf(w, "Duration:  %s\n", s.Duration.Round(time.Millisecond))

	var survived []runner.Result
	for _, r := range s.Results {
		if r.Err == nil && !r.Killed {
			survived = append(survived, r)
		}
	}

	if len(survived) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Survived mutants (test gaps):")
		for i, r := range survived {
			fmt.Fprintf(w, "  %d. %s:%d:%d  %s\n", i+1, relPath(baseDir, r.Point.File), r.Point.Line, r.Point.Column, r.Point.Desc)
		}
	}
}
