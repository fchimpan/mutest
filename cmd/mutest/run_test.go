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

// writeFiles writes name->content files under dir, creating parent
// directories as needed. Names may contain slashes for subpackages.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
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

// TestRun_CwdRelativeToPackageDir covers F2: the test binary must run with the
// package source directory as its cwd, so testdata relative reads succeed. The
// boundary (10 vs 11) is intentionally untested, so the > -> >= mutant survives.
// Before the fix, the binary ran in the module root, the fixture read failed,
// and the mutant was a false KILLED.
func TestRun_CwdRelativeToPackageDir(t *testing.T) {
	tmpDir := t.TempDir()
	writeFiles(t, tmpDir, map[string]string{
		"go.mod":        "module example.com/cwdmod\n\ngo 1.21\n",
		"cwdpkg/lib.go": "package cwdpkg\n\nfunc Threshold(n int) bool { return n > 10 }\n",
		"cwdpkg/lib_test.go": `package cwdpkg

import (
	"os"
	"testing"
)

func TestThreshold(t *testing.T) {
	if _, err := os.ReadFile("testdata/fixture.txt"); err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	// Boundary (10 vs 11) intentionally untested: > -> >= must SURVIVE.
	if !Threshold(20) || Threshold(0) {
		t.Fatal("wrong")
	}
}
`,
		"cwdpkg/testdata/fixture.txt": "fixture\n",
	})

	chdir(t, tmpDir) // module root, NOT the package dir

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./cwdpkg"},
		Workers:  1,
		Timeout:  30 * time.Second,
		JSON:     true,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	if !errors.Is(err, ErrTestsFailed) {
		t.Fatalf("expected ErrTestsFailed (survived mutant), got %v\nstderr: %s", err, stderr.String())
	}

	var summary output.JSONSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if summary.Survived != 1 {
		t.Errorf("expected 1 survived mutant, got %d (killed=%d)", summary.Survived, summary.Killed)
	}
	if summary.Killed != 0 {
		t.Errorf("expected 0 killed, got %d", summary.Killed)
	}
}

// TestRun_NoTestFiles covers F3: a package with no test files cannot have a
// perfect score. Its mutants must SURVIVE and the run must fail. Before the
// fix, the missing test binary made every mutant a false KILLED / Score 100%.
func TestRun_NoTestFiles(t *testing.T) {
	tmpDir := t.TempDir()
	writeFiles(t, tmpDir, map[string]string{
		"go.mod":        "module example.com/notestmod\n\ngo 1.21\n",
		"notest/lib.go": "package notest\n\nfunc Max(a, b int) int {\n\tif a > b {\n\t\treturn a\n\t}\n\treturn b\n}\n",
	})

	chdir(t, tmpDir)

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./notest"},
		Workers:  1,
		Timeout:  30 * time.Second,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	out := stdout.String()

	requireIs(t, err, ErrTestsFailed)
	if !strings.Contains(out, "has no test files") {
		t.Errorf("expected 'has no test files' notice, got: %s", out)
	}
	if !strings.Contains(out, "SURVIVED") {
		t.Errorf("expected SURVIVED marker, got: %s", out)
	}
}

// TestRun_BaselineFailure covers F4: if tests fail without any mutation, the
// run must abort with ErrBaseline and never run mutants. Before the fix, the
// always-failing test made every mutant a false KILLED / Score 100% / exit 0.
func TestRun_BaselineFailure(t *testing.T) {
	tmpDir := t.TempDir()
	writeFiles(t, tmpDir, map[string]string{
		"go.mod":         "module example.com/failmod\n\ngo 1.21\n",
		"failing/lib.go": "package failing\n\nfunc Positive(n int) bool { return n > 0 }\n",
		"failing/lib_test.go": `package failing

import "testing"

func TestAlwaysFails(t *testing.T) {
	t.Fatal("this test is broken and fails regardless of mutations")
}
`,
	})

	chdir(t, tmpDir)

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./failing"},
		Workers:  1,
		Timeout:  30 * time.Second,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	requireIs(t, err, ErrBaseline)

	out := stdout.String()
	if strings.Contains(out, "Mutation Testing Summary") {
		t.Errorf("baseline failure must not print a summary, got: %s", out)
	}
	if strings.Contains(out, "--- KILLED:") || strings.Contains(out, "--- SURVIVED:") {
		t.Errorf("baseline failure must not run mutants, got: %s", out)
	}
}

// TestBaselineErr covers the classification of VerifyBaseline failures: a
// genuine failure is ErrBaseline, but a failure observed after the run
// context was canceled (e.g. SIGINT during the baseline run) must be
// reported as ErrInterrupted — the user's tests are not actually broken.
func TestBaselineErr(t *testing.T) {
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		err     error
		want    error
		notWant error
	}{
		{
			name:    "genuine failure is ErrBaseline",
			ctx:     context.Background(),
			err:     errors.New("package p: baseline tests failed"),
			want:    ErrBaseline,
			notWant: ErrInterrupted,
		},
		{
			name:    "canceled ctx is ErrInterrupted",
			ctx:     canceledCtx,
			err:     errors.New("signal: killed"),
			want:    ErrInterrupted,
			notWant: ErrBaseline,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := baselineErr(tt.ctx, tt.err)
			requireIs(t, got, tt.want)
			if errors.Is(got, tt.notWant) {
				t.Errorf("baselineErr() = %v, must not match %v", got, tt.notWant)
			}
		})
	}
}

