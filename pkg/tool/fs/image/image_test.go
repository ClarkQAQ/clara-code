package image_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	claraimage "clara-code/pkg/tool/fs/image"
)

func TestNormalizeScalesAndPreservesTransparency(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 512, 256))
	for x := 0; x < 512; x++ {
		for y := 0; y < 256; y++ {
			source.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 128, A: 128})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, source); err != nil {
		t.Fatal(err)
	}
	view, err := claraimage.Normalize(buffer.Bytes(), "test.png", claraimage.Options{
		MaxDimension: 128, MaxBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Width > 128 || view.Height > 128 {
		t.Fatalf("size = %dx%d", view.Width, view.Height)
	}
	if view.MIME != "image/png" {
		t.Fatalf("mime = %s", view.MIME)
	}
	if _, err := png.Decode(bytes.NewReader(view.Data)); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeRejectsNonImage(t *testing.T) {
	_, err := claraimage.Normalize([]byte("not an image"), "x.txt", claraimage.Options{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeCapsOutputPixelsIndependentlyOfDecodeLimit(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
		pixels        int64
		wantW, wantH  int
	}{
		{name: "area", width: 512, height: 256, pixels: 8192, wantW: 128, wantH: 64},
		{name: "thin", width: 1, height: 100, pixels: 2, wantW: 1, wantH: 2},
		{name: "wide", width: 100, height: 1, pixels: 2, wantW: 2, wantH: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buffer bytes.Buffer
			if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, tc.width, tc.height))); err != nil {
				t.Fatal(err)
			}
			view, err := claraimage.Normalize(buffer.Bytes(), "test.png", claraimage.Options{MaxOutputPixels: tc.pixels})
			if err != nil || view.Width != tc.wantW || view.Height != tc.wantH {
				t.Fatalf("got %dx%d, want %dx%d: %v", view.Width, view.Height, tc.wantW, tc.wantH, err)
			}
			if _, err := claraimage.Normalize(buffer.Bytes(), "test.png", claraimage.Options{MaxPixels: tc.pixels}); err == nil {
				t.Fatal("source pixel limit was ignored")
			}
		})
	}
}
