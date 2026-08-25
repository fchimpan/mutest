package engine

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fchimpan/mutest/mutator"
)

func mustParseInstrumented(t *testing.T, out []byte) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), "instrumented.go", out, 0); err != nil {
		t.Fatalf("instrumented output does not parse: %v\n%s", err, out)
	}
}

func TestInstrumentFile_NestedBinaryExpr(t *testing.T) {
	// (a > b) == flag: both > and == should be instrumented.
	// The inner > call is embedded in the outer == call's LHS.
	src := []byte(`package repro

func Foo(a, b int, flag bool) bool {
	return (a > b) == flag
}
`)

	// ast.Inspect pre-order: outer == (nodeID=0), inner > (nodeID=1)
	points := []mutator.MutationPoint{
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   0,
			Original: token.EQL,
			Mutated:  token.NEQ,
			MutestID: 2,
			Desc:     "== to !=",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   1,
			Original: token.GTR,
			Mutated:  token.GEQ,
			MutestID: 1,
			Desc:     "> to >=",
		},
	}

	out, helpers, err := instrumentFile(src, "repro.go", points)
	if err != nil {
		t.Fatalf("instrumentFile: %v", err)
	}

	result := string(out)
	mustParseInstrumented(t, out)

	// Both mutations should be instrumented, the inner > call nested inside
	// the outer == flip.
	if !strings.Contains(result, "((_mutest_cmp_1(a, b)) == flag) != _mutest_on(2)") {
		t.Errorf("expected inner > call nested in outer == flip, got:\n%s", result)
	}

	if len(helpers) != 1 {
		t.Errorf("expected 1 cmp helper, got %d", len(helpers))
	}
}

func TestInstrumentFile_DoubleNestedNil(t *testing.T) {
	// (x != nil) == (y != nil): two != nested inside ==.
	// All three mutations should be instrumented.
	src := []byte(`package repro

func Bar(x, y *int) bool {
	return (x != nil) == (y != nil)
}
`)

	// ast.Inspect pre-order: outer == (0), left != (1), right != (2)
	points := []mutator.MutationPoint{
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   0,
			Original: token.EQL,
			Mutated:  token.NEQ,
			MutestID: 2,
			Desc:     "== to !=",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   1,
			Original: token.NEQ,
			Mutated:  token.EQL,
			MutestID: 1,
			Desc:     "!= to ==",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   2,
			Original: token.NEQ,
			Mutated:  token.EQL,
			MutestID: 3,
			Desc:     "!= to ==",
		},
	}

	out, helpers, err := instrumentFile(src, "repro.go", points)
	if err != nil {
		t.Fatalf("instrumentFile: %v", err)
	}

	result := string(out)
	mustParseInstrumented(t, out)

	// All three flips should be present, each ID bound to its own site.
	want := "(((x != nil) != _mutest_on(1)) == ((y != nil) != _mutest_on(3))) != _mutest_on(2)"
	if !strings.Contains(result, want) {
		t.Errorf("expected %s, got:\n%s", want, result)
	}

	if len(helpers) != 0 {
		t.Errorf("expected 0 cmp helpers, got %d", len(helpers))
	}
}

func TestInstrumentFile_TripleNested(t *testing.T) {
	// ((a > b) == flag) != expected: three levels of nesting.
	src := []byte(`package repro

func Baz(a, b int, flag, expected bool) bool {
	return ((a > b) == flag) != expected
}
`)

	// ast.Inspect pre-order: != (0), == (1), > (2)
	points := []mutator.MutationPoint{
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   0,
			Original: token.NEQ,
			Mutated:  token.EQL,
			MutestID: 3,
			Desc:     "!= to ==",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   1,
			Original: token.EQL,
			Mutated:  token.NEQ,
			MutestID: 2,
			Desc:     "== to !=",
		},
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   2,
			Original: token.GTR,
			Mutated:  token.GEQ,
			MutestID: 1,
			Desc:     "> to >=",
		},
	}

	out, helpers, err := instrumentFile(src, "repro.go", points)
	if err != nil {
		t.Fatalf("instrumentFile: %v", err)
	}

	result := string(out)
	mustParseInstrumented(t, out)

	// All three mutations should be present, nested innermost to outermost.
	want := "((((_mutest_cmp_1(a, b)) == flag) != _mutest_on(2)) != expected) != _mutest_on(3)"
	if !strings.Contains(result, want) {
		t.Errorf("expected %s, got:\n%s", want, result)
	}

	if len(helpers) != 1 {
		t.Errorf("expected 1 cmp helper, got %d", len(helpers))
	}
}

