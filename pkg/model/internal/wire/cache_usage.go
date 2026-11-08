package openai

import "encoding/json"

func (details *ResponseUsageInputTokensDetails) UnmarshalJSON(data []byte) error {
	type raw ResponseUsageInputTokensDetails
	var value raw
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var presence struct {
		CachedTokens *int64 `json:"cached_tokens"`
	}
	if err := json.Unmarshal(data, &presence); err != nil {
		return err
	}
	*details = ResponseUsageInputTokensDetails(value)
	details.CachedTokensReported = presence.CachedTokens != nil
	return nil
}
