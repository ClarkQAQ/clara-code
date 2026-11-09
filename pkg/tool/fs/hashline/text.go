package hashline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxFileBytes bounds text accepted by the editor.
const MaxFileBytes = 8 << 20

// Text is normalized file text split into 1-indexed lines.
type Text struct {
	Lines           []string
	TrailingNewline bool
}

// Normalize strips a BOM and converts line endings to LF.
func Normalize(raw string) string {
	raw = strings.TrimPrefix(raw, "\ufeff")
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	return strings.ReplaceAll(raw, "\r", "\n")
}

// ParseText normalizes and splits text without inventing a line for an empty file.
func ParseText(raw string) (Text, error) {
	text := Normalize(raw)
	if !utf8.ValidString(text) {
		return Text{}, fmt.Errorf("file is not valid UTF-8 text")
	}
	if len(text) > MaxFileBytes {
		return Text{}, fmt.Errorf("file is larger than %d bytes", MaxFileBytes)
	}
	if text == "" {
		return Text{}, nil
	}
	trailing := strings.HasSuffix(text, "\n")
	if trailing {
		text = strings.TrimSuffix(text, "\n")
	}
	lines := strings.Split(text, "\n")
	return Text{Lines: lines, TrailingNewline: trailing}, nil
}

// String reconstructs normalized text.
func (t Text) String() string {
	if len(t.Lines) == 0 {
		return ""
	}
	text := strings.Join(t.Lines, "\n")
	if t.TrailingNewline {
		text += "\n"
	}
	return text
}

// Tag computes the canonical four-character Hashline tag.
func Tag(raw string) string {
	text := Normalize(raw)
	normalized := make([]byte, 0, len(text))
	for _, row := range strings.SplitAfter(text, "\n") {
		line := strings.TrimSuffix(row, "\n")
		normalized = append(normalized, strings.TrimRight(line, " \t\r")...)
		if strings.HasSuffix(row, "\n") {
			normalized = append(normalized, '\n')
		}
	}
	return fmt.Sprintf("%04X", xxh32(normalized, 0)&0xffff)
}

func xxh32(input []byte, seed uint32) uint32 {
	const (
		prime1 uint32 = 2654435761
		prime2 uint32 = 2246822519
		prime3 uint32 = 3266489917
		prime4 uint32 = 668265263
		prime5 uint32 = 374761393
	)
	round := func(acc, value uint32) uint32 {
		acc += value * prime2
		acc = acc<<13 | acc>>19
		return acc * prime1
	}
	index := 0
	var hash uint32
	if len(input) >= 16 {
		v1 := seed + prime1 + prime2
		v2 := seed + prime2
		v3 := seed
		v4 := seed - prime1
		for index <= len(input)-16 {
			v1 = round(v1, littleEndian(input[index:]))
			v2 = round(v2, littleEndian(input[index+4:]))
			v3 = round(v3, littleEndian(input[index+8:]))
			v4 = round(v4, littleEndian(input[index+12:]))
			index += 16
		}
		hash = rotate(v1, 1) + rotate(v2, 7) + rotate(v3, 12) + rotate(v4, 18)
	} else {
		hash = seed + prime5
	}
	hash += uint32(len(input))
	for index <= len(input)-4 {
		hash += littleEndian(input[index:]) * prime3
		hash = rotate(hash, 17) * prime4
		index += 4
	}
	for index < len(input) {
		hash += uint32(input[index]) * prime5
		hash = rotate(hash, 11) * prime1
		index++
	}
	hash ^= hash >> 15
	hash *= prime2
	hash ^= hash >> 13
	hash *= prime3
	hash ^= hash >> 16
	return hash
}

func littleEndian(input []byte) uint32 {
	return uint32(input[0]) | uint32(input[1])<<8 | uint32(input[2])<<16 | uint32(input[3])<<24
}

func rotate(value uint32, bits uint) uint32 {
	return value<<bits | value>>(32-bits)
}

// preserveTextFormat reapplies the file's UTF-8 BOM and uniform CRLF endings.
func preserveTextFormat(original, text string) string {
	if strings.Contains(original, "\r\n") && !strings.ContainsAny(strings.ReplaceAll(original, "\r\n", ""), "\r\n") {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if strings.HasPrefix(original, "\ufeff") {
		text = "\ufeff" + text
	}
	return text
}
