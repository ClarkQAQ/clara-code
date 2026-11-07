package openai

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"math"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the OpenAI API endpoint used when no base URL is set.
const DefaultBaseURL = "https://api.openai.com/v1"

const (
	defaultResponseHeaderTimeout = 10 * time.Minute
	maxRetryDelay                = 30 * time.Second
	initialRetryDelay            = 500 * time.Millisecond
)

// HTTPClient is the interface a caller-supplied HTTP client must implement.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// MiddlewareNext performs the rest of the request chain.
type MiddlewareNext func(*http.Request) (*http.Response, error)

// Middleware wraps the transport of a request, so callers can mutate the
// request before it is sent or inspect the response before it is decoded.
type Middleware func(*http.Request, MiddlewareNext) (*http.Response, error)

type requestConfig struct {
	apiKey       string
	baseURL      string
	headers      map[string]string
	client       HTTPClient
	maxRetries   int
	middlewares  []Middleware
	responseInto **http.Response
}

// RequestOption configures a client or a single request. Options passed to a
// call are applied on top of the client ones.
type RequestOption func(*requestConfig)

// WithAPIKey sets the bearer token used to authenticate requests.
func WithAPIKey(apiKey string) RequestOption {
	return func(c *requestConfig) { c.apiKey = apiKey }
}

// WithBaseURL sets the API endpoint, for example a self-hosted gateway.
func WithBaseURL(baseURL string) RequestOption {
	return func(c *requestConfig) { c.baseURL = baseURL }
}

// WithHeader sets a request header. Headers set per call override the client
// ones.
func WithHeader(key, value string) RequestOption {
	return func(c *requestConfig) {
		if c.headers == nil {
			c.headers = map[string]string{}
		}
		c.headers[key] = value
	}
}

// WithHTTPClient sets the HTTP client used to send requests.
func WithHTTPClient(client HTTPClient) RequestOption {
	return func(c *requestConfig) { c.client = client }
}

// WithMaxRetries sets how many times a request is retried when the endpoint
// reports a transient failure. The default is zero: retries are left to the
// caller.
func WithMaxRetries(maxRetries int) RequestOption {
	return func(c *requestConfig) { c.maxRetries = maxRetries }
}

// WithMiddleware appends middlewares to the request chain, in the order given.
func WithMiddleware(middlewares ...Middleware) RequestOption {
	return func(c *requestConfig) { c.middlewares = append(c.middlewares, middlewares...) }
}

// WithResponseInto stores the raw HTTP response of the call in resp, so
// callers can read response headers and trailers.
func WithResponseInto(resp **http.Response) RequestOption {
	return func(c *requestConfig) { c.responseInto = resp }
}

// Client talks to OpenAI or any OpenAI-compatible endpoint. The zero value is
// usable and points at the official API.
type Client struct {
	config requestConfig
}

// NewClient creates a client with the given options.
func NewClient(opts ...RequestOption) Client {
	client := Client{}
	for _, opt := range opts {
		opt(&client.config)
	}
	return client
}

