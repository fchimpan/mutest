package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fchimpan/mutest/engine"
	"github.com/fchimpan/mutest/mutator"
)

// stubSource is a fake test binary whose behavior is switched by the
// MUTEST_STUB environment variable. It is built once in TestMain and
// exec'd by the runner, exercising the real code path (no mocks).
const stubSource = `package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func resolve(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func main() {
	switch os.Getenv("MUTEST_STUB") {
	case "pass":
		os.Exit(0)
	case "fail":
		fmt.Fprintln(os.Stdout, "STUB_FAIL_OUTPUT")
		os.Exit(1)
	case "sleep":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "cwd":
		wd, _ := os.Getwd()
		want := os.Getenv("MUTEST_STUB_WANT_DIR")
		if resolve(wd) != resolve(want) {
			fmt.Fprintf(os.Stderr, "cwd mismatch: got %q want %q\n", wd, want)
			os.Exit(1)
		}
		os.Exit(0)
	case "echoid":
		got := os.Getenv("MUTEST_ID")
		want := os.Getenv("MUTEST_STUB_WANT_ID")
		if got != want {
			fmt.Fprintf(os.Stderr, "id mismatch: got %q want %q\n", got, want)
			os.Exit(1)
		}
		os.Exit(0)
	default:
		os.Exit(0)
	}
}
`

// stubBin is the absolute path to the compiled stub binary.
var stubBin string

func TestMain(m *testing.M) {
	os.Exit(buildStubAndRun(m))
}

func buildStubAndRun(m *testing.M) int {
	dir, err := os.MkdirTemp("", "mutest-stub-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module muteststub\n\ngo 1.21\n"), 0644); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stub.go"), []byte(stubSource), 0644); err != nil {
		panic(err)
	}

	stubBin = filepath.Join(dir, "stub")
	cmd := exec.Command("go", "build", "-o", stubBin, ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("failed to build stub binary: " + err.Error() + "\n" + string(out))
	}

	return m.Run()
}

// want describes the expected classification flags of a single Result.
type want struct {
	killed, timedOut, canceled, hasErr bool
}

func checkResult(t *testing.T, r Result, w want) {
	t.Helper()
	if r.Killed != w.killed {
		t.Errorf("Killed = %v, want %v (output=%q, err=%v)", r.Killed, w.killed, r.Output, r.Err)
	}
	if r.TimedOut != w.timedOut {
		t.Errorf("TimedOut = %v, want %v", r.TimedOut, w.timedOut)
	}
	if r.Canceled != w.canceled {
		t.Errorf("Canceled = %v, want %v", r.Canceled, w.canceled)
	}
	if (r.Err != nil) != w.hasErr {
		t.Errorf("Err = %v, want hasErr=%v", r.Err, w.hasErr)
	}
}

// checkSummarySingle asserts the summary counts of a run that produced
// exactly one result classified according to w.
func checkSummarySingle(t *testing.T, s *Summary, w want) {
	t.Helper()
	if s.Total != 1 {
		t.Errorf("Total = %d, want 1", s.Total)
	}
	var exp Summary
	switch {
	case w.canceled:
		exp.Canceled = 1
	case w.hasErr:
		exp.Errors = 1
	case w.timedOut:
		exp.TimedOut = 1
	case w.killed:
		exp.Killed = 1
	default:
		exp.Survived = 1
	}
	if s.Killed != exp.Killed || s.Survived != exp.Survived || s.TimedOut != exp.TimedOut || s.Errors != exp.Errors || s.Canceled != exp.Canceled {
		t.Errorf("summary = {K:%d S:%d T:%d E:%d C:%d}, want {K:%d S:%d T:%d E:%d C:%d}",
			s.Killed, s.Survived, s.TimedOut, s.Errors, s.Canceled,
			exp.Killed, exp.Survived, exp.TimedOut, exp.Errors, exp.Canceled)
	}
}

// runPkgs runs RunInstrumented over the given packages, collecting the
// results streamed through the progress callback.
func runPkgs(t *testing.T, ctx context.Context, cfg Config, pkgList ...*engine.InstrumentedPackage) (*Summary, []Result) {
	t.Helper()
	pkgs := make(map[string]*engine.InstrumentedPackage, len(pkgList))
	for _, p := range pkgList {
		pkgs[p.ImportPath] = p
	}
	var mu sync.Mutex
	var prog []Result
	summary := RunInstrumented(ctx, pkgs, cfg, func(r Result, done, total int) {
		mu.Lock()
		prog = append(prog, r)
		mu.Unlock()
	})
	return summary, prog
}

