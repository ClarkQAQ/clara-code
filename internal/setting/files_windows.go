package setting

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func platformHome() (string, error) {
	data := os.Getenv("LOCALAPPDATA")
	if !filepath.IsAbs(data) {
		var err error
		data, err = windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(data, "ClaraCode"), nil
}
