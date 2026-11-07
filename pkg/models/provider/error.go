package provider

import (
	yuzu "clara-code/pkg/models"
	openai "clara-code/pkg/models/internal/wire"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var (
	contextLengthPattern    = regexp.MustCompile(`maximum context length (?:is|of) (\d+) tokens.*?(?:resulted in|requested) ~?(\d+) tokens`)
	inputLengthRangePattern = regexp.MustCompile(`Range of input length should be \[\d+,\s*(\d+)\]`)
	inputTooLongPattern     = regexp.MustCompile(`Input too long:\s*(\d+)\s*input tokens,\s*limit is\s*(\d+)`)
)

func toProviderErr(err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		message := toProviderErrMessage(apiErr)
		providerErr := &yuzu.ProviderError{
			Title:           cmp.Or(yuzu.ErrorTitleForStatusCode(apiErr.StatusCode), "provider request failed"),
			Message:         message,
			Cause:           apiErr,
			URL:             apiErr.Request.URL.String(),
			StatusCode:      apiErr.StatusCode,
			RequestBody:     apiErr.DumpRequest(true),
			ResponseHeaders: toHeaderMap(apiErr.Response.Header),
			ResponseBody:    apiErr.DumpResponse(true),
		}

		parseContextTooLargeError(message, providerErr)

		return providerErr
	}

	// Mid-stream SSE error events surface as *openai.StreamError, not
	// *openai.Error, so they need their own classification path.
	var streamErr *openai.StreamError
	if errors.As(err, &streamErr) {
		return toProviderErrFromStreamError(streamErr)
	}

	// Wrap transient transport failures so `.IsRetryable()` works.
	return yuzu.WrapTransportError(err)
}

// streamErrorEnvelope mirrors the OpenAI-standard error envelope
// (`{"error": {"code", "message", "type"}}`) that arrives as an in-band
// SSE event on stream failure.
type streamErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func toProviderErrFromStreamError(streamErr *openai.StreamError) *yuzu.ProviderError {
	var envelope streamErrorEnvelope
	_ = json.Unmarshal(streamErr.Event.Data, &envelope) // best-effort; falls back to the raw message on parse failure.

	errType := cmp.Or(envelope.Error.Type, envelope.Error.Code)

	return &yuzu.ProviderError{
		Title:          "stream error",
		Message:        cmp.Or(envelope.Error.Message, streamErr.Message),
		Cause:          streamErr,
		ResponseBody:   streamErr.Event.Data,
		TransientError: yuzu.TransientStreamErrorTypes[errType],
	}
}

func parseContextTooLargeError(message string, providerErr *yuzu.ProviderError) {
	if matches := contextLengthPattern.FindStringSubmatch(message); matches != nil {
		providerErr.ContextTooLargeErr = true
		providerErr.ContextMaxTokens, _ = strconv.Atoi(matches[1])
		providerErr.ContextUsedTokens, _ = strconv.Atoi(matches[2])
		return
	}
	if matches := inputLengthRangePattern.FindStringSubmatch(message); matches != nil {
		providerErr.ContextTooLargeErr = true
		providerErr.ContextMaxTokens, _ = strconv.Atoi(matches[1])
		return
	}
	if matches := inputTooLongPattern.FindStringSubmatch(message); matches != nil {
		providerErr.ContextTooLargeErr = true
		providerErr.ContextUsedTokens, _ = strconv.Atoi(matches[1])
		providerErr.ContextMaxTokens, _ = strconv.Atoi(matches[2])
	}
}

func toProviderErrMessage(apiErr *openai.Error) string {
	if apiErr.Message != "" {
		return apiErr.Message
	}

	// Some OpenAI-compatible providers send an error envelope the client cannot
	// parse, so fall back to the raw response body.
	data, _ := io.ReadAll(apiErr.Response.Body)
	return string(data)
}

func toHeaderMap(in http.Header) (out map[string]string) {
	out = make(map[string]string, len(in))
	for k, v := range in {
		if l := len(v); l > 0 {
			out[k] = v[l-1]
			in[strings.ToLower(k)] = v
		}
	}
	return out
}
