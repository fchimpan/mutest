package mutest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fchimpan/mutest/mutator"
	"github.com/fchimpan/mutest/runner"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

// --- Existing tests (unchanged behavior) ---

func TestRun_WithTestProject(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  2,
		Timeout:  30 * time.Second,
		Verbose:  true,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)
	output := stdout.String()

	if code != 1 {
		t.Errorf("expected exit code 1, got %d\nstdout: %s\nstderr: %s", code, output, stderr.String())
	}
	if !strings.Contains(output, "Mutation Testing Summary") {
		t.Error("output should contain summary header")
	}
	if !strings.Contains(output, "--- KILLED:") && !strings.Contains(output, "--- SURVIVED:") {
		t.Error("output should contain --- KILLED: or --- SURVIVED: markers")
	}
	if !strings.Contains(output, "Survived mutants (test gaps):") {
		t.Error("output should list survived mutants")
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %s", stderr.String())
	}
}

func TestRun_NoMutationPoints(t *testing.T) {
	tmpDir := t.TempDir()

	for name, content := range map[string]string{
		"go.mod":        "module example.com/empty\n\ngo 1.21\n",
		"empty.go":      "package empty\n\nfunc Add(a, b int) int { return a + b }\n",
		"empty_test.go": "package empty\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1,2) != 3 { t.Fail() } }\n",
	} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	chdir(t, tmpDir)

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  10 * time.Second,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)

	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "no mutation points found") {
		t.Errorf("expected 'no mutation points found', got: %s", stdout.String())
	}
}

func TestRun_InvalidPattern(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./nonexistent_package_xyz"},
		Workers:  1,
		Timeout:  10 * time.Second,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)

	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "error discovering mutations") {
		t.Errorf("expected error message in stderr, got: %s", stderr.String())
	}
}

func TestRun_NonVerbose(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  2,
		Timeout:  30 * time.Second,
		Verbose:  false,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)
	output := stdout.String()

	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	// Default mode now shows per-mutant progress
	if !strings.Contains(output, "--- KILLED:") && !strings.Contains(output, "--- SURVIVED:") {
		t.Error("default output should contain --- KILLED: or --- SURVIVED: markers")
	}
	if !strings.Contains(output, "Mutation Testing Summary") {
		t.Error("output should contain summary")
	}
}

func TestRun_Verbose_ShowsTestOutput(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  2,
		Timeout:  30 * time.Second,
		Verbose:  true,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)
	output := stdout.String()

	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(output, "--- KILLED:") && !strings.Contains(output, "--- SURVIVED:") {
		t.Error("verbose output should contain --- KILLED: or --- SURVIVED: markers")
	}
	// Verbose mode should include indented test output from killed mutants
	if !strings.Contains(output, "        ") {
		t.Error("verbose output should contain indented test output lines")
	}
}

func TestPrintReport_AllKilled(t *testing.T) {
	var buf bytes.Buffer
	summary := &runner.Summary{
		Total:    3,
		Killed:   3,
		Survived: 0,
		Duration: 500 * time.Millisecond,
	}

	printReport(&buf, summary, newRelPathCache("/base"))
	output := buf.String()

	if !strings.Contains(output, "Score:     100.0%") {
		t.Errorf("expected Score: 100.0%%, got: %s", output)
	}
	if strings.Contains(output, "Survived mutants") {
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

	printReport(&buf, summary, newRelPathCache("/base"))
	output := buf.String()

	if !strings.Contains(output, "Errors:    2") {
		t.Errorf("expected Errors: 2 in output, got: %s", output)
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

	printReport(&buf, summary, newRelPathCache("/base"))
	output := buf.String()

	if !strings.Contains(output, "Score:     0.0%") {
		t.Errorf("expected Score: 0.0%%, got: %s", output)
	}
}

// --- Input validation tests ---

func TestValidateConfig_InvalidWorkers(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  0,
		Timeout:  10 * time.Second,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)

	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "-workers must be > 0") {
		t.Errorf("expected workers validation error, got: %s", stderr.String())
	}
}

func TestValidateConfig_InvalidTimeout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  0,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)

	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "-timeout must be > 0") {
		t.Errorf("expected timeout validation error, got: %s", stderr.String())
	}
}

// --- Dry-run tests ---

func TestRun_DryRun_Text(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  10 * time.Second,
		DryRun:   true,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)
	output := stdout.String()

	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(output, "dry run") {
		t.Error("output should mention dry run")
	}
	if !strings.Contains(output, "mutation points") {
		t.Error("output should mention mutation points")
	}
	// Should NOT contain test summary (tests were not run)
	if strings.Contains(output, "Mutation Testing Summary") {
		t.Error("dry-run should not run tests or show summary")
	}
}

func TestRun_DryRun_JSON(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  10 * time.Second,
		DryRun:   true,
		JSON:     true,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)

	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	var points []jsonMutationPoint
	if err := json.Unmarshal(stdout.Bytes(), &points); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}

	if len(points) == 0 {
		t.Error("expected at least one mutation point")
	}
	for i, p := range points {
		if p.File == "" {
			t.Errorf("point[%d].file is empty", i)
		}
		if p.Original == "" || p.Mutated == "" {
			t.Errorf("point[%d] missing original/mutated operator", i)
		}
	}
}

