package provider

import yuzu "clara-code/pkg/model"

// SearchContextSize controls how much context window space the
// web search tool uses. Maps to the OpenAI API's
// search_context_size parameter.
type SearchContextSize string

const (
	// SearchContextSizeLow uses minimal context for search results.
	SearchContextSizeLow SearchContextSize = "low"
	// SearchContextSizeMedium is the default context size.
	SearchContextSizeMedium SearchContextSize = "medium"
	// SearchContextSizeHigh uses maximal context for search results.
	SearchContextSizeHigh SearchContextSize = "high"
)

// WebSearchUserLocation provides geographic context for more
// relevant web search results.
type WebSearchUserLocation struct {
	City     string `json:"city,omitempty"`
	Region   string `json:"region,omitempty"`
	Country  string `json:"country,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

// WebSearchToolOptions configures the OpenAI web search tool.
type WebSearchToolOptions struct {
	// SearchContextSize controls the amount of context window
	// space used for search results. Defaults to medium.
	SearchContextSize SearchContextSize
	// AllowedDomains restricts search results to these domains.
	// Subdomains are included automatically.
	AllowedDomains []string
	// UserLocation provides geographic context for more
	// relevant search results.
	UserLocation *WebSearchUserLocation
}

// WebSearchTool creates a provider-defined web search tool for
// OpenAI models. Pass nil for default options.
func WebSearchTool(opts *WebSearchToolOptions) yuzu.ProviderDefinedTool {
	tool := yuzu.ProviderDefinedTool{
		ID:   "web_search",
		Name: "web_search",
	}
	if opts == nil {
		return tool
	}
	args := map[string]any{}
	if opts.SearchContextSize != "" {
		args["search_context_size"] = opts.SearchContextSize
	}
	if len(opts.AllowedDomains) > 0 {
		args["allowed_domains"] = opts.AllowedDomains
	}
	if opts.UserLocation != nil {
		args["user_location"] = opts.UserLocation
	}
	if len(args) > 0 {
		tool.Args = args
	}
	return tool
}
