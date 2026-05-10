package mutest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fchimpan/mutest/config"
	"github.com/fchimpan/mutest/output"
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

// requireIs asserts that err matches the given sentinel via errors.Is.
func requireIs(t *testing.T, err, sentinel error) {
	t.Helper()
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected errors.Is(err, %v), got %v", sentinel, err)
	}
}

func TestRun_WithTestProject(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  2,
		Timeout:  30 * time.Second,
		Verbose:  true,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	out := stdout.String()

	if !errors.Is(err, ErrTestsFailed) {
		t.Errorf("expected ErrTestsFailed, got %v\nstdout: %s\nstderr: %s", err, out, stderr.String())
	}
	if !strings.Contains(out, "Mutation Testing Summary") {
		t.Error("output should contain summary header")
	}
	if !strings.Contains(out, "--- KILLED:") && !strings.Contains(out, "--- SURVIVED:") {
		t.Error("output should contain --- KILLED: or --- SURVIVED: markers")
	}
	if !strings.Contains(out, "Survived mutants (test gaps):") {
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
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  10 * time.Second,
	}

	if err := run(context.Background(), cfg, &stdout, &stderr); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if !strings.Contains(stdout.String(), "no mutation points found") {
		t.Errorf("expected 'no mutation points found', got: %s", stdout.String())
	}
}

func TestRun_InvalidPattern(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./nonexistent_package_xyz"},
		Workers:  1,
		Timeout:  10 * time.Second,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	requireIs(t, err, ErrDiscovery)
}

func TestRun_NonVerbose(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  2,
		Timeout:  30 * time.Second,
		Verbose:  false,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	out := stdout.String()

	if !errors.Is(err, ErrTestsFailed) {
		t.Errorf("expected ErrTestsFailed, got %v", err)
	}
	if !strings.Contains(out, "--- KILLED:") && !strings.Contains(out, "--- SURVIVED:") {
		t.Error("default output should contain --- KILLED: or --- SURVIVED: markers")
	}
	if !strings.Contains(out, "Mutation Testing Summary") {
		t.Error("output should contain summary")
	}
}

func TestRun_Verbose_ShowsTestOutput(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  2,
		Timeout:  30 * time.Second,
		Verbose:  true,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	out := stdout.String()

	if !errors.Is(err, ErrTestsFailed) {
		t.Errorf("expected ErrTestsFailed, got %v", err)
	}
	if !strings.Contains(out, "--- KILLED:") && !strings.Contains(out, "--- SURVIVED:") {
		t.Error("verbose output should contain --- KILLED: or --- SURVIVED: markers")
	}
	if !strings.Contains(out, "        ") {
		t.Error("verbose output should contain indented test output lines")
	}
}

func TestValidateConfig_InvalidWorkers(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  0,
		Timeout:  10 * time.Second,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	requireIs(t, err, ErrInvalidConfig)
	if !strings.Contains(err.Error(), "-workers must be > 0") {
		t.Errorf("expected workers validation error, got: %v", err)
	}
}

func TestValidateConfig_InvalidTimeout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  0,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	requireIs(t, err, ErrInvalidConfig)
	if !strings.Contains(err.Error(), "-timeout must be > 0") {
		t.Errorf("expected timeout validation error, got: %v", err)
	}
}

func TestRun_DryRun_Text(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  10 * time.Second,
		DryRun:   true,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	out := stdout.String()

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if !strings.Contains(out, "dry run") {
		t.Error("output should mention dry run")
	}
	if !strings.Contains(out, "mutation points") {
		t.Error("output should mention mutation points")
	}
	if strings.Contains(out, "Mutation Testing Summary") {
		t.Error("dry-run should not run tests or show summary")
	}
}

func TestRun_DryRun_JSON(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  10 * time.Second,
		DryRun:   true,
		JSON:     true,
	}

	if err := run(context.Background(), cfg, &stdout, &stderr); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	var points []output.JSONMutationPoint
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
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  10 * time.Second,
		DryRun:   true,
	}

	if err := run(context.Background(), cfg, &stdout, &stderr); err != nil {
		t.Errorf("expected nil error, got %v", err)
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
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  10 * time.Second,
		DryRun:   true,
		JSON:     true,
	}

	if err := run(context.Background(), cfg, &stdout, &stderr); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	var points []output.JSONMutationPoint
	if err := json.Unmarshal(stdout.Bytes(), &points); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}
	if len(points) != 0 {
		t.Errorf("expected empty array, got %d points", len(points))
	}
}

func TestRun_JSON_Summary(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  2,
		Timeout:  30 * time.Second,
		JSON:     true,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	if !errors.Is(err, ErrTestsFailed) {
		t.Errorf("expected ErrTestsFailed, got %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	var summary output.JSONSummary
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

	if strings.Contains(stdout.String(), "mutest:") {
		t.Error("JSON stdout should not contain informational messages")
	}
}

func TestRun_JSON_Verbose_NDJSON(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  2,
		Timeout:  30 * time.Second,
		JSON:     true,
		Verbose:  true,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	if !errors.Is(err, ErrTestsFailed) {
		t.Errorf("expected ErrTestsFailed, got %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 NDJSON lines, got %d: %s", len(lines), stdout.String())
	}

	for i, line := range lines[:len(lines)-1] {
		var result output.JSONResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			t.Errorf("line %d: invalid JSON: %v\nraw: %s", i, err, line)
			continue
		}
		if result.Status == "" {
			t.Errorf("line %d: missing status", i)
		}
	}

	var summary output.JSONSummary
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &summary); err != nil {
		t.Fatalf("last line not a valid summary: %v\nraw: %s", err, lines[len(lines)-1])
	}
	if summary.Total == 0 {
		t.Error("summary total should be > 0")
	}
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
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  10 * time.Second,
		JSON:     true,
	}

	if err := run(context.Background(), cfg, &stdout, &stderr); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	var summary output.JSONSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout.String())
	}
	if summary.Total != 0 {
		t.Errorf("expected total 0, got %d", summary.Total)
	}
}

func TestRun_Threshold_Met(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns:  []string{"./..."},
		Workers:   2,
		Timeout:   30 * time.Second,
		Threshold: 20.0,
	}

	if err := run(context.Background(), cfg, &stdout, &stderr); err != nil {
		t.Errorf("expected nil error (threshold met), got %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
}

func TestRun_Threshold_NotMet(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns:  []string{"./..."},
		Workers:   2,
		Timeout:   30 * time.Second,
		Threshold: 90.0,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	if !errors.Is(err, ErrTestsFailed) {
		t.Errorf("expected ErrTestsFailed (threshold not met), got %v", err)
	}
}

func TestRun_Threshold_Zero_DefaultBehavior(t *testing.T) {
	chdir(t, "../../testdata/project")

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns:  []string{"./..."},
		Workers:   2,
		Timeout:   30 * time.Second,
		Threshold: 0,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	if !errors.Is(err, ErrTestsFailed) {
		t.Errorf("expected ErrTestsFailed (default: survived > 0), got %v", err)
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
			cfg := config.Config{
				Patterns:  []string{"./..."},
				Workers:   1,
				Timeout:   10 * time.Second,
				Threshold: tt.threshold,
			}
			err := run(context.Background(), cfg, &stdout, &stderr)
			requireIs(t, err, ErrInvalidConfig)
			if !strings.Contains(err.Error(), "-threshold") {
				t.Errorf("expected threshold validation error, got: %v", err)
			}
		})
	}
}

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
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  30 * time.Second,
		DryRun:   true,
		JSON:     true,
	}

	if err := run(context.Background(), cfg, &stdout, &stderr); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	var points []output.JSONMutationPoint
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
