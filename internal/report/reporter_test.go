package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fchimpan/mutest/internal/mutator"
	"github.com/fchimpan/mutest/internal/runner"
)

func makeResult(status mutator.MutantStatus, file string, line int, original, mutated, mutatorName string) runner.Result {
	return runner.Result{
		Mutation: mutator.Mutation{
			File:        file,
			Line:        line,
			Original:    original,
			Mutated:     mutated,
			MutatorName: mutatorName,
		},
		Status:   status,
		Duration: 100 * time.Millisecond,
	}
}

// --- ConsoleReporter Tests ---

func TestConsoleReporter_OnMutationsGenerated(t *testing.T) {
	var buf bytes.Buffer
	r := NewConsoleReporter(&buf, false)
	r.OnMutationsGenerated(42)
	if !strings.Contains(buf.String(), "42") {
		t.Errorf("expected '42' in output, got: %q", buf.String())
	}
}

func TestConsoleReporter_OnEquivalentsFiltered_NonZero(t *testing.T) {
	var buf bytes.Buffer
	r := NewConsoleReporter(&buf, false)
	r.OnEquivalentsFiltered(5)
	if !strings.Contains(buf.String(), "5") {
		t.Errorf("expected '5' in output, got: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "SSA") {
		t.Errorf("expected 'SSA' in output, got: %q", buf.String())
	}
}

func TestConsoleReporter_OnEquivalentsFiltered_Zero(t *testing.T) {
	var buf bytes.Buffer
	r := NewConsoleReporter(&buf, false)
	r.OnEquivalentsFiltered(0)
	if buf.String() != "" {
		t.Errorf("expected empty output for 0 equivalents, got: %q", buf.String())
	}
}

func TestConsoleReporter_OnNotCoveredFiltered_NonZero(t *testing.T) {
	var buf bytes.Buffer
	r := NewConsoleReporter(&buf, false)
	r.OnNotCoveredFiltered(10)
	if !strings.Contains(buf.String(), "10") {
		t.Errorf("expected '10' in output, got: %q", buf.String())
	}
}

func TestConsoleReporter_OnNotCoveredFiltered_Zero(t *testing.T) {
	var buf bytes.Buffer
	r := NewConsoleReporter(&buf, false)
	r.OnNotCoveredFiltered(0)
	if buf.String() != "" {
		t.Errorf("expected empty output for 0 not covered, got: %q", buf.String())
	}
}

func TestConsoleReporter_OnMutantResult_Verbose(t *testing.T) {
	var buf bytes.Buffer
	r := NewConsoleReporter(&buf, true)
	result := makeResult(mutator.StatusKilled, "test.go", 10, "+", "-", "arithmetic")
	r.OnMutantResult(result)
	output := buf.String()
	if !strings.Contains(output, "KILLED") {
		t.Errorf("expected KILLED in verbose output, got: %q", output)
	}
	if !strings.Contains(output, "test.go:10") {
		t.Errorf("expected file:line in verbose output, got: %q", output)
	}
}

func TestConsoleReporter_OnMutantResult_Dot(t *testing.T) {
	var buf bytes.Buffer
	r := NewConsoleReporter(&buf, false)

	tests := []struct {
		status mutator.MutantStatus
		want   string
	}{
		{mutator.StatusKilled, "."},
		{mutator.StatusSurvived, "S"},
		{mutator.StatusEquivalent, "E"},
		{mutator.StatusTimeout, "T"},
		{mutator.StatusBuildError, "!"},
	}

	for _, tt := range tests {
		buf.Reset()
		r.OnMutantResult(makeResult(tt.status, "test.go", 1, "+", "-", "arithmetic"))
		if buf.String() != tt.want {
			t.Errorf("status %v: got dot %q, want %q", tt.status, buf.String(), tt.want)
		}
	}
}

func TestConsoleReporter_Summarize(t *testing.T) {
	var buf bytes.Buffer
	r := NewConsoleReporter(&buf, false)

	results := []runner.Result{
		makeResult(mutator.StatusKilled, "test.go", 1, "+", "-", "arithmetic"),
		makeResult(mutator.StatusKilled, "test.go", 2, ">", "<=", "conditional"),
		makeResult(mutator.StatusSurvived, "test.go", 3, "&&", "||", "logical"),
		makeResult(mutator.StatusEquivalent, "test.go", 4, "+", "-", "arithmetic"),
		makeResult(mutator.StatusNotCovered, "test.go", 5, "+", "-", "arithmetic"),
		makeResult(mutator.StatusTimeout, "test.go", 6, "+", "-", "arithmetic"),
		makeResult(mutator.StatusBuildError, "test.go", 7, "+", "-", "arithmetic"),
	}

	start := time.Now().Add(-1 * time.Second) // 1 second ago
	s := r.Summarize(results, start)

	// Verify summary counts
	if s.TotalMutants != 7 {
		t.Errorf("TotalMutants = %d, want 7", s.TotalMutants)
	}
	if s.Killed != 2 {
		t.Errorf("Killed = %d, want 2", s.Killed)
	}
	if s.Survived != 1 {
		t.Errorf("Survived = %d, want 1", s.Survived)
	}
	if s.Equivalent != 1 {
		t.Errorf("Equivalent = %d, want 1", s.Equivalent)
	}
	if s.NotCovered != 1 {
		t.Errorf("NotCovered = %d, want 1", s.NotCovered)
	}
	if s.Timeout != 1 {
		t.Errorf("Timeout = %d, want 1", s.Timeout)
	}
	if s.BuildErrors != 1 {
		t.Errorf("BuildErrors = %d, want 1", s.BuildErrors)
	}

	// Score = killed / (killed + survived) = 2/3 ≈ 66.7%
	expectedScore := float64(2) / float64(3) * 100.0
	if s.MutationScore < expectedScore-0.1 || s.MutationScore > expectedScore+0.1 {
		t.Errorf("MutationScore = %.1f%%, want ≈%.1f%%", s.MutationScore, expectedScore)
	}

	// Output should contain key sections
	output := buf.String()
	if !strings.Contains(output, "Mutation Testing Report") {
		t.Error("expected 'Mutation Testing Report' in output")
	}
	if !strings.Contains(output, "Score:") {
		t.Error("expected 'Score:' in output")
	}
	if !strings.Contains(output, "Surviving Mutants") {
		t.Error("expected 'Surviving Mutants' section when there are survivors")
	}
}

func TestConsoleReporter_Summarize_NoSurvivors(t *testing.T) {
	var buf bytes.Buffer
	r := NewConsoleReporter(&buf, false)

	results := []runner.Result{
		makeResult(mutator.StatusKilled, "test.go", 1, "+", "-", "arithmetic"),
		makeResult(mutator.StatusKilled, "test.go", 2, ">", "<=", "conditional"),
	}

	start := time.Now()
	s := r.Summarize(results, start)

	if s.MutationScore != 100.0 {
		t.Errorf("MutationScore = %.1f, want 100.0", s.MutationScore)
	}

	output := buf.String()
	if strings.Contains(output, "Surviving Mutants") {
		t.Error("should not show 'Surviving Mutants' section when all killed")
	}
}

func TestConsoleReporter_Summarize_Empty(t *testing.T) {
	var buf bytes.Buffer
	r := NewConsoleReporter(&buf, false)

	start := time.Now()
	s := r.Summarize(nil, start)

	if s.TotalMutants != 0 {
		t.Errorf("TotalMutants = %d, want 0", s.TotalMutants)
	}
	if s.MutationScore != 0 {
		t.Errorf("MutationScore = %.1f, want 0", s.MutationScore)
	}
}

// --- JSONReporter Tests ---

func TestJSONReporter_OnCallbacks_AreNoops(t *testing.T) {
	var buf bytes.Buffer
	r := NewJSONReporter(&buf, false)
	// These should not panic or write anything
	r.OnMutationsGenerated(10)
	r.OnEquivalentsFiltered(5)
	r.OnNotCoveredFiltered(3)
	r.OnMutantResult(makeResult(mutator.StatusKilled, "test.go", 1, "+", "-", "arithmetic"))
	if buf.String() != "" {
		t.Errorf("expected no output from JSON reporter callbacks, got: %q", buf.String())
	}
}

func TestJSONReporter_Summarize(t *testing.T) {
	var buf bytes.Buffer
	r := NewJSONReporter(&buf, false)

	results := []runner.Result{
		makeResult(mutator.StatusKilled, "test.go", 1, "+", "-", "arithmetic"),
		makeResult(mutator.StatusSurvived, "test.go", 2, ">", "<=", "conditional"),
	}

	start := time.Now()
	s := r.Summarize(results, start)

	if s.TotalMutants != 2 {
		t.Errorf("TotalMutants = %d, want 2", s.TotalMutants)
	}

	// Verify JSON output
	var summary Summary
	if err := json.Unmarshal(buf.Bytes(), &summary); err != nil {
		t.Fatalf("JSON unmarshal error: %v\nJSON: %s", err, buf.String())
	}
	if summary.Killed != 1 {
		t.Errorf("JSON Killed = %d, want 1", summary.Killed)
	}
	if summary.Survived != 1 {
		t.Errorf("JSON Survived = %d, want 1", summary.Survived)
	}
	// Score = 1/(1+1) = 50%
	if summary.MutationScore != 50.0 {
		t.Errorf("JSON MutationScore = %.1f, want 50.0", summary.MutationScore)
	}
}

func TestJSONReporter_Summarize_Verbose(t *testing.T) {
	var buf bytes.Buffer
	r := NewJSONReporter(&buf, true)

	results := []runner.Result{
		makeResult(mutator.StatusKilled, "test.go", 1, "+", "-", "arithmetic"),
	}

	start := time.Now()
	r.Summarize(results, start)

	// Verbose should include results
	var summary Summary
	if err := json.Unmarshal(buf.Bytes(), &summary); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}
	if len(summary.Results) != 1 {
		t.Errorf("expected 1 result in verbose mode, got %d", len(summary.Results))
	}
}

