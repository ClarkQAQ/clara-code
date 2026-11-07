// Package image reads and normalizes static images for model inspection.
package image

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"path"
	"strings"

	clarafs "clara-code/pkg/tool/fs"
	"clara-code/pkg/util/errs"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const (
	DefaultMaxBytes     = 16 << 20
	DefaultMaxDimension = 2048
	DefaultMaxPixels    = 64_000_000
)

// Options bounds image decoding and output.
type Options struct {
	MaxBytes     int64
	MaxDimension int
	MaxPixels    int64
}

// View is one image normalized for a model.
type View struct {
	Path       clarafs.Path
	Data       []byte
	MIME       string
	SourceMIME string
	Width      int
	Height     int
	SourceSize int64
}

// Load reads one image from storage.
func Load(ctx context.Context, storage clarafs.FS, value clarafs.Path, options Options) (View, error) {
	options = withDefaults(options)
	entry, err := storage.Stat(ctx, value)
	if err != nil {
		return View{}, err
	}
	if entry.IsDir {
		return View{}, errs.New(errs.CodeInvalidArgument, "path is a directory")
	}
	if entry.Size > options.MaxBytes {
		return View{}, fmt.Errorf("image is larger than %d bytes", options.MaxBytes)
	}
	data, err := storage.ReadFile(ctx, value, options.MaxBytes+1)
	if err != nil {
		return View{}, err
	}
	normalized, err := Normalize(data, entry.Name, options)
	if err != nil {
		return View{}, err
	}
	normalized.Path = value
	normalized.SourceSize = entry.Size
	return normalized, nil
}

// Normalize decodes, validates, scales and re-encodes one image.
func Normalize(data []byte, filename string, options Options) (View, error) {
	options = withDefaults(options)
	if len(data) == 0 {
		return View{}, errors.New("image is empty")
	}
	if int64(len(data)) > options.MaxBytes {
		return View{}, fmt.Errorf("image is larger than %d bytes", options.MaxBytes)
	}
	mime := detectMIME(data, filename)
	if !strings.HasPrefix(mime, "image/") {
		return View{}, fmt.Errorf("file is not a supported image (%s)", mime)
	}
	if mime == "image/gif" {
		frames, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return View{}, fmt.Errorf("decode GIF: %w", err)
		}
		if len(frames.Image) != 1 {
			return View{}, errors.New("animated GIF images are not supported")
		}
	}
	if isAnimatedWebP(data) {
		return View{}, errors.New("animated WebP images are not supported")
	}
	if mime == "image/png" && bytes.Contains(data, []byte("acTL")) {
		return View{}, errors.New("animated PNG images are not supported")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return View{}, fmt.Errorf("decode image config: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return View{}, errors.New("image has no pixels")
	}
	if int64(config.Width)*int64(config.Height) > options.MaxPixels {
		return View{}, fmt.Errorf("image exceeds %d pixels", options.MaxPixels)
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return View{}, fmt.Errorf("decode image: %w", err)
	}
	source = scale(source, options.MaxDimension)
	width := source.Bounds().Dx()
	height := source.Bounds().Dy()
	hasAlpha := imageHasAlpha(source)
	encoded, outputMIME, err := encode(source, hasAlpha, options.MaxBytes)
	if err != nil {
		return View{}, err
	}
	return View{
		Data: encoded, MIME: outputMIME, SourceMIME: mime,
		Width: width, Height: height,
	}, nil
}

func withDefaults(options Options) Options {
	if options.MaxBytes <= 0 {
		options.MaxBytes = DefaultMaxBytes
	}
	if options.MaxDimension <= 0 {
		options.MaxDimension = DefaultMaxDimension
	}
	if options.MaxPixels <= 0 {
		options.MaxPixels = DefaultMaxPixels
	}
	return options
}

func detectMIME(data []byte, filename string) string {
	mime := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))
	if mime != "application/octet-stream" && strings.HasPrefix(mime, "image/") {
		return mime
	}
	switch strings.ToLower(path.Ext(filename)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".tif", ".tiff":
		return "image/tiff"
	default:
		return mime
	}
}

func isAnimatedWebP(data []byte) bool {
	return len(data) > 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" &&
		bytes.Contains(data, []byte("ANIM"))
}

func scale(source image.Image, maxDimension int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= maxDimension && height <= maxDimension {
		return source
	}
	ratio := float64(maxDimension) / float64(max(width, height))
	targetWidth := max(1, int(float64(width)*ratio))
	targetHeight := max(1, int(float64(height)*ratio))
	target := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	draw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Over, nil)
	return target
}

func imageHasAlpha(source image.Image) bool {
	bounds := source.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := source.At(x, y).RGBA()
			if alpha != 0xffff {
				return true
			}
		}
	}
	return false
}

func encode(source image.Image, alpha bool, maxBytes int64) ([]byte, string, error) {
	dimension := max(source.Bounds().Dx(), source.Bounds().Dy())
	current := source
	for {
		var buffer bytes.Buffer
		mime := "image/jpeg"
		var err error
		if alpha {
			mime = "image/png"
			err = png.Encode(&buffer, current)
		} else {
			err = jpeg.Encode(&buffer, current, &jpeg.Options{Quality: 85})
		}
		if err != nil {
			return nil, "", err
		}
		if int64(buffer.Len()) <= maxBytes {
			return buffer.Bytes(), mime, nil
		}
		if dimension <= 64 {
			return nil, "", fmt.Errorf("image cannot fit within %d bytes", maxBytes)
		}
		dimension = int(float64(dimension) * 0.8)
		current = scale(current, dimension)
	}
}

// Copy reads one image and writes the normalized bytes to target.
func Copy(ctx context.Context, storage clarafs.FS, source, target clarafs.Path, options Options) (View, error) {
	view, err := Load(ctx, storage, source, options)
	if err != nil {
		return View{}, err
	}
	if _, err := storage.WriteFile(ctx, target, view.Data, clarafs.WriteOptions{
		Mode: 0o644, CreateDirs: true, Overwrite: true,
	}); err != nil {
		return View{}, err
	}
	return view, nil
}

// DecodeAll is exposed for tests and document renderers.
func DecodeAll(data []byte) (image.Image, string, error) {
	reader := bytes.NewReader(data)
	source, format, err := image.Decode(reader)
	if err != nil {
		return nil, "", err
	}
	return source, format, nil
}

// Drain reads and closes an image body.
func Drain(body io.ReadCloser, limit int64) ([]byte, error) {
	defer body.Close()
	if limit <= 0 {
		limit = DefaultMaxBytes
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("image exceeds %d bytes", limit)
	}
	return data, nil
}
