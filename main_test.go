package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fchimpan/mutest/runner"
)

func TestRun_WithTestProject(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := config{
		Dir:     "testdata/project",
		Workers: 2,
		Timeout: 30 * time.Second,
		Verbose: true,
	}

	code := run(cfg, &stdout, &stderr)

	output := stdout.String()

	// Should exit 1 because there are surviving mutants
	if code != 1 {
		t.Errorf("expected exit code 1, got %d\nstdout: %s\nstderr: %s", code, output, stderr.String())
	}

	// Should contain summary
	if !strings.Contains(output, "Mutation Testing Summary") {
		t.Error("output should contain summary header")
	}
	if !strings.Contains(output, "Killed:") {
		t.Error("output should contain Killed count")
	}
	if !strings.Contains(output, "Survived:") {
		t.Error("output should contain Survived count")
	}

	// Verbose mode should show per-mutant results
	if !strings.Contains(output, "[KILLED") && !strings.Contains(output, "[SURVIVED") {
		t.Error("verbose output should contain [KILLED] or [SURVIVED] markers")
	}

	// Should list survived mutants
	if !strings.Contains(output, "Survived mutants (test gaps):") {
		t.Error("output should list survived mutants")
	}

	// Stderr should be empty
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %s", stderr.String())
	}
}

func TestRun_NoMutationPoints(t *testing.T) {
	// Use the engine package dir which has no comparison operators in non-test files...
	// Actually, let's create a temp dir with a Go file that has no comparisons.
	tmpDir := t.TempDir()

	gomod := "module example.com/empty\n\ngo 1.21\n"
	goSrc := "package empty\n\nfunc Add(a, b int) int { return a + b }\n"
	testSrc := "package empty\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1,2) != 3 { t.Fail() } }\n"

	for name, content := range map[string]string{
		"go.mod":       gomod,
		"empty.go":     goSrc,
		"empty_test.go": testSrc,
	} {
		if err := writeFile(tmpDir+"/"+name, content); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	cfg := config{
		Dir:     tmpDir,
		Workers: 1,
		Timeout: 10 * time.Second,
	}

	code := run(cfg, &stdout, &stderr)

	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "no mutation points found") {
		t.Errorf("expected 'no mutation points found', got: %s", stdout.String())
	}
}

func TestRun_InvalidDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := config{
		Dir:     "/nonexistent/dir",
		Workers: 1,
		Timeout: 10 * time.Second,
	}

	code := run(cfg, &stdout, &stderr)

	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "error discovering mutations") {
		t.Errorf("expected error message in stderr, got: %s", stderr.String())
	}
}

func TestRun_NonVerbose(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := config{
		Dir:     "testdata/project",
		Workers: 2,
		Timeout: 30 * time.Second,
		Verbose: false,
	}

	code := run(cfg, &stdout, &stderr)

	output := stdout.String()

	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}

	// Non-verbose should NOT show per-mutant results
	if strings.Contains(output, "[KILLED") || strings.Contains(output, "[SURVIVED") {
		t.Error("non-verbose output should not contain per-mutant markers")
	}

	// But should still have summary
	if !strings.Contains(output, "Mutation Testing Summary") {
		t.Error("output should contain summary")
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

	printReport(&buf, summary, "/base")
	output := buf.String()

	if !strings.Contains(output, "100.0%") {
		t.Errorf("expected 100.0%% kill rate, got: %s", output)
	}
	if strings.Contains(output, "Survived mutants") {
		t.Error("should not list survived mutants when all are killed")
	}
	if strings.Contains(output, "Errors:") {
		t.Error("should not show Errors line when there are none")
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

	printReport(&buf, summary, "/base")
	output := buf.String()

	if !strings.Contains(output, "Errors:    2") {
		t.Errorf("expected Errors: 2 in output, got: %s", output)
	}
}

func TestRelPath(t *testing.T) {
	tests := []struct {
		base, path, want string
	}{
		{"/home/user/project", "/home/user/project/src/main.go", "src/main.go"},
		{"/home/user/project", "/other/path/file.go", "../../../other/path/file.go"},
	}
	for _, tt := range tests {
		got := relPath(tt.base, tt.path)
		if got != tt.want {
			t.Errorf("relPath(%q, %q) = %q, want %q", tt.base, tt.path, got, tt.want)
		}
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

	printReport(&buf, summary, "/base")
	output := buf.String()

	// Kill rate should be 0.0% when all are errors
	if !strings.Contains(output, "0.0%") {
		t.Errorf("expected 0.0%% kill rate, got: %s", output)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
