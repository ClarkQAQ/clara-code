package terminal

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

type key struct {
	code string
	text string
}

const inputLimit = 256 << 10

var pasteEnd = []byte("\x1b[201~")

func keyboardMode() (enable, restore string) {
	if os.Getenv("TMUX") != "" {
		// tmux requests extended keys per pane through modifyOtherKeys.
		return "\x1b[>4;1m", "\x1b[>4m"
	}
	// Push Kitty's disambiguation flag and pop it on exit.
	return "\x1b[>1u", "\x1b[<1u"
}

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
	case "A", "1A", "1;2A":
		return "up"
	case "B", "1B", "1;2B":
		return "down"
	case "C", "1C":
		return "right"
	case "D", "1D":
		return "left"
	case "H", "1H", "1~", "7~":
		return "home"
	case "F", "1F", "4~", "8~":
		return "end"
	case "3~":
		return "delete"
	case "5~":
		return "page-up"
	case "6~":
		return "page-down"
	case "Z", "1;2Z":
		return "backtab"
	}
	if strings.HasSuffix(sequence, "u") || strings.HasPrefix(sequence, "27;") && strings.HasSuffix(sequence, "~") {
		return extendedKey(sequence)
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

func extendedKey(sequence string) string {
	params := sequence[:len(sequence)-1]
	char, modifier, modified := strings.Cut(params, ";")
	if strings.HasSuffix(sequence, "~") {
		modifier, char, modified = strings.Cut(modifier, ";")
		if !modified {
			return ""
		}
	}
	mods := 0
	if modified {
		value, err := strconv.Atoi(modifier)
		if err != nil || value < 1 || value > 256 {
			return ""
		}
		mods = (value - 1) &^ (64 | 128) // Ignore Caps Lock and Num Lock.
	}
	if mods == 0 {
		switch char {
		case "57417":
			return "left"
		case "57418":
			return "right"
		case "57419":
			return "up"
		case "57420":
			return "down"
		case "57421":
			return "page-up"
		case "57422":
			return "page-down"
		case "57423":
			return "home"
		case "57424":
			return "end"
		case "57426":
			return "delete"
		}
	}
	switch char {
	case "13", "57414":
		switch mods {
		case 0:
			return "enter"
		case 1, 2, 3:
			return "newline"
		}
	case "27":
		if mods == 0 {
			return "escape"
		}
	case "9":
		switch mods {
		case 0:
			return "tab"
		case 1:
			return "backtab"
		}
	case "8", "127":
		if mods == 0 || mods == 4 {
			return "backspace"
		}
	}
	value, err := strconv.Atoi(char)
	if err != nil || value < 'a' || value > 'z' {
		return ""
	}
	switch mods {
	case 2:
		return "alt-" + string(rune(value))
	case 4:
		switch value {
		case 'h':
			return "backspace"
		case 'i':
			return "tab"
		case 'j':
			return "newline"
		case 'm':
			return "enter"
		default:
			return "ctrl-" + string(rune(value))
		}
	}
	return ""
}