// TestInstrumentFile_MixedTypeEquality covers issue #39: `any == error` is
// legal Go, but a one-type-parameter generic helper cannot infer T from
// operands of different static types, so the instrumented package failed
// to build. The flip form keeps the original comparison untouched.
func TestInstrumentFile_MixedTypeEquality(t *testing.T) {
	src := []byte(`package repro

import "net/http"

func IsAbort(r any) bool {
	return r == http.ErrAbortHandler
}
`)

	points := []mutator.MutationPoint{
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   0,
			Original: token.EQL,
			Mutated:  token.NEQ,
			MutestID: 1,
			Desc:     "== to !=",
		},
	}

	out, helpers, err := instrumentFile(src, "repro.go", points)
	if err != nil {
		t.Fatalf("instrumentFile: %v", err)
	}

	result := string(out)
	mustParseInstrumented(t, out)

	if strings.Contains(result, "_mutest_eq_") {
		t.Errorf("equality must not use a generic helper (breaks mixed-type inference), got:\n%s", result)
	}
	if !strings.Contains(result, "(r == http.ErrAbortHandler) != _mutest_on(1)") {
		t.Errorf("expected flip preserving the original comparison, got:\n%s", result)
	}
	if len(helpers) != 0 {
		t.Errorf("expected 0 cmp helpers, got %+v", helpers)
	}
}

// TestInstrumentFile_RecoverOperand pins the flip form for position-sensitive
// operands: recover() stops a panic only when called directly by a deferred
// function, so instrumentation must not move the comparison into a nested
// function literal.
func TestInstrumentFile_RecoverOperand(t *testing.T) {
	src := []byte(`package repro

func Catch(sentinel any, f func()) (caught bool) {
	defer func() {
		if recover() == sentinel {
			caught = true
		}
	}()
	f()
	return
}
`)

	points := []mutator.MutationPoint{
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   0,
			Original: token.EQL,
			Mutated:  token.NEQ,
			MutestID: 1,
			Desc:     "== to !=",
		},
	}

	out, _, err := instrumentFile(src, "repro.go", points)
	if err != nil {
		t.Fatalf("instrumentFile: %v", err)
	}

	result := string(out)
	mustParseInstrumented(t, out)

	if !strings.Contains(result, "if (recover() == sentinel) != _mutest_on(1) {") {
		t.Errorf("expected recover() to stay in the deferred function's frame, got:\n%s", result)
	}
}

// TestGenerateRuntime_MutationSwitch pins the two runtime properties call
// sites depend on: _mutest_on must read MUTEST_ID via _mutest_init before
// comparing, and cmp must not be imported without cmp helpers (an unused
// import would fail the build of every equality-only package).
func TestGenerateRuntime_MutationSwitch(t *testing.T) {
	runtime := string(generateRuntime("repro", nil))

	if !strings.Contains(runtime, "func _mutest_on(id int) bool {\n\t_mutest_init()\n\treturn _mutest_active == id\n}") {
		t.Errorf("expected initializing _mutest_on, got:\n%s", runtime)
	}
	if strings.Contains(runtime, `"cmp"`) {
		t.Errorf("cmp must not be imported without cmp helpers, got:\n%s", runtime)
	}
}

