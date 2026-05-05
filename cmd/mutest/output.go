package mutest

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/fchimpan/mutest/mutator"
	"github.com/fchimpan/mutest/runner"
)

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

func calcKillRate(s *runner.Summary) float64 {
	detected := s.Killed + s.TimedOut
	testable := s.Total - s.Errors
	if testable > 0 {
		return float64(detected) / float64(testable) * 100
	}
	return 0
}
