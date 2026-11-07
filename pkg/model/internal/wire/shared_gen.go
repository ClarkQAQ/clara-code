// Code generated from the openai-go v3.54.0 wire definitions. DO NOT EDIT.

package openai

import "encoding/json"

type ChatModel = string

type ComparisonFilterParam struct {
	Key   string                          `json:"key"`
	Type  ComparisonFilterType            `json:"type,omitzero"`
	Value ComparisonFilterValueUnionParam `json:"value,omitzero"`
	paramObj
}

func (p ComparisonFilterParam) MarshalJSON() ([]byte, error) {
	type raw ComparisonFilterParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ComparisonFilterType string

type ComparisonFilterValueArrayItemUnion struct {
	OfString string  `json:",inline"`
	OfFloat  float64 `json:",inline"`
}

func (u ComparisonFilterValueArrayItemUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfFloat)
}

func (u *ComparisonFilterValueArrayItemUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfFloat)
}

type ComparisonFilterValueArrayItemUnionParam struct {
	OfString Opt[string]  `json:",omitzero,inline"`
	OfFloat  Opt[float64] `json:",omitzero,inline"`
}

func (u ComparisonFilterValueArrayItemUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfFloat)
}

func (u *ComparisonFilterValueArrayItemUnionParam) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfFloat)
}

type ComparisonFilterValueUnion struct {
	OfString                     string                                `json:",inline"`
	OfFloat                      float64                               `json:",inline"`
	OfBool                       bool                                  `json:",inline"`
	OfComparisonFilterValueArray []ComparisonFilterValueArrayItemUnion `json:",inline"`
}

func (u ComparisonFilterValueUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfFloat, u.OfBool, u.OfComparisonFilterValueArray)
}

func (u *ComparisonFilterValueUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfFloat, &u.OfBool, &u.OfComparisonFilterValueArray)
}

type ComparisonFilterValueUnionParam struct {
	OfString                     Opt[string]                                `json:",omitzero,inline"`
	OfFloat                      Opt[float64]                               `json:",omitzero,inline"`
	OfBool                       Opt[bool]                                  `json:",omitzero,inline"`
	OfComparisonFilterValueArray []ComparisonFilterValueArrayItemUnionParam `json:",omitzero,inline"`
}

func (u ComparisonFilterValueUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfFloat, u.OfBool, u.OfComparisonFilterValueArray)
}

func (u *ComparisonFilterValueUnionParam) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfFloat, &u.OfBool, &u.OfComparisonFilterValueArray)
}

type CompoundFilter struct {
	Filters []CompoundFilterFilterUnion `json:"filters"`
	Type    CompoundFilterType          `json:"type"`
}

type CompoundFilterFilterUnion struct {
	Key              string                     `json:"key"`
	Type             ComparisonFilterType       `json:"type"`
	Value            ComparisonFilterValueUnion `json:"value"`
	OfCompoundFilter CompoundFilter             `json:",inline"`
}

func (u CompoundFilterFilterUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfCompoundFilter)
}

func (u *CompoundFilterFilterUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfCompoundFilter)
}

type CompoundFilterFilterUnionParam struct {
	OfComparison *ComparisonFilterParam `json:",omitzero,inline"`
	OfFilter     *CompoundFilterParam   `json:",omitzero,inline"`
}

func (u CompoundFilterFilterUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfComparison, u.OfFilter)
}

func (u *CompoundFilterFilterUnionParam) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfComparison, &u.OfFilter)
}

type CompoundFilterParam struct {
	Filters []CompoundFilterFilterUnionParam `json:"filters,omitzero"`
	Type    CompoundFilterType               `json:"type,omitzero"`
	paramObj
}

func (p CompoundFilterParam) MarshalJSON() ([]byte, error) {
	type raw CompoundFilterParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type CompoundFilterType string

type CustomToolInputFormatGrammarParam struct {
	Definition string `json:"definition"`
	Syntax     string `json:"syntax,omitzero"`
	Type       string `json:"type"`
	paramObj
}

func (p CustomToolInputFormatGrammarParam) MarshalJSON() ([]byte, error) {
	type raw CustomToolInputFormatGrammarParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "grammar"})
}

type CustomToolInputFormatTextParam struct {
	Type string `json:"type"`
	paramObj
}

func (p CustomToolInputFormatTextParam) MarshalJSON() ([]byte, error) {
	type raw CustomToolInputFormatTextParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "text"})
}

type CustomToolInputFormatUnion struct {
	Type       string `json:"type"`
	Definition string `json:"definition"`
	Syntax     string `json:"syntax"`
}

type CustomToolInputFormatUnionParam struct {
	OfText    *CustomToolInputFormatTextParam    `json:",omitzero,inline"`
	OfGrammar *CustomToolInputFormatGrammarParam `json:",omitzero,inline"`
}

func (u CustomToolInputFormatUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfText, u.OfGrammar)
}

func (u *CustomToolInputFormatUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "text":
		return json.Unmarshal(data, &u.OfText)
	case "grammar":
		return json.Unmarshal(data, &u.OfGrammar)
	}
	return tryUnmarshal(data)
}

type FunctionDefinitionParam struct {
	Name        string             `json:"name"`
	Strict      Opt[bool]          `json:"strict,omitzero"`
	Description Opt[string]        `json:"description,omitzero"`
	Parameters  FunctionParameters `json:"parameters,omitzero"`
	paramObj
}

func (p FunctionDefinitionParam) MarshalJSON() ([]byte, error) {
	type raw FunctionDefinitionParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type FunctionParameters map[string]any

type Metadata map[string]string

type Reasoning struct {
	Context         ReasoningContext         `json:"context"`
	Effort          ReasoningEffort          `json:"effort"`
	GenerateSummary ReasoningGenerateSummary `json:"generate_summary"`
	Mode            ReasoningMode            `json:"mode"`
	Summary         ReasoningSummary         `json:"summary"`
}

type ReasoningContext string

type ReasoningEffort string

type ReasoningGenerateSummary string

type ReasoningMode string

type ReasoningParam struct {
	Context         ReasoningContext         `json:"context,omitzero"`
	Effort          ReasoningEffort          `json:"effort,omitzero"`
	GenerateSummary ReasoningGenerateSummary `json:"generate_summary,omitzero"`
	Summary         ReasoningSummary         `json:"summary,omitzero"`
	Mode            ReasoningMode            `json:"mode,omitzero"`
	paramObj
}

func (p ReasoningParam) MarshalJSON() ([]byte, error) {
	type raw ReasoningParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ReasoningSummary string

type ResponseFormatJSONObjectParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseFormatJSONObjectParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFormatJSONObjectParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "json_object"})
}

type ResponseFormatJSONSchemaJSONSchemaParam struct {
	Name        string      `json:"name"`
	Strict      Opt[bool]   `json:"strict,omitzero"`
	Description Opt[string] `json:"description,omitzero"`
	Schema      any         `json:"schema,omitzero"`
	paramObj
}

func (p ResponseFormatJSONSchemaJSONSchemaParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFormatJSONSchemaJSONSchemaParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseFormatJSONSchemaParam struct {
	JSONSchema ResponseFormatJSONSchemaJSONSchemaParam `json:"json_schema,omitzero"`
	Type       string                                  `json:"type"`
	paramObj
}

func (p ResponseFormatJSONSchemaParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFormatJSONSchemaParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "json_schema"})
}

type ResponseFormatTextParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseFormatTextParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFormatTextParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "text"})
}

type ResponsesModel = string
