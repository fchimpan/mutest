// Package process provides cancellation and bounded diagnostic output for subprocesses.
package process

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	configure(cmd)
	return cmd
}

// CombinedOutput retains only the last limit bytes, even if a test logs forever.
func CombinedOutput(cmd *exec.Cmd, limit int) ([]byte, error) {
	b := &tailBuffer{limit: limit}
	cmd.Stdout, cmd.Stderr = b, b
	err := cmd.Run()
	// Reap any descendants that outlived the direct child.
	if cmd.Process != nil {
		_ = cmd.Cancel()
	}
	if b.truncated {
		return append([]byte("...(truncated)...\n"), b.data...), err
	}
	return b.data, err
}

type tailBuffer struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(b.data)+n > b.limit {
		b.truncated = true
		if n >= b.limit {
			b.data = append(b.data[:0], p[n-b.limit:]...)
			return n, nil
		}
		b.data = append(b.data[:0], b.data[len(b.data)+n-b.limit:]...)
	}
	b.data = append(b.data, p...)
	return n, nil
}
