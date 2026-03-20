package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestRun_WithTestProject(t *testing.T) {
	chdir(t, "testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  2,
		Timeout:  30 * time.Second,
		Verbose:  true,
	}

	code := run2(cfg, &stdout, &stderr)
	output := stdout.String()

	if code != 1 {
		t.Errorf("expected exit code 1, got %d\nstdout: %s\nstderr: %s", code, output, stderr.String())
	}
	if !strings.Contains(output, "Mutation Testing Summary") {
		t.Error("output should contain summary header")
	}
	if !strings.Contains(output, "[KILLED") && !strings.Contains(output, "[SURVIVED") {
		t.Error("verbose output should contain [KILLED] or [SURVIVED] markers")
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

	code := run2(cfg, &stdout, &stderr)

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

	code := run2(cfg, &stdout, &stderr)

	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "error discovering mutations") {
		t.Errorf("expected error message in stderr, got: %s", stderr.String())
	}
}

func TestRun_NonVerbose(t *testing.T) {
	chdir(t, "testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config{
		Patterns: []string{"./..."},
		Workers:  2,
		Timeout:  30 * time.Second,
		Verbose:  false,
	}

	code := run2(cfg, &stdout, &stderr)
	output := stdout.String()

	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if strings.Contains(output, "[KILLED") || strings.Contains(output, "[SURVIVED") {
		t.Error("non-verbose output should not contain per-mutant markers")
	}
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

	if !strings.Contains(output, "0.0%") {
		t.Errorf("expected 0.0%% kill rate, got: %s", output)
	}
}