// ChatCompletions creates a model response for a chat conversation.
func (c Client) ChatCompletions(ctx context.Context, params ChatCompletionNewParams, opts ...RequestOption) (*ChatCompletion, error) {
	var out ChatCompletion
	if err := c.execute(ctx, "chat/completions", params, false, opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ChatCompletionsStream creates a streamed chat completion.
func (c Client) ChatCompletionsStream(ctx context.Context, params ChatCompletionNewParams, opts ...RequestOption) *Stream[ChatCompletionChunk] {
	// The returned stream owns the response body and closes it.
	response, err := c.doRequest(ctx, "chat/completions", params, true, opts) //nolint:bodyclose
	return newStream[ChatCompletionChunk](response, err)
}

// Responses creates a model response.
func (c Client) Responses(ctx context.Context, params ResponseNewParams, opts ...RequestOption) (*Response, error) {
	var out Response
	if err := c.execute(ctx, "responses", params, false, opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ResponsesStream creates a streamed model response.
func (c Client) ResponsesStream(ctx context.Context, params ResponseNewParams, opts ...RequestOption) *Stream[StreamEvent] {
	// The returned stream owns the response body and closes it.
	response, err := c.doRequest(ctx, "responses", params, true, opts) //nolint:bodyclose
	return newStream[StreamEvent](response, err)
}

func (c Client) execute(ctx context.Context, path string, params any, stream bool, opts []RequestOption, out any) error {
	response, err := c.doRequest(ctx, path, params, stream, opts)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return err
	}
	return nil
}

// doRequest sends a request and returns the raw HTTP response. Responses that
// carry an error status are turned into an [Error], and the response body is
// buffered so it can be read again.
func (c Client) doRequest(ctx context.Context, path string, params any, stream bool, opts []RequestOption) (*http.Response, error) {
	config := c.withOptions(opts)
	body, err := marshalBody(params, stream)
	if err != nil {
		return nil, err
	}
	baseURL := config.baseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	url := strings.TrimSuffix(baseURL, "/") + "/" + path
	send := config.transport()

	var response *http.Response
	for attempt := 0; ; attempt++ {
		request, err := c.newRequest(ctx, url, body, config)
		if err != nil {
			return nil, err
		}
		response, err = send(request)
		if err != nil {
			if attempt >= config.maxRetries || !retryableRequestError(err) {
				return nil, err
			}
			if err := sleep(ctx, retryDelay(attempt)); err != nil {
				return nil, err
			}
			continue
		}
		if response.StatusCode < http.StatusBadRequest {
			break
		}
		if attempt >= config.maxRetries || !retryableStatusCode(response.StatusCode) {
			break
		}
		delay := retryDelay(attempt)
		if hinted, ok := retryAfter(response.Header); ok {
			delay = hinted
		}
		drainAndClose(response)
		if err := sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
	if config.responseInto != nil {
		*config.responseInto = response
	}
	if response.StatusCode >= http.StatusBadRequest {
		return nil, newAPIError(response)
	}
	return response, nil
}

// withOptions resolves the configuration of a call. Client-level maps and
// slices are copied first, so per-call options never leak into the client.
func (c Client) withOptions(opts []RequestOption) requestConfig {
	config := c.config
	if len(config.headers) > 0 {
		headers := make(map[string]string, len(config.headers))
		maps.Copy(headers, config.headers)
		config.headers = headers
	}
	if len(config.middlewares) > 0 {
		config.middlewares = slices.Clone(config.middlewares)
	}
	for _, opt := range opts {
		opt(&config)
	}
	return config
}

func (c Client) newRequest(ctx context.Context, url string, body []byte, config requestConfig) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	request.Body, _ = request.GetBody()
	request.ContentLength = int64(len(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if config.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+config.apiKey)
	}
	for key, value := range config.headers {
		request.Header.Set(key, value)
	}
	return request, nil
}

func (c requestConfig) transport() func(*http.Request) (*http.Response, error) {
	client := c.client
	if client == nil {
		client = defaultHTTPClient()
	}
	next := func(request *http.Request) (*http.Response, error) {
		return client.Do(request)
	}
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		middleware := c.middlewares[i]
		inner := next
		next = func(request *http.Request) (*http.Response, error) {
			return middleware(request, inner)
		}
	}
	return next
}

func defaultHTTPClient() HTTPClient {
	return defaultClientOnce()
}

var defaultClientOnce = sync.OnceValue(func() HTTPClient {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		cloned := transport.Clone()
		cloned.ResponseHeaderTimeout = defaultResponseHeaderTimeout
		return &http.Client{Transport: cloned}
	}
	return &http.Client{Transport: http.DefaultTransport}
})

// marshalBody encodes the request parameters, forcing stream mode when the
// call is streamed.
func marshalBody(params any, stream bool) ([]byte, error) {
	if !stream {
		return json.Marshal(params)
	}
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	fields["stream"] = json.RawMessage("true")
	return json.Marshal(fields)
}

func retryableStatusCode(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests:
		return true
	}
	return status >= http.StatusInternalServerError
}

func retryableRequestError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

// sleep waits before the next attempt, unless the context ends first.
func sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryDelay(attempt int) time.Duration {
	if attempt > 6 {
		attempt = 6
	}
	delay := time.Duration(math.Pow(2, float64(attempt))) * initialRetryDelay
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	return delay + jitter(delay/4)
}

func jitter(limit time.Duration) time.Duration {
	if limit <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0
	}
	return time.Duration(n.Int64())
}

func retryAfter(header http.Header) (time.Duration, bool) {
	if value := header.Get("retry-after-ms"); value != "" {
		if ms, err := strconv.ParseFloat(value, 64); err == nil && ms >= 0 {
			return time.Duration(ms * float64(time.Millisecond)), true
		}
	}
	if value := header.Get("retry-after"); value != "" {
		if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds >= 0 {
			return time.Duration(seconds * float64(time.Second)), true
		}
		if at, err := http.ParseTime(value); err == nil {
			if delay := time.Until(at); delay > 0 {
				return delay, true
			}
			return 0, true
		}
	}
	return 0, false
}

func drainAndClose(response *http.Response) {
	if response.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
}
