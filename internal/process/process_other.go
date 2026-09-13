//go:build !unix && !windows

package process

import "os/exec"

func configure(cmd *exec.Cmd) {}