func stubPkg(mutestID int) *engine.InstrumentedPackage {
	return &engine.InstrumentedPackage{
		ImportPath: "example.com/stub",
		BinaryPath: stubBin,
		Mutations:  []mutator.MutationPoint{{MutestID: mutestID, File: "/x/lib.go", Line: 1, Column: 1}},
	}
}

// TestRunInstrumented_Classification covers T1-1, T1-2, T1-3, T1-5, T1-7, T1-8.
func TestRunInstrumented_Classification(t *testing.T) {
	tests := []struct {
		name     string
		stub     string
		missing  bool // point BinaryPath at a nonexistent file
		setDir   bool // set pkg.Dir and MUTEST_STUB_WANT_DIR to a temp dir
		wantID   string
		mutestID int
		timeout  time.Duration
		want     want
	}{
		{name: "T1-1_exit1_killed", stub: "fail", mutestID: 1, timeout: 30 * time.Second, want: want{killed: true}},
		{name: "T1-2_exit0_survived", stub: "pass", mutestID: 1, timeout: 30 * time.Second, want: want{}},
		{name: "T1-3_sleep_timeout", stub: "sleep", mutestID: 1, timeout: 200 * time.Millisecond, want: want{timedOut: true}},
		{name: "T1-5_missing_binary_error", stub: "pass", missing: true, mutestID: 1, timeout: 30 * time.Second, want: want{hasErr: true}},
		{name: "T1-7_cwd_is_pkg_dir", stub: "cwd", setDir: true, mutestID: 1, timeout: 30 * time.Second, want: want{}},
		{name: "T1-8_echoid_passes_MUTEST_ID", stub: "echoid", wantID: "42", mutestID: 42, timeout: 30 * time.Second, want: want{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MUTEST_STUB", tt.stub)

			pkg := stubPkg(tt.mutestID)
			if tt.missing {
				pkg.BinaryPath = filepath.Join(t.TempDir(), "does-not-exist")
			}
			if tt.setDir {
				dir := t.TempDir()
				pkg.Dir = dir
				t.Setenv("MUTEST_STUB_WANT_DIR", dir)
			}
			if tt.wantID != "" {
				t.Setenv("MUTEST_STUB_WANT_ID", tt.wantID)
			}

			cfg := Config{Workers: 1, Timeout: tt.timeout}
			summary, _ := runPkgs(t, context.Background(), cfg, pkg)

			if len(summary.Results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(summary.Results))
			}
			checkResult(t, summary.Results[0], tt.want)
			checkSummarySingle(t, summary, tt.want)
		})
	}
}

// TestRunInstrumented_CanceledBeforeLaunch covers T1-4: when the parent ctx is
// canceled before RunInstrumented launches any mutant, every result must be
// classified Canceled (not Killed/Survived), and no KILLED result may be
// streamed to the progress callback. Uses Workers=1 with multiple mutants to
// exercise the unlaunched-prefill path.
func TestRunInstrumented_CanceledBeforeLaunch(t *testing.T) {
	t.Setenv("MUTEST_STUB", "pass")

	const n = 3
	var muts []mutator.MutationPoint
	for i := 1; i <= n; i++ {
		muts = append(muts, mutator.MutationPoint{MutestID: i, File: "/x/lib.go", Line: i})
	}
	pkg := &engine.InstrumentedPackage{ImportPath: "example.com/stub", BinaryPath: stubBin, Mutations: muts}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE launch

	cfg := Config{Workers: 1, Timeout: 30 * time.Second}
	summary, prog := runPkgs(t, ctx, cfg, pkg)

	if summary.Canceled != n {
		t.Errorf("Summary.Canceled = %d, want %d", summary.Canceled, n)
	}
	if summary.Killed != 0 {
		t.Errorf("Summary.Killed = %d, want 0", summary.Killed)
	}
	if summary.Survived != 0 {
		t.Errorf("Summary.Survived = %d, want 0", summary.Survived)
	}
	if len(summary.Results) != n {
		t.Fatalf("expected %d results, got %d", n, len(summary.Results))
	}
	for i, r := range summary.Results {
		if !r.Canceled {
			t.Errorf("result[%d].Canceled = false, want true", i)
		}
		if r.Killed {
			t.Errorf("result[%d].Killed = true, want false", i)
		}
	}
	for _, r := range prog {
		if r.Killed {
			t.Error("progress callback streamed a KILLED result during cancellation")
		}
	}
}