func TestRun_DryRun_NoMutations(t *testing.T) {
	tmpDir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":        "module example.com/empty\n\ngo 1.21\n",
		"empty.go":      "package empty\n\nfunc Add(a, b int) int { return a + b }\n",
		"empty_test.go": "package empty\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1,2) != 3 { t.Fail() } }\n",
	} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, tmpDir)

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  10 * time.Second,
		DryRun:   true,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "no mutation points found") {
		t.Errorf("expected 'no mutation points found', got: %s", stdout.String())
	}
}

func TestRun_DryRun_JSON_NoMutations(t *testing.T) {
	tmpDir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":        "module example.com/empty\n\ngo 1.21\n",
		"empty.go":      "package empty\n\nfunc Add(a, b int) int { return a + b }\n",
		"empty_test.go": "package empty\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1,2) != 3 { t.Fail() } }\n",
	} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, tmpDir)

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  10 * time.Second,
		DryRun:   true,
		JSON:     true,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	var points []jsonMutationPoint
	if err := json.Unmarshal(stdout.Bytes(), &points); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}
	if len(points) != 0 {
		t.Errorf("expected empty array, got %d points", len(points))
	}
}

// --- JSON output tests ---

func TestRun_JSON_Summary(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  2,
		Timeout:  30 * time.Second,
		JSON:     true,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)

	if code != 1 {
		t.Errorf("expected exit code 1, got %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	var summary jsonSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}

	if summary.Total == 0 {
		t.Error("expected total > 0")
	}
	if summary.Killed == 0 {
		t.Error("expected at least one killed mutant")
	}
	if summary.Survived == 0 {
		t.Error("expected at least one survived mutant")
	}
	if len(summary.Results) != summary.Total {
		t.Errorf("results length %d != total %d", len(summary.Results), summary.Total)
	}
	if summary.KillRate <= 0 || summary.KillRate >= 100 {
		t.Errorf("unexpected kill rate: %f", summary.KillRate)
	}

	// Informational messages should be on stderr, not stdout
	if strings.Contains(stdout.String(), "mutest:") {
		t.Error("JSON stdout should not contain informational messages")
	}
}

func TestRun_JSON_Verbose_NDJSON(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  2,
		Timeout:  30 * time.Second,
		JSON:     true,
		Verbose:  true,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)

	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 NDJSON lines, got %d: %s", len(lines), stdout.String())
	}

	// All lines except the last should be individual results
	for i, line := range lines[:len(lines)-1] {
		var result jsonResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			t.Errorf("line %d: invalid JSON: %v\nraw: %s", i, err, line)
			continue
		}
		if result.Status == "" {
			t.Errorf("line %d: missing status", i)
		}
	}

	// Last line should be the summary
	var summary jsonSummary
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &summary); err != nil {
		t.Fatalf("last line not a valid summary: %v\nraw: %s", err, lines[len(lines)-1])
	}
	if summary.Total == 0 {
		t.Error("summary total should be > 0")
	}
	// In verbose mode, results are already streamed, so summary.Results should be nil/empty
	if len(summary.Results) != 0 {
		t.Error("verbose JSON summary should not duplicate results")
	}
}

func TestRun_JSON_NoMutations(t *testing.T) {
	tmpDir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":        "module example.com/empty\n\ngo 1.21\n",
		"empty.go":      "package empty\n\nfunc Add(a, b int) int { return a + b }\n",
		"empty_test.go": "package empty\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1,2) != 3 { t.Fail() } }\n",
	} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, tmpDir)

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  10 * time.Second,
		JSON:     true,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	var summary jsonSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}
	if summary.Total != 0 {
		t.Errorf("expected total 0, got %d", summary.Total)
	}
}

// --- Unit tests for JSON helpers ---

func TestWriteJSONSummary(t *testing.T) {
	var buf bytes.Buffer
	summary := &runner.Summary{
		Total:    4,
		Killed:   3,
		Survived: 1,
		Errors:   0,
		Duration: 1234 * time.Millisecond,
	}

	writeJSONSummary(&buf, summary, newRelPathCache("/base"), true)

	var js jsonSummary
	if err := json.Unmarshal(buf.Bytes(), &js); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if js.Total != 4 {
		t.Errorf("expected total 4, got %d", js.Total)
	}
	if js.KillRate != 75.0 {
		t.Errorf("expected kill rate 75.0, got %f", js.KillRate)
	}
	if js.Duration != "1.234s" {
		t.Errorf("expected duration '1.234s', got %s", js.Duration)
	}
}

