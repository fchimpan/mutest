package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/fchimpan/mutest/cmd/mutest"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(mutest.Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