func TestJSONReporter_Summarize_NonVerbose_NoResults(t *testing.T) {
	var buf bytes.Buffer
	r := NewJSONReporter(&buf, false)

	results := []runner.Result{
		makeResult(mutator.StatusKilled, "test.go", 1, "+", "-", "arithmetic"),
	}

	start := time.Now()
	r.Summarize(results, start)

	var summary Summary
	if err := json.Unmarshal(buf.Bytes(), &summary); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}
	if len(summary.Results) != 0 {
		t.Errorf("expected 0 results in non-verbose mode, got %d", len(summary.Results))
	}
}

func TestJSONReporter_Summarize_ValidJSON(t *testing.T) {
	var buf bytes.Buffer
	r := NewJSONReporter(&buf, true)

	results := []runner.Result{
		makeResult(mutator.StatusKilled, "test.go", 1, "+", "-", "arithmetic"),
		makeResult(mutator.StatusSurvived, "test.go", 2, ">", "<=", "conditional"),
		makeResult(mutator.StatusEquivalent, "test.go", 3, "&&", "||", "logical"),
		makeResult(mutator.StatusNotCovered, "test.go", 4, "+", "-", "arithmetic"),
		makeResult(mutator.StatusTimeout, "test.go", 5, "+", "-", "arithmetic"),
		makeResult(mutator.StatusBuildError, "test.go", 6, "+", "-", "arithmetic"),
		makeResult(mutator.StatusSkipped, "test.go", 7, "+", "-", "arithmetic"),
	}

	start := time.Now()
	r.Summarize(results, start)

	// Must be valid JSON
	if !json.Valid(buf.Bytes()) {
		t.Errorf("output is not valid JSON: %s", buf.String())
	}
}

