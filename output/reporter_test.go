package output

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fchimpan/mutest/config"
	"github.com/fchimpan/mutest/mutator"
	"github.com/fchimpan/mutest/runner"
)

func TestReporter_Info(t *testing.T) {
	var stdout, stderr bytes.Buffer

	textRep := NewReporter(config.Config{}, &stdout, &stderr, "/")
	if textRep.Info() != &stdout {
		t.Error("text mode: Info() should return stdout")
	}

	jsonRep := NewReporter(config.Config{JSON: true}, &stdout, &stderr, "/")
	if jsonRep.Info() != &stderr {
		t.Error("json mode: Info() should return stderr")
	}
}

func TestReporter_DryRun_TextEmpty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rep := NewReporter(config.Config{}, &stdout, &stderr, "/")
	rep.DryRun(nil)
	if !strings.Contains(stdout.String(), "no mutation points found") {
		t.Errorf("expected 'no mutation points found', got: %q", stdout.String())
	}
}

func TestReporter_DryRun_TextWithPoints(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rep := NewReporter(config.Config{}, &stdout, &stderr, "/")
	rep.DryRun([]mutator.MutationPoint{{File: "/foo.go", Line: 1, Column: 1, Desc: "test"}})
	out := stdout.String()
	if strings.Contains(out, "no mutation points found") {
		t.Errorf("with points, should not say 'no mutation points found'; got: %q", out)
	}
	if !strings.Contains(out, "discovered 1 mutation points (dry run)") {
		t.Errorf("expected dry run header, got: %q", out)
	}
}

func TestReporter_DryRun_JSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rep := NewReporter(config.Config{JSON: true}, &stdout, &stderr, "/")
	rep.DryRun([]mutator.MutationPoint{{File: "/foo.go", Line: 1, Column: 1, Desc: "test"}})
	out := stdout.String()
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("expected JSON array, got: %q", out)
	}
	if !strings.Contains(out, "foo.go") {
		t.Errorf("expected file name in JSON, got: %q", out)
	}
}

func TestReporter_NoMutationPoints_Text(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rep := NewReporter(config.Config{}, &stdout, &stderr, "/")
	rep.NoMutationPoints()
	if !strings.Contains(stdout.String(), "no mutation points found") {
		t.Errorf("expected 'no mutation points found', got: %q", stdout.String())
	}
}

func TestReporter_NoMutationPoints_JSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rep := NewReporter(config.Config{JSON: true}, &stdout, &stderr, "/")
	rep.NoMutationPoints()
	out := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(out, "{") {
		t.Errorf("expected JSON object, got: %q", out)
	}
	if !strings.Contains(out, `"total":0`) {
		t.Errorf("expected total:0 in JSON, got: %q", out)
	}
}

func TestReporter_ProgressFunc_JSONNonVerboseReturnsNil(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rep := NewReporter(config.Config{JSON: true, Verbose: false}, &stdout, &stderr, "/")
	if rep.ProgressFunc() != nil {
		t.Error("JSON non-verbose: ProgressFunc() should return nil")
	}
}

func TestReporter_ProgressFunc_JSONVerboseStreams(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rep := NewReporter(config.Config{JSON: true, Verbose: true}, &stdout, &stderr, "/")
	pf := rep.ProgressFunc()
	if pf == nil {
		t.Fatal("JSON verbose: ProgressFunc() should not be nil")
	}
	pf(runner.Result{
		Point:    mutator.MutationPoint{File: "/foo.go", Line: 1, Column: 1, Desc: "test"},
		Killed:   true,
		Duration: time.Millisecond,
	}, 1, 1)
	out := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(out, "{") || !strings.Contains(out, `"status":"killed"`) {
		t.Errorf("expected JSON result with killed status, got: %q", out)
	}
}

func TestReporter_ProgressFunc_TextEmitsStatus(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rep := NewReporter(config.Config{}, &stdout, &stderr, "/")
	pf := rep.ProgressFunc()
	pf(runner.Result{
		Point:    mutator.MutationPoint{File: "/foo.go", Line: 1, Column: 1, Desc: "test"},
		Killed:   true,
		Duration: time.Millisecond,
	}, 1, 1)
	if !strings.Contains(stdout.String(), "--- KILLED:") {
		t.Errorf("expected '--- KILLED:' marker, got: %q", stdout.String())
	}
}

func TestReporter_ProgressFunc_TextVerboseWithOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rep := NewReporter(config.Config{Verbose: true}, &stdout, &stderr, "/")
	pf := rep.ProgressFunc()
	pf(runner.Result{
		Point:    mutator.MutationPoint{File: "/foo.go", Line: 1, Column: 1, Desc: "test"},
		Killed:   true,
		Output:   "first line\nsecond line",
		Duration: time.Millisecond,
	}, 1, 1)
	out := stdout.String()
	if !strings.Contains(out, "        first line") {
		t.Errorf("expected indented 'first line', got: %q", out)
	}
	if !strings.Contains(out, "        second line") {
		t.Errorf("expected indented 'second line', got: %q", out)
	}
}

func TestReporter_ProgressFunc_TextVerboseEmptyOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rep := NewReporter(config.Config{Verbose: true}, &stdout, &stderr, "/")
	pf := rep.ProgressFunc()
	pf(runner.Result{
		Point:    mutator.MutationPoint{File: "/foo.go", Line: 1, Column: 1, Desc: "test"},
		Killed:   true,
		Output:   "",
		Duration: time.Millisecond,
	}, 1, 1)
	for line := range strings.SplitSeq(stdout.String(), "\n") {
		if strings.HasPrefix(line, "        ") {
			t.Errorf("expected no indented output for empty Output, got line: %q", line)
		}
	}
}

func TestReporter_Summary_Text(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rep := NewReporter(config.Config{}, &stdout, &stderr, "/")
	rep.Summary(&runner.Summary{Total: 3, Killed: 3})
	if !strings.Contains(stdout.String(), "Mutation Testing Summary") {
		t.Errorf("expected text summary header, got: %q", stdout.String())
	}
}

func TestReporter_Summary_JSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rep := NewReporter(config.Config{JSON: true}, &stdout, &stderr, "/")
	rep.Summary(&runner.Summary{Total: 3, Killed: 3})
	out := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(out, "{") {
		t.Errorf("expected JSON object, got: %q", out)
	}
}

func TestStatusOf(t *testing.T) {
	tests := []struct {
		name string
		res  runner.Result
		want string
	}{
		{"error", runner.Result{Err: fmt.Errorf("boom")}, "ERROR"},
		{"timeout", runner.Result{TimedOut: true}, "TIMEOUT"},
		{"survived", runner.Result{}, "SURVIVED"},
		{"killed", runner.Result{Killed: true}, "KILLED"},
		{"canceled", runner.Result{Canceled: true}, "CANCELED"},
		{"err takes precedence over timeout", runner.Result{Err: fmt.Errorf("x"), TimedOut: true}, "ERROR"},
		{"timeout takes precedence over killed", runner.Result{TimedOut: true, Killed: true}, "TIMEOUT"},
		{"canceled takes precedence over err", runner.Result{Canceled: true, Err: fmt.Errorf("x")}, "CANCELED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusOf(tt.res); got != tt.want {
				t.Errorf("statusOf() = %q, want %q", got, tt.want)
			}
		})
	}
}
