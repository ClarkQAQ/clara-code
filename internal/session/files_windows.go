package session

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func openInputHistory(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}

func databaseURI(path string, params url.Values) url.URL {
	path = filepath.ToSlash(path)
	if strings.HasPrefix(path, "//") {
		// Preserve the UNC prefix in the filename without setting a URI authority.
		return url.URL{Scheme: "file", Path: path, RawQuery: params.Encode()}
	}
	return url.URL{Scheme: "file", Path: "/" + path, RawQuery: params.Encode()}
}
