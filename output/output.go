// Package output formats mutation testing results for the CLI (text and JSON).
package output

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/fchimpan/mutest/mutator"
	"github.com/fchimpan/mutest/runner"
)

// RelPathCache avoids repeated filepath.Rel calls for the same absolute path.
// Mutation testing typically targets a handful of files with many mutation points each,
// so caching the relative path per file avoids redundant work.
type RelPathCache struct {
	base  string
	cache map[string]string
}

// NewRelPathCache returns a cache that resolves paths relative to base.
func NewRelPathCache(base string) *RelPathCache {
	return &RelPathCache{base: base, cache: make(map[string]string)}
}

// Get returns the path relative to the cache's base, falling back to the
// absolute path on error.
func (c *RelPathCache) Get(path string) string {
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

// PrintReport writes the human-readable summary to w.
func PrintReport(w io.Writer, s *runner.Summary, rpc *RelPathCache) {
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
	if s.Canceled > 0 {
		fmt.Fprintf(w, "Canceled:  %d\n", s.Canceled)
	}
	fmt.Fprintf(w, "Score:     %.1f%%\n", CalcKillRate(s))
	fmt.Fprintf(w, "Duration:  %s\n", s.Duration.Round(time.Millisecond))

	var survived []runner.Result
	for _, r := range s.Results {
		if r.Err == nil && !r.Killed && !r.TimedOut && !r.Canceled {
			survived = append(survived, r)
		}
	}

	if len(survived) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Survived mutants (test gaps):")
		for i, r := range survived {
			fmt.Fprintf(w, "  %d. %s:%d:%d  %s\n", i+1, rpc.Get(r.Point.File), r.Point.Line, r.Point.Column, r.Point.Desc)
		}
	}
}

// DryRunText writes the discovered mutation points in human-readable form.
func DryRunText(w io.Writer, points []mutator.MutationPoint, rpc *RelPathCache) {
	fmt.Fprintf(w, "mutest: discovered %d mutation points (dry run)\n\n", len(points))
	for i, p := range points {
		fmt.Fprintf(w, "  %d. %s:%d:%d  %s\n", i+1, rpc.Get(p.File), p.Line, p.Column, p.Desc)
	}
}

// CalcKillRate returns the percentage of testable mutants that were detected
// (killed or timed out). Errors and canceled mutants are excluded from the
// denominator, since neither represents a real test outcome.
func CalcKillRate(s *runner.Summary) float64 {
	detected := s.Killed + s.TimedOut
	testable := s.Total - s.Errors - s.Canceled
	if testable > 0 {
		return float64(detected) / float64(testable) * 100
	}
	return 0
}
