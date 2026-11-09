package session

import (
	"net/url"
	"testing"
)

func TestWindowsDatabaseURI(t *testing.T) {
	for path, want := range map[string]string{
		`C:\Users\一 二\#db%.sqlite`: `file:///C:/Users/%E4%B8%80%20%E4%BA%8C/%23db%25.sqlite?mode=rw`,
		`\\server\share\db.sqlite`: `file:////server/share/db.sqlite?mode=rw`,
	} {
		uri := databaseURI(path, url.Values{"mode": {"rw"}})
		if got := uri.String(); got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
}
