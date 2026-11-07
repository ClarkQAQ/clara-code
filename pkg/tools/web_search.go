package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 8 << 20

// Request is one provider-independent search.
type Request struct {
	Query string
	Count int
}

// Result is one search result.
type SearchResult struct {
	Title    string
	URL      string
	Summary  string
	Content  string
	SiteName string
	Date     string
	Score    float64
}

// Response is one search answer.
type Response struct {
	Answer  string
	Results []SearchResult
}

// Page is one fetched web page.
type Page struct {
	Title   string
	URL     string
	Content string
}

// Searcher performs web searches.
type Searcher interface {
	Search(context.Context, Request) (Response, error)
}

// Fetcher fetches a single page.
type Fetcher interface {
	Fetch(context.Context, string) (Page, error)
}

// HTTPDoer is the HTTP client contract.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// DeepSeekConfig configures the DeepSeek search and fetch API.
type DeepSeekConfig struct {
	BaseURL    string
	APIKey     string
	HTTPClient HTTPDoer
	UserAgent  string
}

// DeepSeek calls DeepSeek's search and fetch endpoints.
type DeepSeek struct {
	baseURL string
	apiKey  string
	client  HTTPDoer
	agent   string
}

// NewDeepSeek creates a DeepSeek provider.
func NewDeepSeek(config DeepSeekConfig) (*DeepSeek, error) {
	base := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if base == "" {
		return nil, errors.New("search: DeepSeek base URL is required")
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("search: invalid DeepSeek base URL %q", base)
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	if config.UserAgent == "" {
		config.UserAgent = "clara-core/dev"
	}
	return &DeepSeek{baseURL: base, apiKey: config.APIKey, client: config.HTTPClient, agent: config.UserAgent}, nil
}

func (d *DeepSeek) Search(ctx context.Context, request Request) (Response, error) {
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" {
		return Response{}, errors.New("search query is empty")
	}
	if request.Count <= 0 {
		request.Count = 10
	}
	if request.Count > 50 {
		request.Count = 50
	}
	var payload struct {
		Results []struct {
			Title       string  `json:"title"`
			URL         string  `json:"url"`
			Snippet     string  `json:"snippet"`
			Summary     string  `json:"summary"`
			Content     string  `json:"content"`
			SiteName    string  `json:"site_name"`
			Date        string  `json:"date"`
			RerankScore float64 `json:"rerank_score"`
		} `json:"results"`
	}
	if err := d.post(ctx, "/v1/tools/search", map[string]any{
		"query": request.Query, "count": request.Count, "need_content": false,
	}, &payload); err != nil {
		return Response{}, err
	}
	response := Response{Results: make([]SearchResult, 0, len(payload.Results))}
	for _, item := range payload.Results {
		response.Results = append(response.Results, SearchResult{
			Title: item.Title, URL: item.URL,
			Summary: first(item.Summary, item.Snippet), Content: item.Content,
			SiteName: item.SiteName, Date: item.Date, Score: item.RerankScore,
		})
	}
	return response, nil
}

func (d *DeepSeek) Fetch(ctx context.Context, rawURL string) (Page, error) {
	if err := validateURL(rawURL); err != nil {
		return Page{}, err
	}
	var payload struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := d.post(ctx, "/v1/tools/fetch", map[string]string{"url": rawURL}, &payload); err != nil {
		return Page{}, err
	}
	return Page{Title: payload.Title, URL: rawURL, Content: payload.Content}, nil
}

func (d *DeepSeek) post(ctx context.Context, endpoint string, input, output any) error {
	if d.apiKey == "" {
		return errors.New("search: API key is not configured")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+d.apiKey)
	request.Header.Set("User-Agent", d.agent)
	response, err := d.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(payload) > maxResponseBytes {
		return errors.New("search: response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("search: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	if err := json.Unmarshal(payload, output); err != nil {
		return fmt.Errorf("search: decode response: %w", err)
	}
	return nil
}

func validateURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil {
		return errors.New("URL cannot contain credentials")
	}
	return nil
}

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// Search groups search tools over one provider.
type Search struct {
	Provider Searcher
}

// New creates a search tool set.
func NewSearch(provider Searcher) *Search { return &Search{Provider: provider} }

// All returns web_search and web_fetch when supported.
func (s *Search) All() []Tool {
	if s == nil || s.Provider == nil {
		return nil
	}
	out := []Tool{s.WebSearch()}
	if _, ok := s.Provider.(Fetcher); ok {
		out = append(out, s.Fetch())
	}
	return out
}

func (s *Search) WebSearch() Tool {
	type input struct {
		Query string `json:"query" description:"search query, at most 128 characters"`
		Count int    `json:"count,omitempty" description:"number of results, 1 to 50"`
	}
	return Func(Definition{
		Name: "web_search",
		Description: "Search the web and return ranked results with source links and summaries. " +
			"Use web_fetch when a summary is not enough.",
		Effects: []Effect{EffectRead, EffectNetwork}, ParallelSafe: true,
	}, func(ctx context.Context, in input) (Result, error) {
		if len([]rune(strings.TrimSpace(in.Query))) > 128 {
			return ErrorResult("search query is longer than 128 characters"), nil
		}
		response, err := s.Provider.Search(ctx, Request(in))
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		return TextResult(renderSearch(response)), nil
	})
}

func (s *Search) Fetch() Tool {
	type input struct {
		URL string `json:"url" description:"absolute HTTP or HTTPS URL"`
	}
	return Func(Definition{
		Name: "web_fetch", Description: "Fetch one web page as Markdown-like text.",
		Effects: []Effect{EffectRead, EffectNetwork}, ParallelSafe: true, OutputLimit: 256 << 10,
	}, func(ctx context.Context, in input) (Result, error) {
		if err := validateURL(in.URL); err != nil {
			return ErrorResult(err.Error()), nil
		}
		page, err := s.Provider.(Fetcher).Fetch(ctx, in.URL)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		title := first(page.Title, page.URL)
		return TextResult(strings.TrimSpace(title + "\n" + page.URL + "\n\n" + page.Content)), nil
	})
}

func renderSearch(response Response) string {
	var builder strings.Builder
	if response.Answer != "" {
		builder.WriteString(response.Answer)
		builder.WriteString("\n\n")
	}
	for index, result := range response.Results {
		fmt.Fprintf(&builder, "%d. %s\n", index+1, first(result.Title, "Untitled"))
		if result.URL != "" {
			builder.WriteString(result.URL + "\n")
		}
		if summary := first(result.Summary, result.Content); summary != "" {
			builder.WriteString(summary + "\n")
		}
		if result.SiteName != "" || result.Date != "" {
			builder.WriteString(strings.TrimSpace(strings.Join([]string{result.SiteName, result.Date}, " · ")))
			builder.WriteByte('\n')
		}
		builder.WriteByte('\n')
	}
	if strings.TrimSpace(builder.String()) == "" {
		return "No web results were found."
	}
	return strings.TrimSpace(builder.String())
}