// TestRunInstrumented_NoTests covers T1-6: a package flagged NoTests must not
// be exec'd; its mutants survive with a fixed explanatory Output.
func TestRunInstrumented_NoTests(t *testing.T) {
	t.Setenv("MUTEST_STUB", "fail") // would be KILLED if the binary were exec'd

	pkg := &engine.InstrumentedPackage{
		ImportPath: "example.com/notest",
		BinaryPath: "", // no binary is built for a package without tests
		NoTests:    true,
		Mutations:  []mutator.MutationPoint{{MutestID: 1, File: "/x/lib.go", Line: 1}},
	}

	cfg := Config{Workers: 1, Timeout: 30 * time.Second}
	summary, _ := runPkgs(t, context.Background(), cfg, pkg)

	if len(summary.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(summary.Results))
	}
	r := summary.Results[0]
	if r.Killed {
		t.Error("NoTests mutant must not be killed")
	}
	if r.Output != "package has no test files" {
		t.Errorf("Output = %q, want %q", r.Output, "package has no test files")
	}
	if summary.Survived != 1 {
		t.Errorf("Summary.Survived = %d, want 1", summary.Survived)
	}
}

// TestVerifyBaseline covers T1-9 and T1-10.
func TestVerifyBaseline(t *testing.T) {
	cfg := Config{Workers: 2, Timeout: 30 * time.Second}

	t.Run("failing_tests_error_with_import_path_and_output", func(t *testing.T) {
		t.Setenv("MUTEST_STUB", "fail")
		pkg := &engine.InstrumentedPackage{ImportPath: "example.com/failpkg", BinaryPath: stubBin}
		pkgs := map[string]*engine.InstrumentedPackage{pkg.ImportPath: pkg}

		err := VerifyBaseline(context.Background(), pkgs, cfg)
		if err == nil {
			t.Fatal("expected baseline error, got nil")
		}
		if !strings.Contains(err.Error(), "example.com/failpkg") {
			t.Errorf("error should contain import path, got: %v", err)
		}
		if !strings.Contains(err.Error(), "STUB_FAIL_OUTPUT") {
			t.Errorf("error should contain test output, got: %v", err)
		}
	})

	t.Run("passing_tests_return_nil", func(t *testing.T) {
		t.Setenv("MUTEST_STUB", "pass")
		pkg := &engine.InstrumentedPackage{ImportPath: "example.com/passpkg", BinaryPath: stubBin}
		pkgs := map[string]*engine.InstrumentedPackage{pkg.ImportPath: pkg}

		if err := VerifyBaseline(context.Background(), pkgs, cfg); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("no_tests_package_is_skipped", func(t *testing.T) {
		t.Setenv("MUTEST_STUB", "fail") // would error if executed
		pkg := &engine.InstrumentedPackage{ImportPath: "example.com/notestpkg", BinaryPath: stubBin, NoTests: true}
		pkgs := map[string]*engine.InstrumentedPackage{pkg.ImportPath: pkg}

		if err := VerifyBaseline(context.Background(), pkgs, cfg); err != nil {
			t.Errorf("NoTests package must be skipped; got %v", err)
		}
	})

	t.Run("does_not_inherit_MUTEST_ID_from_parent_env", func(t *testing.T) {
		// A user's shell may export MUTEST_ID. The baseline must still run
		// with no mutation active: the runner appends MUTEST_ID=0 (helper IDs
		// are 1-based, so 0 activates nothing) and os/exec keeps the last
		// duplicate. Without that, the inherited value would activate a
		// mutation during the baseline and fake a baseline failure.
		t.Setenv("MUTEST_ID", "7") // pollute the parent environment
		t.Setenv("MUTEST_STUB", "echoid")
		t.Setenv("MUTEST_STUB_WANT_ID", "0")
		pkg := &engine.InstrumentedPackage{ImportPath: "example.com/envpkg", BinaryPath: stubBin}
		pkgs := map[string]*engine.InstrumentedPackage{pkg.ImportPath: pkg}

		if err := VerifyBaseline(context.Background(), pkgs, cfg); err != nil {
			t.Errorf("baseline must neutralize inherited MUTEST_ID; got %v", err)
		}
	})
}
