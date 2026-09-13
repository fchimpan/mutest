//go:build unix

package runner

import (
	"context"
	"github.com/fchimpan/mutest/engine"
	"github.com/fchimpan/mutest/mutator"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExternalSignalIsAnError(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "test.sh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nkill -KILL $$\n"), 0755); err != nil {
		t.Fatal(err)
	}
	result := testMutantRuntime(context.Background(), &engine.InstrumentedPackage{BinaryPath: bin}, mutator.MutationPoint{}, Config{Timeout: time.Second})
	if result.Err == nil || result.Killed || result.Canceled || result.TimedOut {
		t.Fatalf("external signal misclassified: %+v", result)
	}
}
