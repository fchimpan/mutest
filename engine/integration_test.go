package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fchimpan/mutest/engine"
	"github.com/fchimpan/mutest/mutator"
)

func TestEqualityInstrumentationDoesNotReserveUnusedOrderedName(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"go.mod":      "module p\ngo 1.24.0\n",
		"lib.go":      "package p\ntype _mutest_ordered int\nfunc F(n int)bool{return n==0}\n",
		"lib_test.go": "package p\nimport \"testing\"\nfunc TestF(t *testing.T){if !F(0){t.Fail()}}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, dir)

	e := engine.New([]string{"."}, &mutator.EqualityMutator{})
	points, err := e.DiscoverAll()
	if err != nil {
		t.Fatal(err)
	}
	pkgs, err := e.InstrumentAll(points)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.CleanupInstrumented(pkgs)
	if err := e.BuildTestBinaries(context.Background(), pkgs); err != nil {
		t.Fatalf("unused helper name collided with user declaration: %v", err)
	}
}
