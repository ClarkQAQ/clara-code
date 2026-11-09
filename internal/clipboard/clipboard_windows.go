package clipboard

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image/png"
	"runtime"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/image/bmp"
	"golang.org/x/sys/windows"
)

const (
	clipboardDIB         = 8
	clipboardUnicodeText = 13
	clipboardDIBV5       = 17
	maxClipboardBytes    = 24 << 20
)

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	openClipboard            = user32.NewProc("OpenClipboard")
	closeClipboard           = user32.NewProc("CloseClipboard")
	clipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	getClipboardData         = user32.NewProc("GetClipboardData")
	registerClipboardFormat  = user32.NewProc("RegisterClipboardFormatW")
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	globalSize               = kernel32.NewProc("GlobalSize")
	globalLock               = kernel32.NewProc("GlobalLock")
	globalUnlock             = kernel32.NewProc("GlobalUnlock")
)

var clipboardReadSlot = make(chan struct{}, 1)

func readPlatform(ctx context.Context) (Content, error) {
	select {
	case clipboardReadSlot <- struct{}{}:
	case <-ctx.Done():
		return Content{}, ctx.Err()
	}
	type result struct {
		content Content
		err     error
	}
	completed := make(chan result, 1)
	go func() {
		defer func() { <-clipboardReadSlot }()
		content, err := readWindowsClipboard(ctx)
		completed <- result{content, err}
	}()
	select {
	case value := <-completed:
		return value.content, value.err
	case <-ctx.Done():
		return Content{}, ctx.Err()
	}
}

func readWindowsClipboard(ctx context.Context) (Content, error) {
	format, data, text, err := readClipboard(ctx)
	if err != nil {
		return Content{}, err
	}
	if len(data) == 0 {
		return Content{Text: text}, nil
	}
	if format != clipboardDIB && format != clipboardDIBV5 {
		return Content{Image: data}, nil
	}
	if err := ctx.Err(); err != nil {
		return Content{}, err
	}
	image, err := dibPNG(data)
	if err != nil && text != "" {
		return Content{Text: text}, nil
	}
	return Content{Image: image}, err
}

func readClipboard(ctx context.Context) (format uintptr, image []byte, text string, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for {
		if err := ctx.Err(); err != nil {
			return 0, nil, "", err
		}
		if ok, _, _ := openClipboard.Call(0); ok != 0 {
			break
		}
		select {
		case <-ctx.Done():
			return 0, nil, "", ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	defer closeClipboard.Call()

	pngName, _ := windows.UTF16PtrFromString("PNG")
	pngFormat, _, _ := registerClipboardFormat.Call(uintptr(unsafe.Pointer(pngName)))
	for _, candidate := range []uintptr{pngFormat, clipboardDIBV5, clipboardDIB} {
		data, readErr := clipboardBytes(candidate)
		if len(data) > 0 {
			format, image = candidate, data
			break
		}
		err = errors.Join(err, readErr)
	}
	rawText, textErr := clipboardBytes(clipboardUnicodeText)
	if len(rawText) > 0 {
		chars := make([]uint16, 0, len(rawText)/2)
		for len(rawText) >= 2 {
			char := binary.LittleEndian.Uint16(rawText)
			if char == 0 {
				break
			}
			chars = append(chars, char)
			rawText = rawText[2:]
		}
		text = string(utf16.Decode(chars))
	}
	if len(image) == 0 && text == "" {
		return 0, nil, "", errors.Join(err, textErr)
	}
	return format, image, text, nil
}

func clipboardBytes(format uintptr) ([]byte, error) {
	if format == 0 {
		return nil, nil
	}
	if available, _, _ := clipboardFormatAvailable.Call(format); available == 0 {
		return nil, nil
	}
	handle, _, err := getClipboardData.Call(format)
	if handle == 0 {
		return nil, err
	}
	size, _, err := globalSize.Call(handle)
	if size == 0 {
		return nil, err
	}
	if size > maxClipboardBytes {
		return nil, errors.New("clipboard exceeds 24 MiB")
	}
	address, _, err := globalLock.Call(handle)
	if address == 0 {
		return nil, err
	}
	defer globalUnlock.Call(handle)
	data := make([]byte, size)
	var copied uintptr
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), address, &data[0], size, &copied); err != nil {
		return nil, err
	}
	return data[:copied], nil
}

func dibPNG(data []byte) ([]byte, error) {
	const bmpHeader = 14
	if len(data) < 40 {
		return nil, errors.New("clipboard bitmap header is truncated")
	}
	header := binary.LittleEndian.Uint32(data)
	if header != 40 && header != 108 && header != 124 {
		return nil, errors.New("unsupported clipboard bitmap header")
	}
	bits := binary.LittleEndian.Uint16(data[14:])
	colors := binary.LittleEndian.Uint32(data[32:])
	if colors == 0 && bits > 0 && bits <= 8 {
		colors = 1 << bits
	}
	offset := uint64(header) + uint64(colors)*4
	if offset >= uint64(len(data)) {
		return nil, errors.New("clipboard bitmap is truncated")
	}
	file := make([]byte, bmpHeader, bmpHeader+len(data))
	copy(file, "BM")
	binary.LittleEndian.PutUint32(file[2:], uint32(bmpHeader+len(data)))
	binary.LittleEndian.PutUint32(file[10:], bmpHeader+uint32(offset))
	file = append(file, data...)

	config, err := bmp.DecodeConfig(bytes.NewReader(file))
	if err != nil {
		return nil, err
	}
	if config.Width == 0 || config.Height == 0 || int64(config.Width)*int64(config.Height) > 64_000_000 {
		return nil, errors.New("clipboard bitmap dimensions exceed the image limit")
	}
	image, err := bmp.Decode(bytes.NewReader(file))
	if err != nil {
		return nil, err
	}
	var output limitedBuffer
	if err := png.Encode(&output, image); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