// --- buildSummary Tests ---

func TestBuildSummary_AllStatuses(t *testing.T) {
	results := []runner.Result{
		{Status: mutator.StatusKilled},
		{Status: mutator.StatusKilled},
		{Status: mutator.StatusSurvived},
		{Status: mutator.StatusEquivalent},
		{Status: mutator.StatusNotCovered},
		{Status: mutator.StatusTimeout},
		{Status: mutator.StatusBuildError},
		{Status: mutator.StatusSkipped},
	}

	start := time.Now()
	s := buildSummary(results, start)

	if s.Killed != 2 {
		t.Errorf("Killed = %d, want 2", s.Killed)
	}
	if s.Survived != 1 {
		t.Errorf("Survived = %d, want 1", s.Survived)
	}
	if s.Equivalent != 1 {
		t.Errorf("Equivalent = %d, want 1", s.Equivalent)
	}
	if s.NotCovered != 1 {
		t.Errorf("NotCovered = %d, want 1", s.NotCovered)
	}
	if s.Timeout != 1 {
		t.Errorf("Timeout = %d, want 1", s.Timeout)
	}
	if s.BuildErrors != 1 {
		t.Errorf("BuildErrors = %d, want 1", s.BuildErrors)
	}
	if s.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", s.Skipped)
	}
	if s.TotalMutants != 8 {
		t.Errorf("TotalMutants = %d, want 8", s.TotalMutants)
	}
}