func TestToJSONResult(t *testing.T) {
	rpc := newRelPathCache("/work")

	t.Run("killed", func(t *testing.T) {
		r := runner.Result{
			Point:    mutator.MutationPoint{File: "/work/foo.go", Package: "foo", Line: 10, Column: 5},
			Killed:   true,
			Duration: 123 * time.Millisecond,
		}
		jr := toJSONResult(r, rpc)
		if jr.Status != "killed" {
			t.Errorf("expected status killed, got %s", jr.Status)
		}
		if jr.File != "foo.go" {
			t.Errorf("expected relative path foo.go, got %s", jr.File)
		}
		if jr.Error != "" {
			t.Error("killed result should have no error")
		}
	})

	t.Run("survived", func(t *testing.T) {
		r := runner.Result{
			Point:    mutator.MutationPoint{File: "/work/bar.go"},
			Killed:   false,
			Duration: 50 * time.Millisecond,
		}
		jr := toJSONResult(r, rpc)
		if jr.Status != "survived" {
			t.Errorf("expected status survived, got %s", jr.Status)
		}
	})

	t.Run("error", func(t *testing.T) {
		r := runner.Result{
			Point:    mutator.MutationPoint{File: "/work/baz.go"},
			Err:      fmt.Errorf("prepare failed"),
			Duration: 1 * time.Millisecond,
		}
		jr := toJSONResult(r, rpc)
		if jr.Status != "error" {
			t.Errorf("expected status error, got %s", jr.Status)
		}
		if jr.Error != "prepare failed" {
			t.Errorf("expected error message, got %s", jr.Error)
		}
	})

	t.Run("timed_out", func(t *testing.T) {
		r := runner.Result{
			Point:    mutator.MutationPoint{File: "/work/timeout.go"},
			Killed:   false,
			TimedOut: true,
			Duration: 30 * time.Second,
		}
		jr := toJSONResult(r, rpc)
		if jr.Status != "timeout" {
			t.Errorf("expected status timeout, got %s", jr.Status)
		}
		if !jr.TimedOut {
			t.Error("expected timed_out to be true")
		}
	})
}

// --- Threshold tests ---

func TestRun_Threshold_Met(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns:  []string{"./..."},
		Workers:   2,
		Timeout:   30 * time.Second,
		Threshold: 20.0, // kill rate is 25%, so 20% threshold should pass
	}

	code := run(context.Background(), cfg, &stdout, &stderr)

	if code != 0 {
		t.Errorf("expected exit code 0 (threshold met), got %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
}

func TestRun_Threshold_NotMet(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns:  []string{"./..."},
		Workers:   2,
		Timeout:   30 * time.Second,
		Threshold: 90.0, // kill rate is 50%, so 90% threshold should fail
	}

	code := run(context.Background(), cfg, &stdout, &stderr)

	if code != 1 {
		t.Errorf("expected exit code 1 (threshold not met), got %d", code)
	}
}

func TestRun_Threshold_Zero_DefaultBehavior(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns:  []string{"./..."},
		Workers:   2,
		Timeout:   30 * time.Second,
		Threshold: 0, // default: any survived = fail
	}

	code := run(context.Background(), cfg, &stdout, &stderr)

	// testdata has survived mutants, so with threshold=0 (default) it should return 1
	if code != 1 {
		t.Errorf("expected exit code 1 (default: survived > 0), got %d", code)
	}
}

func TestValidateConfig_InvalidThreshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
	}{
		{"negative", -1},
		{"over 100", 101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cfg := config{
				Patterns:  []string{"./..."},
				Workers:   1,
				Timeout:   10 * time.Second,
				Threshold: tt.threshold,
			}
			code := run(context.Background(), cfg, &stdout, &stderr)
			if code != 2 {
				t.Errorf("expected exit code 2, got %d", code)
			}
			if !strings.Contains(stderr.String(), "-threshold") {
				t.Errorf("expected threshold validation error, got: %s", stderr.String())
			}
		})
	}
}

// --- MutatorName in discovery ---

func TestRun_EqualityMutator_Discovered(t *testing.T) {
	tmpDir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":     "module example.com/eq\n\ngo 1.21\n",
		"eq.go":      "package eq\n\nfunc Equal(a, b int) bool { return a == b }\n",
		"eq_test.go": "package eq\n\nimport \"testing\"\n\nfunc TestEqual(t *testing.T) { if !Equal(3,3) { t.Fail() }; if Equal(3,4) { t.Fail() } }\n",
	} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, tmpDir)

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  30 * time.Second,
		DryRun:   true,
		JSON:     true,
	}

	code := run(context.Background(), cfg, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	var points []jsonMutationPoint
	if err := json.Unmarshal(stdout.Bytes(), &points); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}
	if len(points) != 1 {
		t.Fatalf("expected 1 equality mutation point, got %d", len(points))
	}
	if points[0].Original != "==" || points[0].Mutated != "!=" {
		t.Errorf("expected == to != mutation, got %s to %s", points[0].Original, points[0].Mutated)
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
			got := calcKillRate(tt.summary)
			if got != tt.want {
				t.Errorf("calcKillRate() = %f, want %f", got, tt.want)
			}
		})
	}
}