// TestRun_AllKilled_ReturnsNil asserts the success path of the default mode:
// when every mutant is killed (Survived == 0 and Errors == 0), run() must
// return nil. This pins the exact `> 0` boundaries of the exit condition and
// that a package with tests is never misflagged as having none.
func TestRun_AllKilled_ReturnsNil(t *testing.T) {
	tmpDir := t.TempDir()
	writeFiles(t, tmpDir, map[string]string{
		"go.mod": "module example.com/allkilled\n\ngo 1.21\n",
		"lib.go": "package allkilled\n\nfunc Threshold(n int) bool { return n > 10 }\n",
		"lib_test.go": `package allkilled

import "testing"

func TestThreshold(t *testing.T) {
	// Full boundary coverage: 11 vs 10 kills the > to >= mutant.
	if !Threshold(11) || Threshold(10) {
		t.Fatal("wrong")
	}
}
`,
	})

	chdir(t, tmpDir)

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./..."},
		Workers:  1,
		Timeout:  30 * time.Second,
		JSON:     true,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected nil error when all mutants are killed, got %v\nstderr: %s", err, stderr.String())
	}

	var summary output.JSONSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if summary.Total != 1 || summary.Killed != 1 {
		t.Errorf("expected total=1 killed=1, got total=%d killed=%d", summary.Total, summary.Killed)
	}
	if summary.Survived != 0 || summary.Errors != 0 {
		t.Errorf("expected survived=0 errors=0, got survived=%d errors=%d", summary.Survived, summary.Errors)
	}
}

// TestRun_GoVersionTooOld covers F9: a target module whose go directive is
// below 1.20 must fail fast with a clear diagnostic before instrumentation
// or build, instead of surfacing a confusing compiler error deep inside
// mutest's generated helpers. mutest's equality helper is instantiated as
// `_mutest_eq_N[T comparable](a, b T)`; comparing two `error` interface
// values (as below) reproduces the "interface satisfies comparable"
// requirement that only compiles under go1.20+, so before this fix the
// build itself fails under go1.19.
func TestRun_GoVersionTooOld(t *testing.T) {
	tmpDir := t.TempDir()
	writeFiles(t, tmpDir, map[string]string{
		"go.mod": "module example.com/go119\n\ngo 1.19\n",
		"pkg/lib.go": `package pkg

import "io"

func IsEOF(err error) bool { return err == io.EOF }
`,
		"pkg/lib_test.go": `package pkg

import (
	"io"
	"testing"
)

func TestIsEOF(t *testing.T) {
	if !IsEOF(io.EOF) || IsEOF(nil) {
		t.Fatal("wrong")
	}
}
`,
	})

	chdir(t, tmpDir)

	var stdout, stderr bytes.Buffer
	cfg := config.Config{
		Patterns: []string{"./pkg"},
		Workers:  1,
		Timeout:  30 * time.Second,
	}

	err := run(context.Background(), cfg, &stdout, &stderr)
	requireIs(t, err, ErrDiscovery)
	if errors.Is(err, ErrBuild) {
		t.Fatalf("expected a preflight go-version error, not a build error: %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "go directive") {
		t.Errorf("expected error to mention 'go directive', got: %v", msg)
	}
	if !strings.Contains(msg, "1.20") {
		t.Errorf("expected error to mention the minimum version '1.20', got: %v", msg)
	}
}

// TestRun_GoVersionOK_Regression covers F9's regression requirement: modules
// at or above the 1.20 minimum must be completely unaffected by the new
// preflight check (no false rejections).
func TestRun_GoVersionOK_Regression(t *testing.T) {
	for _, goVersion := range []string{"1.20", "1.21", "1.24"} {
		t.Run("go "+goVersion, func(t *testing.T) {
			tmpDir := t.TempDir()
			writeFiles(t, tmpDir, map[string]string{
				"go.mod": "module example.com/vercheck\n\ngo " + goVersion + "\n",
				"lib.go": "package vercheck\n\nfunc Threshold(n int) bool { return n > 10 }\n",
				"lib_test.go": `package vercheck

import "testing"

func TestThreshold(t *testing.T) {
	if !Threshold(11) || Threshold(10) {
		t.Fatal("wrong")
	}
}
`,
			})

			chdir(t, tmpDir)

			var stdout, stderr bytes.Buffer
			cfg := config.Config{
				Patterns: []string{"./..."},
				Workers:  1,
				Timeout:  30 * time.Second,
				DryRun:   true,
			}

			if err := run(context.Background(), cfg, &stdout, &stderr); err != nil {
				t.Fatalf("expected nil error for go %s (>= 1.20 minimum), got %v\nstderr: %s", goVersion, err, stderr.String())
			}
			if !strings.Contains(stdout.String(), "mutation points") {
				t.Errorf("expected dry-run output listing mutation points, got: %s", stdout.String())
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
