package clipboard

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestPlatformClipboardReaders(t *testing.T) {
	for _, platform := range []string{"darwin", "linux"} {
		for _, wayland := range []bool{false, true} {
			calls := 0
			result, err := ReadWith(t.Context(), platform, wayland, func(_ context.Context, name string, args ...string) ([]byte, error) {
				calls++
				if platform == "darwin" {
					if !strings.Contains(strings.Join(args, " "), "NSPasteboard") {
						t.Fatal("not reading native clipboard")
					}
					return []byte(base64.StdEncoding.EncodeToString([]byte("image"))), nil
				}
				if wayland && name != "wl-paste" || !wayland && name != "xclip" {
					t.Fatal("wrong Linux clipboard", name)
				}
				return []byte("image"), nil
			})
			if err != nil || string(result.Image) != "image" || calls != 1 {
				t.Fatal(platform, result, err, calls)
			}
		}
	}
}
func TestTextFallbackAndUnavailableClipboard(t *testing.T) {
	calls := 0
	result, err := ReadWith(t.Context(), "linux", true, func(context.Context, string, ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("no image")
		}
		return []byte("plain text"), nil
	})
	if err != nil || result.Text != "plain text" || len(result.Image) != 0 {
		t.Fatal(result, err)
	}
	_, err = ReadWith(t.Context(), "linux", false, func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("unavailable") })
	if err == nil || !strings.Contains(err.Error(), "/attach") {
		t.Fatal("missing fallback guidance", err)
	}
}
