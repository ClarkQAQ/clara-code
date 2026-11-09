//go:build !windows

package host

import (
	"os"
	"path/filepath"
	"strings"
)

func resolveNative(cwd, value string) (string, error) {
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		value = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(value, "~"), "/"))
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(cwd, value)
	}
	return filepath.Clean(value), nil
}

func isLink(info os.FileInfo) bool { return info.Mode()&os.ModeSymlink != 0 }