func TestBuildSummary_ScoreCalculation(t *testing.T) {
	tests := []struct {
		name    string
		results []runner.Result
		want    float64
	}{
		{
			name:    "all killed",
			results: []runner.Result{{Status: mutator.StatusKilled}, {Status: mutator.StatusKilled}},
			want:    100.0,
		},
		{
			name:    "none killed",
			results: []runner.Result{{Status: mutator.StatusSurvived}, {Status: mutator.StatusSurvived}},
			want:    0.0,
		},
		{
			name:    "half killed",
			results: []runner.Result{{Status: mutator.StatusKilled}, {Status: mutator.StatusSurvived}},
			want:    50.0,
		},
		{
			name: "equivalent and not_covered excluded from score",
			results: []runner.Result{
				{Status: mutator.StatusKilled},
				{Status: mutator.StatusEquivalent}, // not in denominator
				{Status: mutator.StatusNotCovered},  // not in denominator
			},
			want: 100.0, // 1 killed, 0 survived → 100%
		},
		{
			name:    "empty results",
			results: nil,
			want:    0.0,
		},
		{
			name:    "only equivalent",
			results: []runner.Result{{Status: mutator.StatusEquivalent}},
			want:    0.0, // no killed+survived → 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := buildSummary(tt.results, time.Now())
			if s.MutationScore < tt.want-0.1 || s.MutationScore > tt.want+0.1 {
				t.Errorf("MutationScore = %.1f, want %.1f", s.MutationScore, tt.want)
			}
		})
	}
}

// --- statusTag and statusDot Tests ---

func TestStatusTag(t *testing.T) {
	tests := []struct {
		status mutator.MutantStatus
		want   string
	}{
		{mutator.StatusKilled, "KILLED"},
		{mutator.StatusSurvived, "SURVIVED"},
		{mutator.StatusEquivalent, "EQUIV"},
		{mutator.StatusNotCovered, "NOCOV"},
		{mutator.StatusTimeout, "TIMEOUT"},
		{mutator.StatusBuildError, "ERROR"},
		{mutator.StatusSkipped, "SKIP"},
		{mutator.MutantStatus(99), "?"},
	}
	for _, tt := range tests {
		got := statusTag(tt.status)
		if got != tt.want {
			t.Errorf("statusTag(%v) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestStatusDot(t *testing.T) {
	tests := []struct {
		status mutator.MutantStatus
		want   string
	}{
		{mutator.StatusKilled, "."},
		{mutator.StatusSurvived, "S"},
		{mutator.StatusEquivalent, "E"},
		{mutator.StatusTimeout, "T"},
		{mutator.StatusBuildError, "!"},
		{mutator.MutantStatus(99), "?"},
	}
	for _, tt := range tests {
		got := statusDot(tt.status)
		if got != tt.want {
			t.Errorf("statusDot(%v) = %q, want %q", tt.status, got, tt.want)
		}
	}
}
