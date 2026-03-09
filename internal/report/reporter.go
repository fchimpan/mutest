// Package report handles mutation testing output formatting.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/fchimpan/mutest/internal/mutator"
	"github.com/fchimpan/mutest/internal/runner"
)

// Summary holds the final mutation testing results.
type Summary struct {
	TotalMutants  int            `json:"total"`
	Killed        int            `json:"killed"`
	Survived      int            `json:"survived"`
	Equivalent    int            `json:"equivalent"`
	NotCovered    int            `json:"not_covered"`
	Timeout       int            `json:"timeout"`
	BuildErrors   int            `json:"build_errors"`
	Skipped       int            `json:"skipped"`
	MutationScore float64        `json:"mutation_score"`
	Duration      time.Duration  `json:"duration"`
	Results       []runner.Result `json:"results,omitempty"`
}

// Reporter receives events during pipeline execution and produces reports.
type Reporter interface {
	OnMutationsGenerated(count int)
	OnEquivalentsFiltered(count int)
	OnNotCoveredFiltered(count int)
	OnMutantResult(result runner.Result)
	Summarize(results []runner.Result, start time.Time) *Summary
}

// ConsoleReporter prints results to the terminal with progress indicators.
type ConsoleReporter struct {
	w       io.Writer
	verbose bool
	mu      sync.Mutex
}

func NewConsoleReporter(w io.Writer, verbose bool) *ConsoleReporter {
	return &ConsoleReporter{w: w, verbose: verbose}
}

func (r *ConsoleReporter) OnMutationsGenerated(count int) {
	fmt.Fprintf(r.w, "Generated %d mutations\n", count)
}

func (r *ConsoleReporter) OnEquivalentsFiltered(count int) {
	if count > 0 {
		fmt.Fprintf(r.w, "Filtered %d equivalent mutants (SSA analysis)\n", count)
	}
}

func (r *ConsoleReporter) OnNotCoveredFiltered(count int) {
	if count > 0 {
		fmt.Fprintf(r.w, "Filtered %d uncovered mutants (coverage analysis)\n", count)
	}
}

func (r *ConsoleReporter) OnMutantResult(result runner.Result) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.verbose {
		fmt.Fprintf(r.w, "  %-10s %s:%d  %s -> %s  [%s] (%s)\n",
			statusTag(result.Status),
			result.Mutation.File, result.Mutation.Line,
			result.Mutation.Original, result.Mutation.Mutated,
			result.Mutation.MutatorName,
			result.Duration.Round(time.Millisecond),
		)
	} else {
		fmt.Fprint(r.w, statusDot(result.Status))
	}
}

func (r *ConsoleReporter) Summarize(results []runner.Result, start time.Time) *Summary {
	s := buildSummary(results, start)

	if !r.verbose {
		fmt.Fprintln(r.w) // newline after dots
	}

	fmt.Fprintf(r.w, "\n── Mutation Testing Report ──\n")
	fmt.Fprintf(r.w, "Score:      %.1f%%\n", s.MutationScore)
	fmt.Fprintf(r.w, "Killed:     %d / %d\n", s.Killed, s.Killed+s.Survived)
	fmt.Fprintf(r.w, "Survived:   %d\n", s.Survived)
	fmt.Fprintf(r.w, "Equivalent: %d\n", s.Equivalent)
	fmt.Fprintf(r.w, "NotCovered: %d\n", s.NotCovered)
	fmt.Fprintf(r.w, "Timeout:    %d\n", s.Timeout)
	fmt.Fprintf(r.w, "Errors:     %d\n", s.BuildErrors)
	fmt.Fprintf(r.w, "Total:      %d\n", s.TotalMutants)
	fmt.Fprintf(r.w, "Duration:   %s\n", s.Duration.Round(time.Millisecond))

	// Show surviving mutants
	if s.Survived > 0 {
		fmt.Fprintf(r.w, "\n── Surviving Mutants ──\n")
		for _, res := range results {
			if res.Status == mutator.StatusSurvived {
				fmt.Fprintf(r.w, "  %s:%d  %s -> %s  (%s)\n",
					res.Mutation.File, res.Mutation.Line,
					res.Mutation.Original, res.Mutation.Mutated,
					res.Mutation.MutatorName,
				)
			}
		}
	}

	return s
}

// JSONReporter outputs results as JSON.
type JSONReporter struct {
	w       io.Writer
	verbose bool
	mu      sync.Mutex
}

func NewJSONReporter(w io.Writer, verbose bool) *JSONReporter {
	return &JSONReporter{w: w, verbose: verbose}
}

func (r *JSONReporter) OnMutationsGenerated(count int)    {}
func (r *JSONReporter) OnEquivalentsFiltered(count int)   {}
func (r *JSONReporter) OnNotCoveredFiltered(count int)    {}
func (r *JSONReporter) OnMutantResult(result runner.Result) {}

func (r *JSONReporter) Summarize(results []runner.Result, start time.Time) *Summary {
	s := buildSummary(results, start)
	if r.verbose {
		s.Results = results
	}
	enc := json.NewEncoder(r.w)
	enc.SetIndent("", "  ")
	enc.Encode(s)
	return s
}

func buildSummary(results []runner.Result, start time.Time) *Summary {
	s := &Summary{Duration: time.Since(start)}
	for _, res := range results {
		s.TotalMutants++
		switch res.Status {
		case mutator.StatusKilled:
			s.Killed++
		case mutator.StatusSurvived:
			s.Survived++
		case mutator.StatusEquivalent:
			s.Equivalent++
		case mutator.StatusNotCovered:
			s.NotCovered++
		case mutator.StatusTimeout:
			s.Timeout++
		case mutator.StatusBuildError:
			s.BuildErrors++
		case mutator.StatusSkipped:
			s.Skipped++
		}
	}
	denominator := s.Killed + s.Survived
	if denominator > 0 {
		s.MutationScore = float64(s.Killed) / float64(denominator) * 100.0
	}
	return s
}

func statusTag(s mutator.MutantStatus) string {
	switch s {
	case mutator.StatusKilled:
		return "KILLED"
	case mutator.StatusSurvived:
		return "SURVIVED"
	case mutator.StatusEquivalent:
		return "EQUIV"
	case mutator.StatusNotCovered:
		return "NOCOV"
	case mutator.StatusTimeout:
		return "TIMEOUT"
	case mutator.StatusBuildError:
		return "ERROR"
	case mutator.StatusSkipped:
		return "SKIP"
	default:
		return "?"
	}
}

func statusDot(s mutator.MutantStatus) string {
	switch s {
	case mutator.StatusKilled:
		return "."
	case mutator.StatusSurvived:
		return "S"
	case mutator.StatusEquivalent:
		return "E"
	case mutator.StatusTimeout:
		return "T"
	case mutator.StatusBuildError:
		return "!"
	default:
		return "?"
	}
}
