package responses

import (
	"bufio"
	"bytes"
	"clara-code/pkg/util/errs"
	"errors"
	"io"
)

// SSEDecoder reads OpenResponses server-sent events.
type SSEDecoder struct {
	scanner *bufio.Scanner
	body    io.ReadCloser
	event   SSEEvent
	err     error
}

// NewSSEDecoder reads events from body and closes it when exhausted.
func NewSSEDecoder(body io.ReadCloser) *SSEDecoder {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	return &SSEDecoder{scanner: scanner, body: body}
}

// Next advances to the next complete event.
func (d *SSEDecoder) Next() bool {
	if d.err != nil {
		return false
	}
	var eventType string
	var data []byte
	for d.scanner.Scan() {
		line := d.scanner.Bytes()
		if len(line) == 0 {
			if len(data) == 0 {
				eventType = ""
				continue
			}
			d.event = SSEEvent{Type: eventType, Data: bytes.TrimSpace(data)}
			return true
		}
		name, value, found := bytes.Cut(line, []byte(":"))
		if !found {
			continue
		}
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}
		switch string(name) {
		case "event":
			eventType = string(value)
		case "data":
			if len(data)+len(value)+1 > 16<<20 {
				d.err = errs.New(errs.CodeLimit, "SSE event exceeds the size limit")
				return false
			}
			data = append(data, value...)
			data = append(data, '\n')
		}
	}
	if len(data) > 0 {
		d.event = SSEEvent{Type: eventType, Data: bytes.TrimSpace(data)}
		return true
	}
	d.err = d.scanner.Err()
	if d.err == nil {
		d.err = io.EOF
	}
	return false
}

// Event returns the current event.
func (d *SSEDecoder) Event() SSEEvent { return d.event }

// Err reports the terminal read error.
func (d *SSEDecoder) Err() error {
	if d.err == nil {
		return nil
	}
	if errors.Is(d.err, io.EOF) {
		return io.EOF
	}
	return d.err
}

// Close closes the response body.
func (d *SSEDecoder) Close() error {
	if d.body == nil {
		return nil
	}
	return d.body.Close()
}
