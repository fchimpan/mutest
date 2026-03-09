package analysis

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCoverProfile(t *testing.T, dir string, content string) string {
	t.Helper()
	path := filepath.Join(dir, "cover.prof")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing cover profile: %v", err)
	}
	return path
}

func TestParseCoverProfile_Basic(t *testing.T) {
	dir := t.TempDir()
	// Create a dummy file so resolvePackageFile finds it
	srcDir := filepath.Join(dir, "pkg")
	os.MkdirAll(srcDir, 0o755)
	srcFile := filepath.Join(srcDir, "foo.go")
	os.WriteFile(srcFile, []byte("package pkg"), 0o644)

	profile := `mode: set
pkg/foo.go:10.1,15.2 3 1
pkg/foo.go:20.1,25.2 2 0
`
	path := writeCoverProfile(t, dir, profile)
	cm, err := parseCoverProfile(path, dir)
	if err != nil {
		t.Fatalf("parseCoverProfile() error: %v", err)
	}

	// Lines 10-15 should be covered (count=1)
	for line := 10; line <= 15; line++ {
		if !cm.IsCovered(srcFile, line) {
			t.Errorf("line %d should be covered", line)
		}
	}

	// Lines 20-25 should NOT be covered (count=0)
	for line := 20; line <= 25; line++ {
		if cm.IsCovered(srcFile, line) {
			t.Errorf("line %d should not be covered (count=0)", line)
		}
	}
}

func TestParseCoverProfile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	profile := `mode: set
`
	path := writeCoverProfile(t, dir, profile)
	cm, err := parseCoverProfile(path, dir)
	if err != nil {
		t.Fatalf("parseCoverProfile() error: %v", err)
	}
	if cm.IsCovered("/any/file.go", 1) {
		t.Error("empty profile should not mark anything as covered")
	}
}

func TestParseCoverProfile_MultipleCounts(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "pkg")
	os.MkdirAll(srcDir, 0o755)
	srcFile := filepath.Join(srcDir, "foo.go")
	os.WriteFile(srcFile, []byte("package pkg"), 0o644)

	profile := `mode: count
pkg/foo.go:5.1,10.2 3 5
pkg/foo.go:12.1,14.2 1 100
`
	path := writeCoverProfile(t, dir, profile)
	cm, err := parseCoverProfile(path, dir)
	if err != nil {
		t.Fatalf("parseCoverProfile() error: %v", err)
	}

	// All lines in both ranges should be covered
	for line := 5; line <= 10; line++ {
		if !cm.IsCovered(srcFile, line) {
			t.Errorf("line %d should be covered", line)
		}
	}
	for line := 12; line <= 14; line++ {
		if !cm.IsCovered(srcFile, line) {
			t.Errorf("line %d should be covered", line)
		}
	}
	// Gap should not be covered
	if cm.IsCovered(srcFile, 11) {
		t.Error("line 11 (gap) should not be covered")
	}
}

func TestParseCoverProfile_NonexistentFile(t *testing.T) {
	_, err := parseCoverProfile("/nonexistent/cover.prof", "/tmp")
	if err == nil {
		t.Error("expected error for nonexistent cover profile")
	}
}

func TestParseCoverProfile_InvalidLines(t *testing.T) {
	dir := t.TempDir()
	profile := `mode: set
this is a completely invalid line
another invalid line without colon
`
	path := writeCoverProfile(t, dir, profile)
	cm, err := parseCoverProfile(path, dir)
	if err != nil {
		t.Fatalf("parseCoverProfile() should not error on invalid lines: %v", err)
	}
	// Should just skip invalid lines and have empty coverage
	if cm.IsCovered("/any/file.go", 1) {
		t.Error("invalid lines should result in empty coverage")
	}
}

func TestCoverageMap_IsCovered_NilReceiver(t *testing.T) {
	var cm *CoverageMap
	// Nil CoverageMap should return true (assume covered)
	if !cm.IsCovered("/any/file.go", 1) {
		t.Error("nil CoverageMap.IsCovered() should return true")
	}
}

func TestCoverageMap_TestsForLine_NilReceiver(t *testing.T) {
	var cm *CoverageMap
	got := cm.TestsForLine("/any/file.go", 1)
	if got != nil {
		t.Errorf("nil CoverageMap.TestsForLine() should return nil, got %v", got)
	}
}

