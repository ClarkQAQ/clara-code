package setting

import (
	"os"
	"path/filepath"
	"runtime"
)

func Home() (string, error) {
	if path := os.Getenv("CLARA_CODE_HOME"); path != "" {
		return filepath.Abs(path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "clara-code"), nil
	}
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "clara-code"), nil
}