// TestBuildTestBinary_NoTestsDetection covers the NoTests detection: `go test
// -c` exits 0 without producing a binary when the package has no test files.
// BuildTestBinary must flag such packages via os.Stat (NoTests=true, no
// BinaryPath) and must leave packages WITH tests fully built.
func TestBuildTestBinary_NoTestsDetection(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantNoTests bool
	}{
		{
			name: "package with tests builds a binary",
			files: map[string]string{
				"go.mod":      "module example.com/buildbin\n\ngo 1.21\n",
				"lib.go":      "package buildbin\n\nfunc Positive(n int) bool { return n > 0 }\n",
				"lib_test.go": "package buildbin\n\nimport \"testing\"\n\nfunc TestPositive(t *testing.T) { if !Positive(1) || Positive(0) { t.Fail() } }\n",
			},
			wantNoTests: false,
		},
		{
			name: "package without tests is flagged NoTests",
			files: map[string]string{
				"go.mod": "module example.com/buildbin\n\ngo 1.21\n",
				"lib.go": "package buildbin\n\nfunc Positive(n int) bool { return n > 0 }\n",
			},
			wantNoTests: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for name, content := range tt.files {
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
			pkgs, err := eng.InstrumentAll(points)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { CleanupInstrumented(pkgs) })
			if len(pkgs) != 1 {
				t.Fatalf("expected 1 instrumented package, got %d", len(pkgs))
			}

			for _, pkg := range pkgs {
				if err := eng.BuildTestBinary(context.Background(), pkg); err != nil {
					t.Fatalf("BuildTestBinary: %v", err)
				}
				if pkg.NoTests != tt.wantNoTests {
					t.Errorf("NoTests = %v, want %v", pkg.NoTests, tt.wantNoTests)
				}
				if tt.wantNoTests {
					if pkg.BinaryPath != "" {
						t.Errorf("BinaryPath = %q, want empty for a package without tests", pkg.BinaryPath)
					}
				} else {
					if pkg.BinaryPath == "" {
						t.Fatal("BinaryPath is empty for a package with tests")
					}
					if _, err := os.Stat(pkg.BinaryPath); err != nil {
						t.Errorf("test binary not found at BinaryPath: %v", err)
					}
				}
			}
		})
	}
}

// TestInstrumentAll_ExistingRuntimeFileErrors covers F14: instrumentPackage
// injects its generated runtime helpers at the virtual path
// pkgDir/mutest_runtime.go via the overlay. If a file with that exact name
// already exists in the package, the overlay would silently replace it
// (dropping whatever the user's file declared) instead of failing loudly.
// InstrumentAll must return an explicit error naming the collision instead.
func TestInstrumentAll_ExistingRuntimeFileErrors(t *testing.T) {
	tmpDir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":            "module example.com/collide\n\ngo 1.21\n",
		"lib.go":            "package collide\n\nfunc Positive(n int) bool { return n > 0 }\n",
		"mutest_runtime.go": "package collide\n\n// userDefined is a real file that happens to share mutest's\n// generated-runtime filename.\nfunc userDefined() int { return 42 }\n",
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
	if len(points) == 0 {
		t.Fatal("expected at least 1 mutation point")
	}

	pkgs, err := eng.InstrumentAll(points)
	if err == nil {
		t.Cleanup(func() { CleanupInstrumented(pkgs) })
		t.Fatal("expected an error because mutest_runtime.go already exists in the package")
	}
	if !strings.Contains(err.Error(), "mutest_runtime.go") {
		t.Errorf("expected error to mention mutest_runtime.go, got: %v", err)
	}
}

func TestInstrumentFile_NoNesting(t *testing.T) {
	// a > b without nesting should still work as before.
	src := []byte(`package repro

func Simple(a, b int) bool {
	return a > b
}
`)

	points := []mutator.MutationPoint{
		{
			File:     "repro.go",
			Package:  "repro",
			NodeID:   0,
			Original: token.GTR,
			Mutated:  token.GEQ,
			MutestID: 1,
			Desc:     "> to >=",
		},
	}

	out, helpers, err := instrumentFile(src, "repro.go", points)
	if err != nil {
		t.Fatalf("instrumentFile: %v", err)
	}

	result := string(out)
	if !strings.Contains(result, "_mutest_cmp_1(a, b)") {
		t.Errorf("expected simple replacement, got:\n%s", result)
	}
	if len(helpers) != 1 {
		t.Errorf("expected 1 helper, got %d", len(helpers))
	}
}
