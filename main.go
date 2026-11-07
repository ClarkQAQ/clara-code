package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"clara-code/internal/cli"
)

var version = "0.1.0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	if err := cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, version); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "clara-code:", err)
		os.Exit(1)
	}
}
