package mutest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	mutest "github.com/fchimpan/mutest/cmd/mutest"
	"github.com/fchimpan/mutest/output"
)

func TestRegressionMutationSemantics(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		test   string
		total  int
		killed int
	}{
		{
			name:   "defined bool",
			src:    "type Flag bool\nfunc F(n int) Flag { return n > 0 }",
			test:   `if F(0) {t.Fatal("boundary")}`,
			total:  1,
			killed: 1,
		},
		{
			name:  "constant precision",
			src:   "func F() bool {return 9007199254740992.0 < 9007199254740993.0}",
			test:  `if !F() {t.Fatal("precision")}`,
			total: 1,
		},
		{
			name:  "large constant",
			src:   "func F() bool {return 1<<100 > 0}",
			test:  `if !F() {t.Fatal("overflow")}`,
			total: 1,
		},
		{
			name:   "constant boundary",
			src:    "func F() bool {return 1<<100 <= 1<<100}",
			test:   `if !F() {t.Fatal("boundary")}`,
			total:  1,
			killed: 1,
		},
		{
			name:   "len boundary",
			src:    "func F(s []int) bool {return len(s)>0}",
			test:   `if F(nil) {t.Fatal("empty")}`,
			total:  1,
			killed: 1,
		},
		{
			name:   "fallback",
			src:    `func F(err error) string {if err!=nil {return "fallback"};return "ok"}`,
			test:   `if F(nil)!="ok" {t.Fail()}`,
			total:  1,
			killed: 1,
		},
		{
			name:   "success branch",
			src:    `func F(err error) string {if err==nil {return "ok"};return "bad"}`,
			test:   `if F(nil)!="ok" {t.Fail()}`,
			total:  1,
			killed: 1,
		},
		{
			name: "direct propagation",
			src:  "func F(err error) error {if err!=nil {return err};return nil}",
			test: `if F(nil)!=nil {t.Fail()}`,
		},
		{
			name: "wrapped propagation",
			src: `import "fmt"
func F(err error) error {if err!=nil {return fmt.Errorf("wrap: %w",err)};return nil}`,
			test: `if F(nil)!=nil {t.Fail()}`,
		},
		{
			name: "return side effects",
			src: `var Calls int
func record()int{Calls++;return 0}
func F(err error)(int,error) {if err!=nil {return record(),err};return 0,nil}`,
			test:   `_,_=F(nil);if Calls!=0 {t.Fail()}`,
			total:  1,
			killed: 1,
		},
		{
			name:   "typed non error",
			src:    "func F(err *int) int {if err!=nil {return 1};return 0}",
			test:   `if F(nil)!=0 {t.Fail()}`,
			total:  1,
			killed: 1,
		},
		{
			name: "operand side effects",
			src: `var Calls int
func next() int {Calls++;return Calls}
func F() bool {return next()<next()}`,
			test:  `Calls=0;if !F() || Calls!=2 {t.Fatal("evaluated twice")}`,
			total: 1,
		},
		{
			name: "cgo",
			src: `/* typedef int number; */
import "C"
func F(n int) bool {return n>0}`,
			test:   "if F(0) {t.Fail()}",
			total:  1,
			killed: 1,
		},
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
			writeFiles(t, dir, map[string]string{
				"go.mod":      "module regression\n\ngo 1.24.0\n",
				"lib.go":      "package regression\n" + tt.src + "\n",
				"lib_test.go": "package regression\nimport \"testing\"\nfunc TestF(t *testing.T){" + tt.test + "}\n",
			})
			chdir(t, dir)
			var out, diag bytes.Buffer
			err := mutest.Run(context.Background(), []string{"-json", "-workers", "1", "./..."}, &out, &diag)
			if err != nil && !errors.Is(err, mutest.ErrTestsFailed) {
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
		name    string
		src     string
		wantErr bool
	}{
		{
			name:    "syntax error",
			src:     "package p\nfunc F(n int) bool {return n > }",
			wantErr: true,
		},
		{
			name: "physical lines",
			src:  "package p\n//line input.go:1000\nfunc F(n int) bool {\n//mutest:skip\nreturn n > 0\n}",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := t.TempDir()
			writeFiles(t, d, map[string]string{
				"go.mod": "module p\ngo 1.24.0\n",
				"lib.go": tt.src,
			})
			chdir(t, d)
			var out bytes.Buffer
			err := mutest.Run(context.Background(), []string{"-dry-run", "-json", "./..."}, &out, io.Discard)
			if tt.wantErr {
				if !errors.Is(err, mutest.ErrDiscovery) {
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
	writeFiles(t, d, map[string]string{
		"go.mod": "module p\ngo 1.24.0\n",
		"lib.go": "package p\n",
	})
	chdir(t, d)
	err := mutest.Run(context.Background(), []string{"-dry-run", "-json"}, failingWriter{}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("got %v", err)
	}
}
func TestRunCanceledBeforeDiscovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := mutest.Run(ctx, []string{"-dry-run"}, io.Discard, io.Discard)
	if !errors.Is(err, mutest.ErrInterrupted) {
		t.Fatalf("got %v", err)
	}
	if strings.Count(err.Error(), mutest.ErrInterrupted.Error()) != 1 {
		t.Fatalf("duplicated interruption diagnostic: %v", err)
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
	err := mutest.Run(context.Background(), []string{"-json", "-workers", "2", "./..."}, &out, &diag)
	if !errors.Is(err, mutest.ErrTestsFailed) {
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

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}
