package engine

import (
	"encoding/json"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fchimpan/mutest/mutator"
)

func testProjectDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "testdata", "project"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// chdir changes to the given directory for the duration of the test.
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

func TestDiscoverAll(t *testing.T) {
	chdir(t, testProjectDir(t))
	eng := New([]string{"./..."}, &mutator.ComparisonMutator{})

	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}

	if len(points) != 4 {
		t.Errorf("expected 4 mutation points, got %d", len(points))
	}

	for i, p := range points {
		if p.File == "" {
			t.Errorf("point[%d]: empty file path", i)
		}
		if !filepath.IsAbs(p.File) {
			t.Errorf("point[%d]: expected absolute path, got %s", i, p.File)
		}
		if p.Line == 0 {
			t.Errorf("point[%d]: zero line number", i)
		}
		if p.Desc == "" {
			t.Errorf("point[%d]: empty description", i)
		}
		if p.Package != "testproject" {
			t.Errorf("point[%d]: expected package testproject, got %s", i, p.Package)
		}
		if p.ImportPath == "" {
			t.Errorf("point[%d]: empty import path", i)
		}
	}
}

func TestDiscoverAll_NoMutators(t *testing.T) {
	chdir(t, testProjectDir(t))
	eng := New([]string{"./..."})

	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 0 {
		t.Errorf("expected 0 points with no mutators, got %d", len(points))
	}
}

func TestDiscoverAll_InvalidPattern(t *testing.T) {
	eng := New([]string{"./nonexistent_package_xyz"}, &mutator.ComparisonMutator{})

	_, err := eng.DiscoverAll()
	if err == nil {
		t.Error("expected error for nonexistent package pattern")
	}
}

func TestDiscoverAll_SkipsTestFiles(t *testing.T) {
	chdir(t, testProjectDir(t))
	eng := New([]string{"./..."}, &mutator.ComparisonMutator{})

	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range points {
		if strings.HasSuffix(p.File, "_test.go") {
			t.Errorf("should not discover mutations in test files: %s", p.File)
		}
	}
}

func TestPrepareAndCleanup(t *testing.T) {
	chdir(t, testProjectDir(t))
	compMut := &mutator.ComparisonMutator{}
	eng := New([]string{"./..."}, compMut)

	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(points) == 0 {
		t.Fatal("no mutation points found")
	}

	m, err := eng.Prepare(compMut, points[0])
	if err != nil {
		t.Fatal(err)
	}

	// Verify temp files exist
	if _, err := os.Stat(m.OverlayPath); err != nil {
		t.Errorf("overlay file should exist: %v", err)
	}
	mutatedPath := filepath.Join(m.TempDir, "mutated.go")
	if _, err := os.Stat(mutatedPath); err != nil {
		t.Errorf("mutated file should exist: %v", err)
	}

	// Verify overlay JSON structure
	data, err := os.ReadFile(m.OverlayPath)
	if err != nil {
		t.Fatal(err)
	}
	var overlay Overlay
	if err := json.Unmarshal(data, &overlay); err != nil {
		t.Fatal(err)
	}
	if len(overlay.Replace) != 1 {
		t.Errorf("expected 1 replacement in overlay, got %d", len(overlay.Replace))
	}
	for orig, repl := range overlay.Replace {
		if orig != points[0].File {
			t.Errorf("overlay key should be %s, got %s", points[0].File, orig)
		}
		if !strings.Contains(repl, "mutest-") {
			t.Errorf("overlay value should be in temp dir, got %s", repl)
		}
	}

	// Verify mutated source is valid Go
	mutatedSrc, err := os.ReadFile(mutatedPath)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "mutated.go", mutatedSrc, 0)
	if err != nil {
		t.Errorf("mutated file should be valid Go: %v", err)
	}

	// Verify mutation was applied
	origSrc, err := os.ReadFile(points[0].File)
	if err != nil {
		t.Fatal(err)
	}
	origFormatted, _ := format.Source(origSrc)
	if string(mutatedSrc) == string(origFormatted) {
		t.Error("mutated source should differ from original")
	}

	// Cleanup
	eng.Cleanup(m)
	if _, err := os.Stat(m.TempDir); !os.IsNotExist(err) {
		t.Error("temp dir should be removed after cleanup")
	}
}

func TestPrepare_InvalidFile(t *testing.T) {
	compMut := &mutator.ComparisonMutator{}
	eng := New([]string{"./..."}, compMut)

	bogusPoint := mutator.MutationPoint{
		File:   "/nonexistent/file.go",
		NodeID: 0,
	}

	_, err := eng.Prepare(compMut, bogusPoint)
	if err == nil {
		t.Error("expected error for nonexistent source file")
	}
}

func TestPrepare_AllPoints(t *testing.T) {
	chdir(t, testProjectDir(t))
	compMut := &mutator.ComparisonMutator{}
	eng := New([]string{"./..."}, compMut)

	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}

	for i, pt := range points {
		m, err := eng.Prepare(compMut, pt)
		if err != nil {
			t.Errorf("point[%d]: Prepare failed: %v", i, err)
			continue
		}
		mutatedSrc, err := os.ReadFile(filepath.Join(m.TempDir, "mutated.go"))
		if err != nil {
			t.Errorf("point[%d]: read mutated file: %v", i, err)
		}
		if _, err := parser.ParseFile(token.NewFileSet(), "", mutatedSrc, 0); err != nil {
			t.Errorf("point[%d]: mutated file not valid Go: %v", i, err)
		}
		eng.Cleanup(m)
	}
}
