package engine

import (
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

	if len(points) == 0 {
		t.Error("expected mutation points, got 0")
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

func TestDiscoverAll_SkipDirective_Function(t *testing.T) {
	tmpDir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/skip\n\ngo 1.21\n",
		"skip.go": `package skip

//mutest:skip
func Skipped(a, b int) bool {
	return a > b
}

func NotSkipped(a, b int) bool {
	return a < b
}
`,
	} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, tmpDir)

	eng := New([]string{"./..."}, &mutator.ComparisonMutator{})
	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 {
		t.Errorf("expected 1 point (skipped function excluded), got %d", len(points))
	}
	if len(points) > 0 && points[0].Desc != "< to <=" {
		t.Errorf("expected remaining point to be '<', got %q", points[0].Desc)
	}
}

func TestDiscoverAll_SkipDirective_Line(t *testing.T) {
	tmpDir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/skip\n\ngo 1.21\n",
		"skip.go": `package skip

func Mixed(a, b int) bool {
	if a > b { //mutest:skip
		return true
	}
	return a < b
}
`,
	} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, tmpDir)

	eng := New([]string{"./..."}, &mutator.ComparisonMutator{})
	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 {
		t.Errorf("expected 1 point (skipped line excluded), got %d", len(points))
	}
	if len(points) > 0 && points[0].Desc != "< to <=" {
		t.Errorf("expected remaining point to be '<', got %q", points[0].Desc)
	}
}

func TestDiscoverAll_SkipDirective_SpaceVariant(t *testing.T) {
	tmpDir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/skip\n\ngo 1.21\n",
		"skip.go": `package skip

// mutest:skip
func Skipped(a, b int) bool {
	return a > b
}
`,
	} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, tmpDir)

	eng := New([]string{"./..."}, &mutator.ComparisonMutator{})
	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 0 {
		t.Errorf("expected 0 points (function with space variant skipped), got %d", len(points))
	}
}

func TestDiscoverAll_SkipDirective_Block(t *testing.T) {
	tmpDir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/skip\n\ngo 1.21\n",
		"skip.go": `package skip

func Block(a, b, c int) bool {
	if a > b { //mutest:skip
		if b > c {
			return true
		}
		return a > c
	}
	return a < b
}
`,
	} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, tmpDir)

	eng := New([]string{"./..."}, &mutator.ComparisonMutator{})
	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	// Only "a < b" on the last line should remain; the if block (a > b, b > c, a > c) is skipped.
	if len(points) != 1 {
		t.Errorf("expected 1 point (block-skipped), got %d", len(points))
		for _, p := range points {
			t.Logf("  %s:%d %s", p.File, p.Line, p.Desc)
		}
	}
	if len(points) > 0 && points[0].Desc != "< to <=" {
		t.Errorf("expected remaining point to be '<', got %q", points[0].Desc)
	}
}

func TestDiscoverAll_SkipDirective_ForBlock(t *testing.T) {
	tmpDir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/skip\n\ngo 1.21\n",
		"skip.go": `package skip

func ForBlock(items []int) bool {
	for i := 0; i < len(items); i++ { //mutest:skip
		if items[i] > 0 {
			return true
		}
	}
	return len(items) > 1
}
`,
	} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, tmpDir)

	eng := New([]string{"./..."}, &mutator.ComparisonMutator{})
	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	// Only "len(items) > 1" should remain; the for block (i < len, items[i] > 0) is skipped.
	if len(points) != 1 {
		t.Errorf("expected 1 point (for-block skipped), got %d", len(points))
		for _, p := range points {
			t.Logf("  %s:%d %s", p.File, p.Line, p.Desc)
		}
	}
}

func TestDiscoverAll_SkipDirective_IfElseBlock(t *testing.T) {
	tmpDir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/skip\n\ngo 1.21\n",
		"skip.go": `package skip

func IfElse(a, b int) int {
	if a > b { //mutest:skip
		return a
	} else if a < b {
		return b
	}
	return 0
}
`,
	} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, tmpDir)

	eng := New([]string{"./..."}, &mutator.ComparisonMutator{})
	points, err := eng.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	// The entire if/else if chain is one ast.IfStmt — all should be skipped.
	if len(points) != 0 {
		t.Errorf("expected 0 points (if-else block skipped), got %d", len(points))
		for _, p := range points {
			t.Logf("  %s:%d %s", p.File, p.Line, p.Desc)
		}
	}
}
