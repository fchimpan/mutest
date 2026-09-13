package mutest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/fchimpan/mutest/output"
	"github.com/fchimpan/mutest/runner"
)

func TestRegressionMutationSemantics(t *testing.T) {
	cases := []struct {
		name, src, test string
		total, killed   int
	}{
		{"defined bool", `type Flag bool
func F(n int) Flag { return n > 0 }`, `if F(0) {t.Fatal("boundary")}`, 1, 1},
		{"constant precision", `func F() bool {return 9007199254740992.0 < 9007199254740993.0}`, `if !F() {t.Fatal("precision")}`, 1, 0},
		{"large constant", `func F() bool {return 1<<100 > 0}`, `if !F() {t.Fatal("overflow")}`, 1, 0},
		{"constant boundary", `func F() bool {return 1<<100 <= 1<<100}`, `if !F() {t.Fatal("boundary")}`, 1, 1},
		{"len boundary", `func F(s []int) bool {return len(s)>0}`, `if F(nil) {t.Fatal("empty")}`, 1, 1},
		{"fallback", `func F(err error) string {if err!=nil {return "fallback"};return "ok"}`, `if F(nil)!="ok" {t.Fail()}`, 1, 1},
		{"success branch", `func F(err error) string {if err==nil {return "ok"};return "bad"}`, `if F(nil)!="ok" {t.Fail()}`, 1, 1},
		{"direct propagation", `func F(err error) error {if err!=nil {return err};return nil}`, `if F(nil)!=nil {t.Fail()}`, 0, 0},
		{"wrapped propagation", `import "fmt"
func F(err error) error {if err!=nil {return fmt.Errorf("wrap: %w",err)};return nil}`, `if F(nil)!=nil {t.Fail()}`, 0, 0},
		{"return side effects", `var Calls int
func record()int{Calls++;return 0}
func F(err error)(int,error) {if err!=nil {return record(),err};return 0,nil}`, `_,_=F(nil);if Calls!=0 {t.Fail()}`, 1, 1},
		{"typed non error", `func F(err *int) int {if err!=nil {return 1};return 0}`, `if F(nil)!=0 {t.Fail()}`, 1, 1},
		{"operand side effects", `var Calls int
func next() int {Calls++;return Calls}
func F() bool {return next()<next()}`, `Calls=0;if !F() || Calls!=2 {t.Fatal("evaluated twice")}`, 1, 0},
		{"cgo", `/* typedef int number; */
import "C"
func F(n int) bool {return n>0}`, `if F(0) {t.Fail()}`, 1, 1},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "cgo" {
				if os.Getenv("CGO_ENABLED") == "0" {
					t.Skip("cgo disabled")
				}
				if _, err := exec.LookPath("cc"); err != nil {
					t.Skip("C compiler unavailable")
				}
			}
			dir := t.TempDir()
			writeFiles(t, dir, map[string]string{"go.mod": "module regression\n\ngo 1.24.0\n", "lib.go": "package regression\n" + tt.src + "\n", "lib_test.go": "package regression\nimport \"testing\"\nfunc TestF(t *testing.T){" + tt.test + "}\n"})
			chdir(t, dir)
			var out, diag bytes.Buffer
			err := Run(context.Background(), []string{"-json", "-workers", "1", "./..."}, &out, &diag)
			if err != nil && !errors.Is(err, ErrTestsFailed) {
				t.Fatalf("%v\n%s", err, diag.String())
			}
			var s output.JSONSummary
			if err := json.Unmarshal(out.Bytes(), &s); err != nil {
				t.Fatal(err)
			}
			if s.Total != tt.total || s.Killed != tt.killed || s.Errors != 0 || s.Canceled != 0 {
				t.Fatalf("got %+v", s)
			}
		})
	}
}

func TestRegressionDiscoveryErrorsAndPositions(t *testing.T) {
	for _, tt := range []struct {
		name, src string
		wantErr   bool
	}{
		{"syntax error", "package p\nfunc F(n int) bool {return n > }", true},
		{"physical lines", "package p\n//line input.go:1000\nfunc F(n int) bool {\n//mutest:skip\nreturn n > 0\n}", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := t.TempDir()
			writeFiles(t, d, map[string]string{"go.mod": "module p\ngo 1.24.0\n", "lib.go": tt.src})
			chdir(t, d)
			var out bytes.Buffer
			err := Run(context.Background(), []string{"-dry-run", "-json", "./..."}, &out, io.Discard)
			if tt.wantErr {
				if !errors.Is(err, ErrDiscovery) {
					t.Fatalf("got %v", err)
				}
			} else if err != nil || strings.TrimSpace(out.String()) != "[]" {
				t.Fatalf("%v %s", err, &out)
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("disk full") }
func TestRunPropagatesOutputFailure(t *testing.T) {
	d := t.TempDir()
	writeFiles(t, d, map[string]string{"go.mod": "module p\ngo 1.24.0\n", "lib.go": "package p\n"})
	chdir(t, d)
	err := Run(context.Background(), []string{"-dry-run", "-json"}, failingWriter{}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("got %v", err)
	}
}
func TestRunCanceledBeforeDiscovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, []string{"-dry-run"}, io.Discard, io.Discard)
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("got %v", err)
	}
	if strings.Count(err.Error(), ErrInterrupted.Error()) != 1 {
		t.Fatalf("duplicated interruption diagnostic: %v", err)
	}
}
func TestThresholdUsesExactRate(t *testing.T) {
	s := &runner.Summary{Total: 2001, Killed: 2000, Survived: 1}
	if !errors.Is(evaluateSummary(s, 100), ErrTestsFailed) {
		t.Fatal("survivor passed threshold 100")
	}
	if evaluateSummary(s, 99.9) != nil {
		t.Fatal("valid threshold rejected")
	}
}

func TestRunSerializesSharedPackageFixtures(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module fixture\ngo 1.24.0\n",
		"lib.go": `package fixture
 func A(n int) bool {return n>0}
 func B(n int) bool {return n<0}
 `,
		"lib_test.go": `package fixture
 import("os";"testing";"time")
 func TestSharedFixture(t *testing.T) {
  f,err:=os.OpenFile("fixture.lock",os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
  if err!=nil {t.Fatal(err)}
  f.Close()
  defer os.Remove("fixture.lock")
  time.Sleep(100*time.Millisecond)
 }
 `,
	})
	chdir(t, dir)
	var out, diag bytes.Buffer
	err := Run(context.Background(), []string{"-json", "-workers", "2", "./..."}, &out, &diag)
	if !errors.Is(err, ErrTestsFailed) {
		t.Fatalf("%v: %s", err, &diag)
	}
	var s output.JSONSummary
	if err := json.Unmarshal(out.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	if s.Total != 2 || s.Survived != 2 || s.Killed != 0 {
		t.Fatalf("false detection from fixture conflict: %+v", s)
	}
}
