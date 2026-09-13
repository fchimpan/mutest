//go:build unix

package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeGo(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte("#!/bin/sh\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestDiscoveryCancelsRunningGoList(t *testing.T) {
	fakeGo(t, "sleep 30\n")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := New([]string{"."}).DiscoverAllContext(ctx)
	if err == nil || ctx.Err() == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("cancellation: %v after %v", err, time.Since(start))
	}
}

func TestBuildTestBinariesBoundsConcurrency(t *testing.T) {
	t.Setenv("MUTEST_BUILD_LOCK", filepath.Join(t.TempDir(), "build.lock"))
	fakeGo(t, `mkdir "$MUTEST_BUILD_LOCK" || exit 1
trap 'rmdir "$MUTEST_BUILD_LOCK"' EXIT
sleep 0.1
while [ "$#" -gt 0 ]; do
 if [ "$1" = "-o" ]; then shift; touch "$1"; break; fi
 shift
done
`)
	pkgs := map[string]*InstrumentedPackage{}
	for _, name := range []string{"a", "b", "c"} {
		pkgs[name] = &InstrumentedPackage{ImportPath: name, TempDir: t.TempDir()}
	}
	if err := New(nil).BuildTestBinaries(context.Background(), pkgs, 1); err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		if pkg.BinaryPath == "" {
			t.Fatalf("package not built: %+v", pkg)
		}
	}
}

func TestBuildTestBinariesPreservesBuildError(t *testing.T) {
	fakeGo(t, "echo deliberate-build-failure >&2\nexit 7\n")
	pkgs := map[string]*InstrumentedPackage{"p": {ImportPath: "p", TempDir: t.TempDir()}}
	// Exercise the existing API with no optional worker limit.
	err := New(nil).BuildTestBinaries(context.Background(), pkgs)
	if err == nil || !strings.Contains(err.Error(), "deliberate-build-failure") {
		t.Fatalf("original build failure lost: %v", err)
	}
}
