package terminal

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

type key struct {
	code string
	text string
}

const inputLimit = 256 << 10

var pasteEnd = []byte("\x1b[201~")

type decoder struct {
	buffer  []byte
	paste   []byte
	pasting bool
}

func (d *decoder) feed(data []byte, timeout bool, emit func(key)) {
	d.buffer = append(d.buffer, data...)
	for len(d.buffer) > 0 {
		if d.pasting {
			if end := bytes.Index(d.buffer, pasteEnd); end >= 0 {
				d.addPaste(d.buffer[:end])
				emit(key{"paste", string(d.paste)})
				d.paste, d.pasting = nil, false
				d.buffer = d.buffer[end+len(pasteEnd):]
				continue
			}
			count := len(d.buffer) - len(pasteEnd) + 1
			if count > 0 {
				d.addPaste(d.buffer[:count])
				d.buffer = d.buffer[count:]
			}
			return
		}
		if d.buffer[0] != 0x1b {
			if !utf8.FullRune(d.buffer) && !timeout {
				return
			}
			r, size := utf8.DecodeRune(d.buffer)
			d.buffer = d.buffer[size:]
			switch r {
			case '\r':
				emit(key{code: "enter"})
			case '\n':
				emit(key{code: "newline"})
			case '\t':
				emit(key{code: "tab"})
			case 0x7f, 0x08:
				emit(key{code: "backspace"})
			case 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x0b, 0x0c, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a:
				emit(key{code: "ctrl-" + string('a'+r-1)})
			default:
				if r >= ' ' {
					emit(key{"text", string(r)})
				}
			}
			continue
		}
		if len(d.buffer) == 1 && !timeout {
			return
		}
		if len(d.buffer) >= 2 {
			if d.buffer[1] >= 'a' && d.buffer[1] <= 'z' {
				emit(key{code: "alt-" + string(d.buffer[1])})
				d.buffer = d.buffer[2:]
				continue
			}
			switch d.buffer[1] {
			case '\r', '\n':
				emit(key{code: "newline"})
				d.buffer = d.buffer[2:]
				continue
			case '[', 'O':
				end := 2
				for end < len(d.buffer) && (d.buffer[end] < 0x40 || d.buffer[end] > 0x7e) {
					end++
				}
				if end == len(d.buffer) && !timeout && end < 64 {
					return
				}
				if end < len(d.buffer) && end < 64 {
					sequence := string(d.buffer[2 : end+1])
					d.buffer = d.buffer[end+1:]
					if sequence == "200~" {
						d.pasting = true
					} else if code := sequenceKey(sequence); code != "" {
						emit(key{code: code})
					}
					continue
				}
			}
		}
		d.buffer = d.buffer[1:]
		emit(key{code: "escape"})
	}
}

func (d *decoder) addPaste(data []byte) {
	room := inputLimit - len(d.paste)
	d.paste = append(d.paste, data[:min(room, len(data))]...)
}

func sequenceKey(sequence string) string {
	switch sequence {
	case "A", "1;2A":
		return "up"
	case "B", "1;2B":
		return "down"
	case "C":
		return "right"
	case "D":
		return "left"
	case "H", "1~", "7~":
		return "home"
	case "F", "4~", "8~":
		return "end"
	case "3~":
		return "delete"
	case "5~":
		return "page-up"
	case "6~":
		return "page-down"
	case "Z", "1;2Z", "9;2u":
		return "backtab"
	case "13;2u", "13;3u", "27;2;13~", "27;3;13~":
		return "newline"
	case "13u":
		return "enter"
	}
	if strings.HasPrefix(sequence, "1;") {
		switch sequence[len(sequence)-1] {
		case 'C':
			return "word-right"
		case 'D':
			return "word-left"
		}
	}
	return ""
}

func readInput(ctx context.Context, file *os.File, keys chan<- key, failure chan<- error) {
	defer close(keys)
	fd := int(file.Fd())
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	var data [8192]byte
	var parser decoder
	for ctx.Err() == nil {
		wait := 100
		if len(parser.buffer) > 0 && !parser.pasting {
			wait = 35
		}
		n, err := unix.Poll(fds, wait)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			failure <- err
			return
		}
		emit := func(input key) {
			select {
			case keys <- input:
			case <-ctx.Done():
			}
		}
		if n == 0 {
			parser.feed(nil, true, emit)
			continue
		}
		size, err := unix.Read(fd, data[:])
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil {
			failure <- err
			return
		}
		if size == 0 {
			return
		}
		parser.feed(data[:size], false, emit)
	}
}
