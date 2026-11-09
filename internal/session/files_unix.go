//go:build unix

package session

import (
	"net/url"
	"os"

	"golang.org/x/sys/unix"
)

func openInputHistory(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|unix.O_NONBLOCK, 0)
}

func databaseURI(path string, params url.Values) url.URL {
	return url.URL{Scheme: "file", Path: path, RawQuery: params.Encode()}
}
