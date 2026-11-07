package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
)

// Event is a single server-sent event.
type Event struct {
	Type string
	Data []byte
}

// Decoder reads server-sent events from a response body.
type Decoder interface {
	Next() bool
	Event() Event
	Close() error
	Err() error
}

// newDecoder returns a decoder for the response body, or nil when there is
// nothing to read.
func newDecoder(response *http.Response) Decoder {
	if response == nil || response.Body == nil {
		return nil
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(nil, bufio.MaxScanTokenSize<<9)
	return &eventStreamDecoder{body: response.Body, scanner: scanner}
}

type eventStreamDecoder struct {
	event   Event
	body    io.ReadCloser
	scanner *bufio.Scanner
	err     error
}

func (d *eventStreamDecoder) Next() bool {
	if d.err != nil {
		return false
	}
	eventType := ""
	var data []byte
	for d.scanner.Scan() {
		line := d.scanner.Bytes()
		if len(line) == 0 {
			if len(data) == 0 {
				eventType = ""
				continue
			}
			d.event = Event{Type: eventType, Data: data}
			return true
		}
		name, value, _ := bytes.Cut(line, []byte(":"))
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}
		switch string(name) {
		case "":
			// A comment line carries no data.
		case "event":
			eventType = string(value)
		case "data":
			data = append(data, value...)
			data = append(data, '\n')
		}
	}
	d.err = d.scanner.Err()
	return false
}

func (d *eventStreamDecoder) Event() Event { return d.event }
func (d *eventStreamDecoder) Close() error { return d.body.Close() }
func (d *eventStreamDecoder) Err() error   { return d.err }

// Stream iterates over a server-sent event stream.
type Stream[T any] struct {
	decoder   Decoder
	current   T
	err       error
	closeErr  error
	closeOnce sync.Once
	done      atomic.Bool
}

func newStream[T any](response *http.Response, err error) *Stream[T] {
	return &Stream[T]{decoder: newDecoder(response), err: err}
}

// Next advances the stream. It reports false once the stream ends, either
// normally or because of an error, which [Stream.Err] then returns.
func (s *Stream[T]) Next() bool {
	if s.err != nil {
		return s.finish(s.err)
	}
	if s.done.Load() || s.decoder == nil {
		return false
	}
	if !s.decoder.Next() {
		return s.finish(s.decoder.Err())
	}
	event := s.decoder.Event()
	if bytes.HasPrefix(event.Data, []byte("[DONE]")) {
		return s.finish(nil)
	}
	if errField := jsonField(event.Data, "error"); errField != nil && !bytes.Equal(bytes.TrimSpace(errField), []byte("null")) {
		return s.finish(&StreamError{
			Message: fmt.Sprintf("received error while streaming: %s", errField),
			Event:   event,
		})
	}
	var next T
	if err := json.Unmarshal(event.Data, &next); err != nil {
		return s.finish(err)
	}
	s.current = next
	return true
}

func (s *Stream[T]) finish(err error) bool {
	s.err = err
	_ = s.Close()
	return false
}

// Current returns the event decoded by the last call to [Stream.Next].
func (s *Stream[T]) Current() T { return s.current }

// Err returns the error that ended the stream, if any.
func (s *Stream[T]) Err() error { return s.err }

// Close releases the response body. Repeated calls return the first result.
func (s *Stream[T]) Close() error {
	s.closeOnce.Do(func() {
		s.done.Store(true)
		if s.decoder != nil {
			s.closeErr = s.decoder.Close()
		}
	})
	return s.closeErr
}
