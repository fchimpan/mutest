//go:build unix

package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCancellationStopsDescendants(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "child-survived")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	cmd := Command(ctx, "sh", "-c", `(sleep 0.6; touch "$1") & wait`, "sh", marker)
	start := time.Now()
	_, err := CombinedOutput(cmd, 1024)
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("termination: %v elapsed=%s", err, time.Since(start))
	}
	time.Sleep(700 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("descendant survived cancellation")
	}
}

func TestNormalExitStopsRemainingDescendants(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "orphan")
	cmd := Command(context.Background(), "sh", "-c", `(sleep 0.5; touch "$1") >/dev/null 2>&1 &`, "sh", marker)
	if _, err := CombinedOutput(cmd, 1024); err != nil {
		t.Fatal(err)
	}
	time.Sleep(800 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("normal parent exit left a descendant running")
	}
}
