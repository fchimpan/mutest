package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/fchimpan/mutest/config"
	"github.com/fchimpan/mutest/mutator"
	"github.com/fchimpan/mutest/runner"
)

// Reporter encapsulates mode-dependent output (text vs JSON, verbose vs not)
// so that callers can dispatch to the right formatter without inspecting
// config flags themselves.
type Reporter struct {
	cfg    config.Config
	stdout io.Writer
	stderr io.Writer
	rpc    *RelPathCache
}

// NewReporter builds a Reporter for the given config. baseDir is the directory
// against which file paths are reported (typically the current working dir).
func NewReporter(cfg config.Config, stdout, stderr io.Writer, baseDir string) *Reporter {
	return &Reporter{
		cfg:    cfg,
		stdout: stdout,
		stderr: stderr,
		rpc:    NewRelPathCache(baseDir),
	}
}

// Info returns the writer for progress/status messages. In JSON mode these
// go to stderr so stdout stays machine-readable.
func (r *Reporter) Info() io.Writer {
	if r.cfg.JSON {
		return r.stderr
	}
	return r.stdout
}

// DryRun emits the discovered mutation points without running tests.
func (r *Reporter) DryRun(points []mutator.MutationPoint) {
	if r.cfg.JSON {
		// DryRunJSON encodes an empty slice as "[]" too, so no special-case needed.
		DryRunJSON(r.stdout, points, r.rpc)
		return
	}
	if len(points) == 0 {
		fmt.Fprintln(r.stdout, "mutest: no mutation points found")
		return
	}
	DryRunText(r.stdout, points, r.rpc)
}

// NoMutationPoints emits the "0 points found" output when the full pipeline
// has nothing to do.
func (r *Reporter) NoMutationPoints() {
	if r.cfg.JSON {
		WriteJSONSummary(r.stdout, &runner.Summary{}, r.rpc, true)
		return
	}
	fmt.Fprintln(r.stdout, "mutest: no mutation points found")
}

// ProgressFunc builds the per-mutant callback for the runner. Returns nil
// for JSON non-verbose mode where only the final summary is emitted.
func (r *Reporter) ProgressFunc() runner.ProgressFunc {
	if r.cfg.JSON {
		if !r.cfg.Verbose {
			return nil
		}
		enc := NewJSONEncoder(r.stdout)
		return func(res runner.Result, done, total int) {
			enc.Encode(ToJSONResult(res, r.rpc))
		}
	}
	return func(res runner.Result, done, total int) {
		fmt.Fprintf(r.stdout, "--- %s: %s:%d:%d  %s (%.2fs)\n",
			statusOf(res), r.rpc.Get(res.Point.File), res.Point.Line, res.Point.Column, res.Point.Desc, res.Duration.Seconds())
		if r.cfg.Verbose && res.Output != "" {
			for line := range strings.SplitSeq(strings.TrimRight(res.Output, "\n"), "\n") {
				fmt.Fprintf(r.stdout, "        %s\n", line)
			}
		}
	}
}

// Summary emits the aggregate post-run report.
func (r *Reporter) Summary(s *runner.Summary) {
	if r.cfg.JSON {
		// When verbose, results were already streamed as NDJSON;
		// emit summary without duplicating them.
		WriteJSONSummary(r.stdout, s, r.rpc, !r.cfg.Verbose)
		return
	}
	PrintReport(r.stdout, s, r.rpc)
}

func statusOf(r runner.Result) string {
	switch {
	case r.Canceled:
		return "CANCELED"
	case r.Err != nil:
		return "ERROR"
	case r.TimedOut:
		return "TIMEOUT"
	case !r.Killed:
		return "SURVIVED"
	default:
		return "KILLED"
	}
}
