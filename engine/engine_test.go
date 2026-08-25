package engine

import (
	"os"
	"os/exec"
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

// TestDiscoverAll_InvalidPattern_IncludesGoDiagnostic covers F13:
// resolveFiles must surface `go list`'s own stderr diagnostic instead of the
// bare "exit status 1" that *exec.ExitError.Error() returns on its own. The
// expected substring is computed by shelling out to `go list` the same way
// resolveFiles does, so the assertion tracks the real diagnostic instead of
// hardcoding a message that could drift across Go versions.
func TestDiscoverAll_InvalidPattern_IncludesGoDiagnostic(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/badpattern\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	chdir(t, tmpDir)

	const pattern = "./nonexistent_package_xyz"

	cmd := exec.Command("go", "list", "-json", pattern)
	_, rawErr := cmd.Output()
	exitErr, ok := rawErr.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected `go list` to fail with an ExitError for a bad pattern, got %v (%T)", rawErr, rawErr)
	}
	wantSubstr := strings.TrimSpace(string(exitErr.Stderr))
	if wantSubstr == "" {
		t.Fatal("expected go list to write a diagnostic to stderr")
	}

	eng := New([]string{pattern}, &mutator.ComparisonMutator{})
	_, err := eng.DiscoverAll()
	if err == nil {
		t.Fatal("expected error for nonexistent package pattern")
	}
	if err.Error() == "exit status 1" {
		t.Fatal("error must include go's diagnostic, not just the bare exit status")
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("expected error to contain go list's diagnostic %q, got: %v", wantSubstr, err)
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

// TestDiscoverAll_SkipsConstDeclarations verifies that comparison/equality
// operators inside `const` declarations are never discovered as mutation
// points. Instrumenting a const expression turns it into a helper function
// call, which is not a constant expression and fails to build. Cases cover
// package-level single const, block-form `const (...)`, and function-local
// const; the last case is a control asserting that a runtime comparison on
// the line immediately after a const declaration is NOT swept up by the
// skip range.
func TestDiscoverAll_SkipsConstDeclarations(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantLines []int
		wantDescs []string
	}{
		{
			name: "package-level single const",
			src: `package constskip

const MaxSize = 100
const IsBig = MaxSize > 10

func Half(n int) int {
	if n < 2 {
		return 0
	}
	return n / 2
}
`,
			wantLines: []int{7},
			wantDescs: []string{"< to <="},
		},
		{
			name: "block-form const",
			src: `package constskip

const (
	A = 1 > 0
	B = 2 == 2
)

func Check(n int) bool {
	return n < 5
}
`,
			wantLines: []int{9},
			wantDescs: []string{"< to <="},
		},
		{
			name: "function-local const",
			src: `package constskip

func Compute(n int) int {
	const ok = 1 <= 2
	if ok {
		return n
	}
	return n + 1
}

func Runtime(n int) bool {
	return n >= 3
}
`,
			wantLines: []int{12},
			wantDescs: []string{">= to >"},
		},
		{
			name: "control: runtime comparison right after a const decl is not skipped",
			src: `package constskip

const IsBig = 100 > 10
func RuntimeCheck(n int) bool { return n > 5 }
`,
			wantLines: []int{4},
			wantDescs: []string{"> to >="},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for name, content := range map[string]string{
				"go.mod": "module example.com/constskip\n\ngo 1.21\n",
				"lib.go": tc.src,
			} {
				if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			chdir(t, tmpDir)

			eng := New([]string{"./..."}, &mutator.ComparisonMutator{}, &mutator.EqualityMutator{})
			points, err := eng.DiscoverAll()
			if err != nil {
				t.Fatal(err)
			}

			if len(points) != len(tc.wantLines) {
				t.Fatalf("expected %d point(s), got %d: %+v", len(tc.wantLines), len(points), points)
			}
			for i, wantLine := range tc.wantLines {
				if points[i].Line != wantLine {
					t.Errorf("point[%d]: expected line %d, got %d", i, wantLine, points[i].Line)
				}
				if points[i].Desc != tc.wantDescs[i] {
					t.Errorf("point[%d]: expected desc %q, got %q", i, tc.wantDescs[i], points[i].Desc)
				}
			}
		})
	}
}

// TestCheckGoVersion covers F9: the target module's go directive must be
// >= 1.20, mutest's published floor. A module with no reported GoVersion
// (e.g. GOPATH mode) must be skipped, not rejected.
func TestCheckGoVersion(t *testing.T) {
	tests := []struct {
		name    string
		mod     *goModule
		wantErr bool
	}{
		{"nil module (GOPATH mode) is skipped", nil, false},
		{"empty GoVersion is skipped", &goModule{Path: "example.com/gopath", GoVersion: ""}, false},
		{"below minimum 1.19 is rejected", &goModule{Path: "example.com/old", GoVersion: "1.19"}, true},
		{"at minimum 1.20 is accepted", &goModule{Path: "example.com/atmin", GoVersion: "1.20"}, false},
		{"above minimum 1.24 is accepted", &goModule{Path: "example.com/new", GoVersion: "1.24"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkGoVersion(tt.mod)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if !strings.Contains(err.Error(), "go directive") {
					t.Errorf("expected error to mention 'go directive', got: %v", err)
				}
				if !strings.Contains(err.Error(), "1.20") {
					t.Errorf("expected error to mention the minimum version '1.20', got: %v", err)
				}
				if !strings.Contains(err.Error(), tt.mod.Path) {
					t.Errorf("expected error to mention the module path %q, got: %v", tt.mod.Path, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

// TestDiscoverAll_SkipDirective_StandaloneComment covers F15: a
// //mutest:skip comment placed alone on its own line (nothing but
// whitespace before it) applies to the line that follows — and to the whole
// block if that following line begins one (if/for/switch/select) — mirroring
// the most natural misuse of the directive. A comment at the end of a line
// of code (the previously documented, and only working, form) must keep
// skipping only that line: the standalone case must not cause it to also
// swallow the next line.
func TestDiscoverAll_SkipDirective_StandaloneComment(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantDescs []string // Desc of each surviving (non-skipped) point, in order.
	}{
		{
			name: "standalone line skips only the following line",
			src: `package skip

func Mixed(a, b int) bool {
	//mutest:skip
	x := a > b
	return x || a < b
}
`,
			wantDescs: []string{"< to <="},
		},
		{
			name: "standalone line before an if skips the whole block",
			src: `package skip

func Mixed(a, b, c int) bool {
	//mutest:skip
	if a > b {
		return b > c
	}
	return a < b
}
`,
			wantDescs: []string{"< to <="},
		},
		{
			name: "end-of-line comment still skips only its own line",
			src: `package skip

func Mixed(a, b int) bool {
	x := a > b //mutest:skip
	return x || a < b
}
`,
			wantDescs: []string{"< to <="},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for name, content := range map[string]string{
				"go.mod":  "module example.com/skip\n\ngo 1.21\n",
				"skip.go": tc.src,
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
			if len(points) != len(tc.wantDescs) {
				t.Fatalf("expected %d point(s), got %d: %+v", len(tc.wantDescs), len(points), points)
			}
			for i, want := range tc.wantDescs {
				if points[i].Desc != want {
					t.Errorf("point[%d]: expected desc %q, got %q", i, want, points[i].Desc)
				}
			}
		})
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