func TestCoverageMap_TestsForLine_NoData(t *testing.T) {
	cm := &CoverageMap{
		lineTests: make(map[string][]string),
		covered:   make(map[string]bool),
	}
	got := cm.TestsForLine("/any/file.go", 1)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestCoverageMap_IsCovered_UncoveredLine(t *testing.T) {
	cm := &CoverageMap{
		lineTests: make(map[string][]string),
		covered:   make(map[string]bool),
	}
	if cm.IsCovered("/some/file.go", 42) {
		t.Error("uncovered line should return false")
	}
}

func TestParseLineFromPos(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"10.5", 10},
		{"1.1", 1},
		{"999.99", 999},
		{"0.1", 0},       // zero line → invalid
		{"abc.1", 0},     // non-numeric → 0
		{"noperiod", 0},  // no "." separator → 0
		{".5", 0},        // empty before period
		{"", 0},          // empty string
	}
	for _, tt := range tests {
		got := parseLineFromPos(tt.input)
		if got != tt.want {
			t.Errorf("parseLineFromPos(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestResolvePackageFile_DirectMatch(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "internal", "foo")
	os.MkdirAll(srcDir, 0o755)
	srcFile := filepath.Join(srcDir, "bar.go")
	os.WriteFile(srcFile, []byte("package foo"), 0o644)

	got := resolvePackageFile("internal/foo/bar.go", dir)
	if got != srcFile {
		t.Errorf("resolvePackageFile() = %q, want %q", got, srcFile)
	}
}

func TestResolvePackageFile_WithModulePrefix(t *testing.T) {
	dir := t.TempDir()
	// Create go.mod
	goMod := "module github.com/example/mymodule\n\ngo 1.21\n"
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644)

	// Create source file
	srcDir := filepath.Join(dir, "internal", "foo")
	os.MkdirAll(srcDir, 0o755)
	srcFile := filepath.Join(srcDir, "bar.go")
	os.WriteFile(srcFile, []byte("package foo"), 0o644)

	got := resolvePackageFile("github.com/example/mymodule/internal/foo/bar.go", dir)
	if got != srcFile {
		t.Errorf("resolvePackageFile() = %q, want %q", got, srcFile)
	}
}

func TestResolvePackageFile_NoMatch(t *testing.T) {
	dir := t.TempDir()
	// File doesn't exist, no go.mod
	got := resolvePackageFile("nonexistent/file.go", dir)
	// Should return the original path as-is
	if got != "nonexistent/file.go" {
		t.Errorf("resolvePackageFile() = %q, want %q", got, "nonexistent/file.go")
	}
}

func TestReadModulePath(t *testing.T) {
	t.Run("valid go.mod", func(t *testing.T) {
		dir := t.TempDir()
		goMod := "module github.com/example/mymod\n\ngo 1.21\n"
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644)

		got := readModulePath(dir)
		if got != "github.com/example/mymod" {
			t.Errorf("readModulePath() = %q, want %q", got, "github.com/example/mymod")
		}
	})

	t.Run("no go.mod", func(t *testing.T) {
		dir := t.TempDir()
		got := readModulePath(dir)
		if got != "" {
			t.Errorf("readModulePath() = %q, want empty", got)
		}
	})

	t.Run("go.mod without module line", func(t *testing.T) {
		dir := t.TempDir()
		goMod := "go 1.21\n\nrequire golang.org/x/tools v0.1.0\n"
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644)

		got := readModulePath(dir)
		if got != "" {
			t.Errorf("readModulePath() = %q, want empty", got)
		}
	})

	t.Run("module line with extra spaces", func(t *testing.T) {
		dir := t.TempDir()
		goMod := "module   github.com/spaced/module  \n\ngo 1.21\n"
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644)

		got := readModulePath(dir)
		if got != "github.com/spaced/module" {
			t.Errorf("readModulePath() = %q, want %q", got, "github.com/spaced/module")
		}
	})
}

func TestParseCoverProfile_ModulePrefixedPaths(t *testing.T) {
	dir := t.TempDir()

	// Create go.mod
	goMod := "module github.com/example/mymod\n\ngo 1.21\n"
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644)

	// Create source file
	srcDir := filepath.Join(dir, "pkg")
	os.MkdirAll(srcDir, 0o755)
	srcFile := filepath.Join(srcDir, "foo.go")
	os.WriteFile(srcFile, []byte("package pkg"), 0o644)

	profile := `mode: set
github.com/example/mymod/pkg/foo.go:5.1,10.2 3 1
`
	path := writeCoverProfile(t, dir, profile)
	cm, err := parseCoverProfile(path, dir)
	if err != nil {
		t.Fatalf("parseCoverProfile() error: %v", err)
	}

	for line := 5; line <= 10; line++ {
		if !cm.IsCovered(srcFile, line) {
			t.Errorf("line %d should be covered (module-prefixed path)", line)
		}
	}
}

func TestParseCoverProfile_SingleLineRange(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "pkg")
	os.MkdirAll(srcDir, 0o755)
	srcFile := filepath.Join(srcDir, "foo.go")
	os.WriteFile(srcFile, []byte("package pkg"), 0o644)

	profile := `mode: set
pkg/foo.go:7.1,7.20 1 1
`
	path := writeCoverProfile(t, dir, profile)
	cm, err := parseCoverProfile(path, dir)
	if err != nil {
		t.Fatalf("parseCoverProfile() error: %v", err)
	}

	if !cm.IsCovered(srcFile, 7) {
		t.Error("single-line range should mark line 7 as covered")
	}
	if cm.IsCovered(srcFile, 6) {
		t.Error("line 6 should not be covered")
	}
	if cm.IsCovered(srcFile, 8) {
		t.Error("line 8 should not be covered")
	}
}
