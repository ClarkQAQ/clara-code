package completions

import (
	"bufio"
	"bytes"
	"clara-code/pkg/util/errs"
	"io"
)

// SSEDecoder reads Chat Completions server-sent events.
type SSEDecoder struct {
	scanner *bufio.Scanner
	body    io.ReadCloser
	data    []byte
	done    bool
	err     error
}

func NewSSEDecoder(body io.ReadCloser) *SSEDecoder {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	return &SSEDecoder{scanner: scanner, body: body}
}

// Next advances to the next data payload.
func (d *SSEDecoder) Next() bool {
	if d.done || d.err != nil {
		return false
	}
	var payload []byte
	for d.scanner.Scan() {
		line := d.scanner.Bytes()
		if len(line) == 0 {
			if len(payload) > 0 {
				d.data = bytes.TrimSpace(payload)
				if bytes.Equal(d.data, []byte("[DONE]")) {
					d.done = true
				}
				return true
			}
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		value := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload)+len(value)+1 > 16<<20 {
			d.err = errs.New(errs.CodeLimit, "SSE event exceeds the size limit")
			return false
		}
		payload = append(payload, value...)
		payload = append(payload, '\n')
	}
	if len(payload) > 0 {
		d.data = bytes.TrimSpace(payload)
		d.done = bytes.Equal(d.data, []byte("[DONE]"))
		return true
	}
	d.err = d.scanner.Err()
	if d.err == nil {
		d.err = io.EOF
	}
	return false
}

func (d *SSEDecoder) Data() []byte { return d.data }
func (d *SSEDecoder) Done() bool   { return d.done }
func (d *SSEDecoder) Err() error   { return d.err }
func (d *SSEDecoder) Close() error {
	if d.body == nil {
		return nil
	}
	return d.body.Close()
}
