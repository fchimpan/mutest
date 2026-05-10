package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/fchimpan/mutest/cmd/mutest"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := mutest.Run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "mutest: %v\n", err)
		os.Exit(1)
	}
}
