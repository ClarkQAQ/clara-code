package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"clara-code/internal/cli"
)

var version = "dev"

func main() {
	signals := []os.Signal{os.Interrupt, syscall.SIGTERM}
	if runtime.GOOS != "windows" {
		signals = append(signals, syscall.SIGHUP)
	}
	ctx, stop := signal.NotifyContext(context.Background(), signals...)
	defer stop()
	if err := cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, version); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "clara-code:", err)
		os.Exit(1)
	}
}
