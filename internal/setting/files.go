package setting

import (
	"os"
	"path/filepath"
)

func Home() (string, error) {
	if path := os.Getenv("CLARA_CODE_HOME"); path != "" {
		return filepath.Abs(path)
	}
	return platformHome()
}
