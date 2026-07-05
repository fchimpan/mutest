package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/fchimpan/mutest/cmd/mutest"
)

func main() {
	// syscall.SIGTERM is included alongside os.Interrupt (SIGINT) so that a
	// CI job cancellation (which typically sends SIGTERM, not SIGINT) also
	// triggers the graceful shutdown path in cmd/mutest/run.go: without it,
	// SIGTERM kills the process before its deferred cleanup can remove the
	// instrumented-package temp dirs, leaking them. syscall.SIGTERM is
	// defined as a signal.Signal constant on Windows too, so this builds
	// cross-platform even though Windows does not actually deliver it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := mutest.Run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "mutest: %v\n", err)
		os.Exit(1)
	}
}
