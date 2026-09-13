package process

import (
	"context"
	"os/exec"
	"strconv"
	"time"
)

func configure(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		// taskkill /T terminates the process tree, including tests' subprocesses.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, "taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
