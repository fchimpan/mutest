package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

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
