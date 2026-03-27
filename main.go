package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
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

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	workers := flag.Int("workers", runtime.NumCPU(), "max parallel test processes")
	timeout := flag.Duration("timeout", 30*time.Second, "per-mutant test timeout")
	verbose := flag.Bool("v", false, "show test output for each mutant")
	run := flag.String("run", "", "regexp to pass to go test -run")
	jsonOutput := flag.Bool("json", false, "emit results as JSON")
	dryRun := flag.Bool("dry-run", false, "discover mutations without running tests")
	threshold := flag.Float64("threshold", 0, "minimum kill rate percentage (0-100); exit 1 if below (0 = any survived mutant fails)")
	skipErrPropagation := flag.Bool("skip-err-propagation", true, "skip simple error propagation patterns (if err != nil { return err })")
	diffBase := flag.String("diff", "", "only mutate lines changed relative to this git ref (e.g., origin/main)")
	flag.Parse()

	if *showVersion {
		if commit != "none" {
			fmt.Printf("mutest %s (commit: %s, built: %s)\n", version, commit, date)
		} else {
			fmt.Printf("mutest %s\n", version)
		}
		return
	}

	patterns := flag.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	cfg := config{
		Patterns:           patterns,
		Workers:            *workers,
		Timeout:            *timeout,
		Verbose:            *verbose,
		Run:                *run,
		JSON:               *jsonOutput,
		DryRun:             *dryRun,
		Threshold:          *threshold,
		SkipErrPropagation: *skipErrPropagation,
		Diff:               *diffBase,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	code := run2(ctx, cfg, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
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

// run2 executes the mutation testing pipeline, returning an exit code.
func run2(ctx context.Context, cfg config, stdout, stderr io.Writer) int {
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
		if err != nil { //mutest:skip
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
			fmt.Fprintf(stdout, "--- %s: %s:%d:%d  %s (%s)\n",
				status, rpc.get(r.Point.File), r.Point.Line, r.Point.Column, r.Point.Desc, fmtSeconds(r.Duration))
			if cfg.Verbose && r.Output != "" {
				for _, line := range strings.Split(strings.TrimRight(r.Output, "\n"), "\n") {
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

func runDryRun(cfg config, stdout io.Writer, points []mutator.MutationPoint, rpc *relPathCache) int {
	if cfg.JSON {
		pts := make([]jsonMutationPoint, len(points))
		for i, p := range points {
			pts[i] = jsonMutationPoint{
				File:     rpc.get(p.File),
				Package:  p.Package,
				Line:     p.Line,
				Column:   p.Column,
				Original: p.Original.String(),
				Mutated:  p.Mutated.String(),
				Desc:     p.Desc,
			}
		}
		enc := newJSONEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(pts)
	} else {
		fmt.Fprintf(stdout, "mutest: discovered %d mutation points (dry run)\n\n", len(points))
		for i, p := range points {
			fmt.Fprintf(stdout, "  %d. %s:%d:%d  %s\n", i+1, rpc.get(p.File), p.Line, p.Column, p.Desc)
		}
	}
	return 0
}

// relPathCache avoids repeated filepath.Rel calls for the same absolute path.
// Mutation testing typically targets a handful of files with many mutation points each,
// so caching the relative path per file avoids redundant work.
type relPathCache struct {
	base  string
	cache map[string]string
}

func newRelPathCache(base string) *relPathCache {
	return &relPathCache{base: base, cache: make(map[string]string)}
}

func (c *relPathCache) get(path string) string {
	if rel, ok := c.cache[path]; ok {
		return rel
	}
	rel, err := filepath.Rel(c.base, path)
	if err != nil {
		rel = path
	}
	c.cache[path] = rel
	return rel
}

// --- Text report ---

func printReport(w io.Writer, s *runner.Summary, rpc *relPathCache) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "===== Mutation Testing Summary =====")
	fmt.Fprintf(w, "Total:     %d\n", s.Total)
	fmt.Fprintf(w, "Killed:    %d\n", s.Killed)
	if s.TimedOut > 0 {
		fmt.Fprintf(w, "Timeout:   %d\n", s.TimedOut)
	}
	fmt.Fprintf(w, "Survived:  %d\n", s.Survived)
	if s.Errors > 0 {
		fmt.Fprintf(w, "Errors:    %d\n", s.Errors)
	}
	fmt.Fprintf(w, "Score:     %.1f%%\n", calcKillRate(s))
	fmt.Fprintf(w, "Duration:  %s\n", s.Duration.Round(time.Millisecond))

	var survived []runner.Result
	for _, r := range s.Results {
		if r.Err == nil && !r.Killed && !r.TimedOut {
			survived = append(survived, r)
		}
	}

	if len(survived) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Survived mutants (test gaps):")
		for i, r := range survived {
			fmt.Fprintf(w, "  %d. %s:%d:%d  %s\n", i+1, rpc.get(r.Point.File), r.Point.Line, r.Point.Column, r.Point.Desc)
		}
	}
}

// --- JSON types and helpers ---

type jsonMutationPoint struct {
	File     string `json:"file"`
	Package  string `json:"package"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Original string `json:"original"`
	Mutated  string `json:"mutated"`
	Desc     string `json:"desc"`
}

type jsonResult struct {
	Status   string `json:"status"`
	File     string `json:"file"`
	Package  string `json:"package"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Original string `json:"original"`
	Mutated  string `json:"mutated"`
	Desc     string `json:"desc"`
	Duration string `json:"duration"`
	TimedOut bool   `json:"timed_out,omitempty"`
	Error    string `json:"error,omitempty"`
}

type jsonSummary struct {
	Total    int          `json:"total"`
	Killed   int          `json:"killed"`
	TimedOut int          `json:"timed_out"`
	Survived int          `json:"survived"`
	Errors   int          `json:"errors"`
	KillRate float64      `json:"kill_rate"`
	Duration string       `json:"duration"`
	Results  []jsonResult `json:"results"`
}

func toJSONResult(r runner.Result, rpc *relPathCache) jsonResult {
	status := "survived"
	if r.Err != nil {
		status = "error"
	} else if r.TimedOut {
		status = "timeout"
	} else if r.Killed {
		status = "killed"
	}
	jr := jsonResult{
		Status:   status,
		File:     rpc.get(r.Point.File),
		Package:  r.Point.Package,
		Line:     r.Point.Line,
		Column:   r.Point.Column,
		Original: r.Point.Original.String(),
		Mutated:  r.Point.Mutated.String(),
		Desc:     r.Point.Desc,
		Duration: r.Duration.Round(time.Millisecond).String(),
		TimedOut: r.TimedOut,
	}
	if r.Err != nil {
		jr.Error = r.Err.Error()
	}
	return jr
}

func writeJSONSummary(w io.Writer, s *runner.Summary, rpc *relPathCache, includeResults bool) {
	killRate := calcKillRate(s)

	var results []jsonResult
	if includeResults {
		results = make([]jsonResult, len(s.Results))
		for i, r := range s.Results {
			results[i] = toJSONResult(r, rpc)
		}
	}

	summary := jsonSummary{
		Total:    s.Total,
		Killed:   s.Killed,
		TimedOut: s.TimedOut,
		Survived: s.Survived,
		Errors:   s.Errors,
		KillRate: math.Round(killRate*10) / 10,
		Duration: s.Duration.Round(time.Millisecond).String(),
		Results:  results,
	}
	enc := newJSONEncoder(w)
	enc.Encode(summary)
}

func newJSONEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc
}

func fmtSeconds(d time.Duration) string {
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func calcKillRate(s *runner.Summary) float64 {
	detected := s.Killed + s.TimedOut
	testable := s.Total - s.Errors
	if testable > 0 {
		return float64(detected) / float64(testable) * 100
	}
	return 0
}
