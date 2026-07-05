package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fchimpan/mutest/mutator"
	"github.com/fchimpan/mutest/runner"
)

func TestPrintReport_AllKilled(t *testing.T) {
	var buf bytes.Buffer
	summary := &runner.Summary{
		Total:    3,
		Killed:   3,
		Survived: 0,
		Duration: 500 * time.Millisecond,
	}

	PrintReport(&buf, summary, NewRelPathCache("/base"))
	out := buf.String()

	if !strings.Contains(out, "Score:     100.0%") {
		t.Errorf("expected Score: 100.0%%, got: %s", out)
	}
	if strings.Contains(out, "Survived mutants") {
		t.Error("should not list survived mutants when all are killed")
	}
}

func TestPrintReport_WithErrors(t *testing.T) {
	var buf bytes.Buffer
	summary := &runner.Summary{
		Total:    5,
		Killed:   2,
		Survived: 1,
		Errors:   2,
		Duration: 1 * time.Second,
	}

	PrintReport(&buf, summary, NewRelPathCache("/base"))
	out := buf.String()

	if !strings.Contains(out, "Errors:    2") {
		t.Errorf("expected Errors: 2 in output, got: %s", out)
	}
}

func TestPrintReport_AllErrors(t *testing.T) {
	var buf bytes.Buffer
	summary := &runner.Summary{
		Total:    2,
		Killed:   0,
		Survived: 0,
		Errors:   2,
		Duration: 100 * time.Millisecond,
	}

	PrintReport(&buf, summary, NewRelPathCache("/base"))
	out := buf.String()

	if !strings.Contains(out, "Score:     0.0%") {
		t.Errorf("expected Score: 0.0%%, got: %s", out)
	}
}

func TestPrintReport_Canceled(t *testing.T) {
	var buf bytes.Buffer
	summary := &runner.Summary{
		Total:    2,
		Killed:   1,
		Canceled: 1,
		Duration: 100 * time.Millisecond,
		Results: []runner.Result{
			{Point: mutator.MutationPoint{File: "/base/a.go", Line: 1, Column: 1, Desc: "> to >="}, Killed: true},
			{Point: mutator.MutationPoint{File: "/base/b.go", Line: 2, Column: 1, Desc: "< to <="}, Canceled: true},
		},
	}

	PrintReport(&buf, summary, NewRelPathCache("/base"))
	out := buf.String()

	if !strings.Contains(out, "Canceled:") {
		t.Errorf("expected a 'Canceled:' line, got: %s", out)
	}
	if strings.Contains(out, "Survived mutants") {
		t.Errorf("canceled mutants must not be listed as survived, got: %s", out)
	}
}

func TestPrintReport_NoCanceledLineWhenZero(t *testing.T) {
	var buf bytes.Buffer
	summary := &runner.Summary{
		Total:    2,
		Killed:   2,
		Duration: 100 * time.Millisecond,
	}

	PrintReport(&buf, summary, NewRelPathCache("/base"))

	if strings.Contains(buf.String(), "Canceled:") {
		t.Errorf("Canceled line must be omitted when Canceled == 0, got: %s", buf.String())
	}
}

func TestPrintReport_ErroredNotListedAsSurvived(t *testing.T) {
	var buf bytes.Buffer
	summary := &runner.Summary{
		Total:    2,
		Killed:   1,
		Errors:   1,
		Duration: 100 * time.Millisecond,
		Results: []runner.Result{
			{Point: mutator.MutationPoint{File: "/base/a.go", Line: 1, Column: 1, Desc: "> to >="}, Killed: true},
			{Point: mutator.MutationPoint{File: "/base/b.go", Line: 2, Column: 1, Desc: "< to <="}, Err: errors.New("exec failed")},
		},
	}

	PrintReport(&buf, summary, NewRelPathCache("/base"))

	if strings.Contains(buf.String(), "Survived mutants") {
		t.Errorf("errored results must not be listed as survived, got: %s", buf.String())
	}
}

// TestRoundedKillRate covers F11: threshold comparisons must use the same
// rounding as the displayed score (text %.1f, JSON kill_rate), so a run that
// displays "Score: 80.0%" never fails "-threshold 80" over an unrounded
// fractional remainder like 79.96.
func TestRoundedKillRate(t *testing.T) {
	tests := []struct {
		name    string
		summary *runner.Summary
		want    float64
	}{
		{"exact percentage is unaffected", &runner.Summary{Total: 4, Killed: 3}, 75.0},
		{"79.96 rounds up to 80.0", &runner.Summary{Total: 2500, Killed: 1999}, 80.0},
		{"79.94 rounds down to 79.9", &runner.Summary{Total: 5000, Killed: 3997}, 79.9},
		{"all survived is 0.0", &runner.Summary{Total: 3, Survived: 3}, 0.0},
		{"empty summary is 0.0", &runner.Summary{}, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RoundedKillRate(tt.summary); got != tt.want {
				t.Errorf("RoundedKillRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalcKillRate(t *testing.T) {
	tests := []struct {
		name    string
		summary *runner.Summary
		want    float64
	}{
		{"all killed", &runner.Summary{Total: 3, Killed: 3}, 100.0},
		{"none killed", &runner.Summary{Total: 3, Survived: 3}, 0.0},
		{"with errors", &runner.Summary{Total: 5, Killed: 2, Survived: 1, Errors: 2}, float64(2) / float64(3) * 100},
		{"all errors", &runner.Summary{Total: 2, Errors: 2}, 0.0},
		{"empty", &runner.Summary{}, 0.0},
		{"with timeout", &runner.Summary{Total: 5, Killed: 2, TimedOut: 1, Survived: 2}, float64(3) / float64(5) * 100},
		{"timeout and errors", &runner.Summary{Total: 6, Killed: 2, TimedOut: 1, Survived: 1, Errors: 2}, float64(3) / float64(4) * 100},
		{"all timeout", &runner.Summary{Total: 3, TimedOut: 3}, 100.0},
		{"with canceled", &runner.Summary{Total: 5, Killed: 2, Canceled: 1, Survived: 2}, float64(2) / float64(4) * 100},
		{"canceled and errors", &runner.Summary{Total: 6, Killed: 2, Canceled: 1, Errors: 1, Survived: 2}, float64(2) / float64(4) * 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalcKillRate(tt.summary)
			if got != tt.want {
				t.Errorf("CalcKillRate() = %f, want %f", got, tt.want)
			}
		})
	}
}
