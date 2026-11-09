//go:build !windows

package clipboard

import (
	"context"
	"os"
	"runtime"
)

func readPlatform(ctx context.Context) (Content, error) {
	return ReadWith(ctx, runtime.GOOS, os.Getenv("WAYLAND_DISPLAY") != "", command)
}
