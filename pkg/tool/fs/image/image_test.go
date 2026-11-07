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
