package models

import "encoding/json"

// ResponsesFormat normalizes a structured output format for the Responses API.
func ResponsesFormat(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var format map[string]json.RawMessage
	if err := json.Unmarshal(raw, &format); err != nil {
		return nil, err
	}
	if nested := format["json_schema"]; len(nested) > 0 {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(nested, &fields); err != nil {
			return nil, err
		}
		for key, value := range fields {
			format[key] = value
		}
		delete(format, "json_schema")
		return json.Marshal(format)
	}
	return raw, nil
}

// CompletionsFormat normalizes a structured output format for Chat Completions.
func CompletionsFormat(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var format map[string]json.RawMessage
	if err := json.Unmarshal(raw, &format); err != nil {
		return nil, err
	}
	var kind string
	if value := format["type"]; len(value) > 0 {
		if err := json.Unmarshal(value, &kind); err != nil {
			return nil, err
		}
	}
	if kind == "json_schema" && len(format["json_schema"]) == 0 {
		delete(format, "type")
		return json.Marshal(map[string]any{"type": "json_schema", "json_schema": format})
	}
	return raw, nil
}
