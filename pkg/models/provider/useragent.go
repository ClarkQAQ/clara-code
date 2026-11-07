package provider

import (
	yuzu "clara-code/pkg/models"
	openai "clara-code/pkg/models/internal/wire"
	"strings"
)

// DefaultUserAgent is the User-Agent sent when the caller sets neither
// WithUserAgent nor a User-Agent header.
const DefaultUserAgent = "Yuzu (https://github.com/ClarkQAQ/yuzu)"

// ResolveHeaders returns a new header map, with a User-Agent field.
//
// Setting the value via WithUserAgent() takes precedence, however the
// user agent can also be set via HTTP headers (i.e. WithHeaders()).
// Otherwise, the default user agent will be used.
//
// Also note that the input map is never mutated.
func ResolveHeaders(headers map[string]string, explicitUA, defaultUA string) map[string]string {
	out := make(map[string]string, len(headers)+1)
	var uaKeys []string

	for k, v := range headers {
		out[k] = v
		if strings.EqualFold(k, "User-Agent") {
			uaKeys = append(uaKeys, k)
		}
	}

	switch {
	case explicitUA != "":
		for _, k := range uaKeys {
			delete(out, k)
		}
		out["User-Agent"] = explicitUA
	case len(uaKeys) > 0:
		val := out[uaKeys[0]]
		for _, k := range uaKeys {
			delete(out, k)
		}
		out["User-Agent"] = val
	default:
		out["User-Agent"] = defaultUA
	}

	return out
}

// CallUserAgent resolves the User-Agent for a single API call. It returns the
// resolved UA string and true if a per-call override should be applied, or
// empty string and false if the client-level UA should be used as-is.
func CallUserAgent(callUA string) (string, bool) {
	if callUA != "" {
		return callUA, true
	}
	return "", false
}

// CallHeaders resolves per-call headers. It returns the headers map and true
// if per-call headers should be applied, or nil and false if the client-level
// headers should be used as-is.
func CallHeaders(headers map[string]string) (map[string]string, bool) {
	if len(headers) > 0 {
		return headers, true
	}
	return nil, false
}

// callUARequestOptions returns per-request options that override the
// client-level User-Agent header when the Call carries agent-level UA
// settings.
func callUARequestOptions(call yuzu.Call) []openai.RequestOption {
	if ua, ok := CallUserAgent(call.UserAgent); ok {
		return []openai.RequestOption{openai.WithHeader("User-Agent", ua)}
	}
	return nil
}

// objectCallUARequestOptions returns per-request options that override the
// client-level User-Agent header when the ObjectCall carries agent-level UA
// settings.
func objectCallUARequestOptions(call yuzu.ObjectCall) []openai.RequestOption {
	if ua, ok := CallUserAgent(call.UserAgent); ok {
		return []openai.RequestOption{openai.WithHeader("User-Agent", ua)}
	}
	return nil
}

func callHeadersRequestOptions(call yuzu.Call) []openai.RequestOption {
	headers, ok := CallHeaders(call.Headers)
	if !ok {
		return nil
	}
	opts := make([]openai.RequestOption, 0, len(headers))
	for k, v := range headers {
		opts = append(opts, openai.WithHeader(k, v))
	}
	return opts
}

func objectCallHeadersRequestOptions(call yuzu.ObjectCall) []openai.RequestOption {
	headers, ok := CallHeaders(call.Headers)
	if !ok {
		return nil
	}
	opts := make([]openai.RequestOption, 0, len(headers))
	for k, v := range headers {
		opts = append(opts, openai.WithHeader(k, v))
	}
	return opts
}
