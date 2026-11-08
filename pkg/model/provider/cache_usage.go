package provider

import "encoding/json"

func cacheReadReported(raw string) bool {
	var value struct {
		Details struct {
			CachedTokens *int64 `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	}
	return json.Unmarshal([]byte(raw), &value) == nil && value.Details.CachedTokens != nil
}
