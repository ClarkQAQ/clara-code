package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
)

// Error is returned when the endpoint answers with an error status. The
// request and response are kept so callers can report what was sent and
// received.
type Error struct {
	Code       string
	Message    string
	Param      string
	Type       string
	StatusCode int
	Request    *http.Request
	Response   *http.Response

	raw []byte
}

// RawJSON returns the unmodified response body of the failed request.
func (e *Error) RawJSON() string { return string(e.raw) }

func (e *Error) Error() string {
	status := ""
	if e.Response != nil {
		status = fmt.Sprintf("%d %s", e.Response.StatusCode, http.StatusText(e.Response.StatusCode))
	}
	if e.Request != nil {
		return fmt.Sprintf("%s %q: %s %s", e.Request.Method, e.Request.URL, status, e.RawJSON())
	}
	return fmt.Sprintf("%s %s", status, e.RawJSON())
}

// DumpRequest returns the request that produced the error, optionally with its
// body.
func (e *Error) DumpRequest(body bool) []byte {
	if e.Request == nil {
		return nil
	}
	if e.Request.GetBody != nil {
		e.Request.Body, _ = e.Request.GetBody()
	}
	out, _ := httputil.DumpRequestOut(e.Request, body)
	return out
}

// DumpResponse returns the response that produced the error, optionally with
// its body.
func (e *Error) DumpResponse(body bool) []byte {
	if e.Response == nil {
		return nil
	}
	e.Response.Body = io.NopCloser(bytes.NewReader(e.raw))
	out, _ := httputil.DumpResponse(e.Response, body)
	e.Response.Body = io.NopCloser(bytes.NewReader(e.raw))
	return out
}

// newAPIError reads the failed response, keeping the body readable for callers
// that want to dump or reparse it.
func newAPIError(response *http.Response) *Error {
	contents, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(contents))

	err := &Error{
		StatusCode: response.StatusCode,
		Request:    response.Request,
		Response:   response,
		raw:        contents,
	}
	err.parse(contents)
	return err
}

func (e *Error) parse(contents []byte) {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Param   string `json:"param"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(contents, &envelope) == nil && envelope.Error.Message != "" {
		e.Code = envelope.Error.Code
		e.Message = envelope.Error.Message
		e.Param = envelope.Error.Param
		e.Type = envelope.Error.Type
		return
	}
	var bare struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(contents, &bare) == nil {
		e.Message = bare.Message
	}
}

// StreamError is returned when the endpoint reports an error in the middle of
// a server-sent event stream. It carries the failing event so the original
// payload stays available.
type StreamError struct {
	Message string
	Event   Event
}

func (e *StreamError) Error() string { return e.Message }
