// Code generated from the openai-go v3.54.0 wire definitions. DO NOT EDIT.

package openai

import "encoding/json"

type ApplyPatchToolParam struct {
	AllowedCallers []string `json:"allowed_callers,omitzero"`
	Type           string   `json:"type"`
	paramObj
}

func (p ApplyPatchToolParam) MarshalJSON() ([]byte, error) {
	type raw ApplyPatchToolParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "apply_patch"})
}

type ComputerActionClickParam struct {
	Button string   `json:"button,omitzero"`
	X      int64    `json:"x"`
	Y      int64    `json:"y"`
	Keys   []string `json:"keys,omitzero"`
	Type   string   `json:"type"`
	paramObj
}

func (p ComputerActionClickParam) MarshalJSON() ([]byte, error) {
	type raw ComputerActionClickParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "click"})
}

type ComputerActionDoubleClickParam struct {
	Keys []string `json:"keys,omitzero"`
	X    int64    `json:"x"`
	Y    int64    `json:"y"`
	Type string   `json:"type"`
	paramObj
}

func (p ComputerActionDoubleClickParam) MarshalJSON() ([]byte, error) {
	type raw ComputerActionDoubleClickParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "double_click"})
}

type ComputerActionDragParam struct {
	Path []ComputerActionDragPathParam `json:"path,omitzero"`
	Keys []string                      `json:"keys,omitzero"`
	Type string                        `json:"type"`
	paramObj
}

func (p ComputerActionDragParam) MarshalJSON() ([]byte, error) {
	type raw ComputerActionDragParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "drag"})
}

type ComputerActionDragPath struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

type ComputerActionDragPathParam struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
	paramObj
}

func (p ComputerActionDragPathParam) MarshalJSON() ([]byte, error) {
	type raw ComputerActionDragPathParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ComputerActionKeypressParam struct {
	Keys []string `json:"keys,omitzero"`
	Type string   `json:"type"`
	paramObj
}

func (p ComputerActionKeypressParam) MarshalJSON() ([]byte, error) {
	type raw ComputerActionKeypressParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "keypress"})
}

type ComputerActionList []ComputerActionUnion

type ComputerActionListParam []ComputerActionUnionParam

type ComputerActionMoveParam struct {
	X    int64    `json:"x"`
	Y    int64    `json:"y"`
	Keys []string `json:"keys,omitzero"`
	Type string   `json:"type"`
	paramObj
}

func (p ComputerActionMoveParam) MarshalJSON() ([]byte, error) {
	type raw ComputerActionMoveParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "move"})
}

type ComputerActionScreenshotParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ComputerActionScreenshotParam) MarshalJSON() ([]byte, error) {
	type raw ComputerActionScreenshotParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "screenshot"})
}

type ComputerActionScrollParam struct {
	ScrollX int64    `json:"scroll_x"`
	ScrollY int64    `json:"scroll_y"`
	X       int64    `json:"x"`
	Y       int64    `json:"y"`
	Keys    []string `json:"keys,omitzero"`
	Type    string   `json:"type"`
	paramObj
}

func (p ComputerActionScrollParam) MarshalJSON() ([]byte, error) {
	type raw ComputerActionScrollParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "scroll"})
}

type ComputerActionTypeParam struct {
	Text string `json:"text"`
	Type string `json:"type"`
	paramObj
}

func (p ComputerActionTypeParam) MarshalJSON() ([]byte, error) {
	type raw ComputerActionTypeParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "type"})
}

type ComputerActionUnion struct {
	Button  string                   `json:"button"`
	Type    string                   `json:"type"`
	X       int64                    `json:"x"`
	Y       int64                    `json:"y"`
	Keys    []string                 `json:"keys"`
	Path    []ComputerActionDragPath `json:"path"`
	ScrollX int64                    `json:"scroll_x"`
	ScrollY int64                    `json:"scroll_y"`
	Text    string                   `json:"text"`
}

type ComputerActionUnionParam struct {
	OfClick       *ComputerActionClickParam       `json:",omitzero,inline"`
	OfDoubleClick *ComputerActionDoubleClickParam `json:",omitzero,inline"`
	OfDrag        *ComputerActionDragParam        `json:",omitzero,inline"`
	OfKeypress    *ComputerActionKeypressParam    `json:",omitzero,inline"`
	OfMove        *ComputerActionMoveParam        `json:",omitzero,inline"`
	OfScreenshot  *ComputerActionScreenshotParam  `json:",omitzero,inline"`
	OfScroll      *ComputerActionScrollParam      `json:",omitzero,inline"`
	OfType        *ComputerActionTypeParam        `json:",omitzero,inline"`
	OfWait        *ComputerActionWaitParam        `json:",omitzero,inline"`
}

func (u ComputerActionUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfClick, u.OfDoubleClick, u.OfDrag, u.OfKeypress, u.OfMove, u.OfScreenshot, u.OfScroll, u.OfType, u.OfWait)
}

func (u *ComputerActionUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "click":
		return json.Unmarshal(data, &u.OfClick)
	case "double_click":
		return json.Unmarshal(data, &u.OfDoubleClick)
	case "drag":
		return json.Unmarshal(data, &u.OfDrag)
	case "keypress":
		return json.Unmarshal(data, &u.OfKeypress)
	case "move":
		return json.Unmarshal(data, &u.OfMove)
	case "screenshot":
		return json.Unmarshal(data, &u.OfScreenshot)
	case "scroll":
		return json.Unmarshal(data, &u.OfScroll)
	case "type":
		return json.Unmarshal(data, &u.OfType)
	case "wait":
		return json.Unmarshal(data, &u.OfWait)
	}
	return tryUnmarshal(data)
}

type ComputerActionWaitParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ComputerActionWaitParam) MarshalJSON() ([]byte, error) {
	type raw ComputerActionWaitParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "wait"})
}

type ComputerToolParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ComputerToolParam) MarshalJSON() ([]byte, error) {
	type raw ComputerToolParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "computer"})
}

type ComputerUsePreviewToolEnvironment string

type ComputerUsePreviewToolParam struct {
	DisplayHeight int64                             `json:"display_height"`
	DisplayWidth  int64                             `json:"display_width"`
	Environment   ComputerUsePreviewToolEnvironment `json:"environment,omitzero"`
	Type          string                            `json:"type"`
	paramObj
}

func (p ComputerUsePreviewToolParam) MarshalJSON() ([]byte, error) {
	type raw ComputerUsePreviewToolParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "computer_use_preview"})
}

type ContainerAutoMemoryLimit string

type ContainerAutoNetworkPolicyUnion struct {
	Type           string                               `json:"type"`
	AllowedDomains []string                             `json:"allowed_domains"`
	DomainSecrets  []ContainerNetworkPolicyDomainSecret `json:"domain_secrets"`
}

type ContainerAutoNetworkPolicyUnionParam struct {
	OfDisabled  *ContainerNetworkPolicyDisabledParam  `json:",omitzero,inline"`
	OfAllowlist *ContainerNetworkPolicyAllowlistParam `json:",omitzero,inline"`
}

func (u ContainerAutoNetworkPolicyUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfDisabled, u.OfAllowlist)
}

func (u *ContainerAutoNetworkPolicyUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "disabled":
		return json.Unmarshal(data, &u.OfDisabled)
	case "allowlist":
		return json.Unmarshal(data, &u.OfAllowlist)
	}
	return tryUnmarshal(data)
}

type ContainerAutoParam struct {
	MemoryLimit   ContainerAutoMemoryLimit             `json:"memory_limit,omitzero"`
	FileIDs       []string                             `json:"file_ids,omitzero"`
	NetworkPolicy ContainerAutoNetworkPolicyUnionParam `json:"network_policy,omitzero"`
	Skills        []ContainerAutoSkillUnionParam       `json:"skills,omitzero"`
	Type          string                               `json:"type"`
	paramObj
}

func (p ContainerAutoParam) MarshalJSON() ([]byte, error) {
	type raw ContainerAutoParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "container_auto"})
}

type ContainerAutoSkillUnion struct {
	SkillID     string            `json:"skill_id"`
	Type        string            `json:"type"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Name        string            `json:"name"`
	Source      InlineSkillSource `json:"source"`
}

type ContainerAutoSkillUnionParam struct {
	OfSkillReference *SkillReferenceParam `json:",omitzero,inline"`
	OfInline         *InlineSkillParam    `json:",omitzero,inline"`
}

func (u ContainerAutoSkillUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfSkillReference, u.OfInline)
}

func (u *ContainerAutoSkillUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "skill_reference":
		return json.Unmarshal(data, &u.OfSkillReference)
	case "inline":
		return json.Unmarshal(data, &u.OfInline)
	}
	return tryUnmarshal(data)
}

type ContainerNetworkPolicyAllowlistParam struct {
	AllowedDomains []string                                  `json:"allowed_domains,omitzero"`
	DomainSecrets  []ContainerNetworkPolicyDomainSecretParam `json:"domain_secrets,omitzero"`
	Type           string                                    `json:"type"`
	paramObj
}

func (p ContainerNetworkPolicyAllowlistParam) MarshalJSON() ([]byte, error) {
	type raw ContainerNetworkPolicyAllowlistParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "allowlist"})
}

type ContainerNetworkPolicyDisabledParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ContainerNetworkPolicyDisabledParam) MarshalJSON() ([]byte, error) {
	type raw ContainerNetworkPolicyDisabledParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "disabled"})
}

type ContainerNetworkPolicyDomainSecret struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Value  string `json:"value"`
}

type ContainerNetworkPolicyDomainSecretParam struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	paramObj
}

func (p ContainerNetworkPolicyDomainSecretParam) MarshalJSON() ([]byte, error) {
	type raw ContainerNetworkPolicyDomainSecretParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ContainerReferenceParam struct {
	ContainerID string `json:"container_id"`
	Type        string `json:"type"`
	paramObj
}

func (p ContainerReferenceParam) MarshalJSON() ([]byte, error) {
	type raw ContainerReferenceParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "container_reference"})
}

type CustomToolParam struct {
	Name           string                          `json:"name"`
	DeferLoading   Opt[bool]                       `json:"defer_loading,omitzero"`
	Description    Opt[string]                     `json:"description,omitzero"`
	AllowedCallers []string                        `json:"allowed_callers,omitzero"`
	Format         CustomToolInputFormatUnionParam `json:"format,omitzero"`
	Type           string                          `json:"type"`
	paramObj
}

func (p CustomToolParam) MarshalJSON() ([]byte, error) {
	type raw CustomToolParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "custom"})
}

type EasyInputMessageContentUnionParam struct {
	OfString               Opt[string]                          `json:",omitzero,inline"`
	OfInputItemContentList ResponseInputMessageContentListParam `json:",omitzero,inline"`
}

func (u EasyInputMessageContentUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfInputItemContentList)
}

func (u *EasyInputMessageContentUnionParam) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfInputItemContentList)
}

type EasyInputMessageParam struct {
	Content EasyInputMessageContentUnionParam `json:"content,omitzero"`
	Role    EasyInputMessageRole              `json:"role,omitzero"`
	Phase   EasyInputMessagePhase             `json:"phase,omitzero"`
	Type    EasyInputMessageType              `json:"type,omitzero"`
	paramObj
}

func (p EasyInputMessageParam) MarshalJSON() ([]byte, error) {
	type raw EasyInputMessageParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type EasyInputMessagePhase string

type EasyInputMessageRole string

type EasyInputMessageType string

type FileSearchToolFiltersUnionParam struct {
	OfComparisonFilter *ComparisonFilterParam `json:",omitzero,inline"`
	OfCompoundFilter   *CompoundFilterParam   `json:",omitzero,inline"`
}

func (u FileSearchToolFiltersUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfComparisonFilter, u.OfCompoundFilter)
}

func (u *FileSearchToolFiltersUnionParam) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfComparisonFilter, &u.OfCompoundFilter)
}

type FileSearchToolParam struct {
	VectorStoreIDs []string                          `json:"vector_store_ids,omitzero"`
	MaxNumResults  Opt[int64]                        `json:"max_num_results,omitzero"`
	Filters        FileSearchToolFiltersUnionParam   `json:"filters,omitzero"`
	RankingOptions FileSearchToolRankingOptionsParam `json:"ranking_options,omitzero"`
	Type           string                            `json:"type"`
	paramObj
}

func (p FileSearchToolParam) MarshalJSON() ([]byte, error) {
	type raw FileSearchToolParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "file_search"})
}

type FileSearchToolRankingOptions struct {
	HybridSearch   FileSearchToolRankingOptionsHybridSearch `json:"hybrid_search"`
	Ranker         string                                   `json:"ranker"`
	ScoreThreshold float64                                  `json:"score_threshold"`
}

type FileSearchToolRankingOptionsHybridSearch struct {
	EmbeddingWeight float64 `json:"embedding_weight"`
	TextWeight      float64 `json:"text_weight"`
}

type FileSearchToolRankingOptionsHybridSearchParam struct {
	EmbeddingWeight float64 `json:"embedding_weight"`
	TextWeight      float64 `json:"text_weight"`
	paramObj
}

func (p FileSearchToolRankingOptionsHybridSearchParam) MarshalJSON() ([]byte, error) {
	type raw FileSearchToolRankingOptionsHybridSearchParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type FileSearchToolRankingOptionsParam struct {
	ScoreThreshold Opt[float64]                                  `json:"score_threshold,omitzero"`
	HybridSearch   FileSearchToolRankingOptionsHybridSearchParam `json:"hybrid_search,omitzero"`
	Ranker         string                                        `json:"ranker,omitzero"`
	paramObj
}

func (p FileSearchToolRankingOptionsParam) MarshalJSON() ([]byte, error) {
	type raw FileSearchToolRankingOptionsParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type FunctionShellToolEnvironmentUnionParam struct {
	OfContainerAuto      *ContainerAutoParam      `json:",omitzero,inline"`
	OfLocal              *LocalEnvironmentParam   `json:",omitzero,inline"`
	OfContainerReference *ContainerReferenceParam `json:",omitzero,inline"`
}

func (u FunctionShellToolEnvironmentUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfContainerAuto, u.OfLocal, u.OfContainerReference)
}

func (u *FunctionShellToolEnvironmentUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "container_auto":
		return json.Unmarshal(data, &u.OfContainerAuto)
	case "local":
		return json.Unmarshal(data, &u.OfLocal)
	case "container_reference":
		return json.Unmarshal(data, &u.OfContainerReference)
	}
	return tryUnmarshal(data)
}

type FunctionShellToolParam struct {
	AllowedCallers []string                               `json:"allowed_callers,omitzero"`
	Environment    FunctionShellToolEnvironmentUnionParam `json:"environment,omitzero"`
	Type           string                                 `json:"type"`
	paramObj
}

func (p FunctionShellToolParam) MarshalJSON() ([]byte, error) {
	type raw FunctionShellToolParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "shell"})
}

type FunctionToolParam struct {
	Strict         Opt[bool]      `json:"strict,omitzero"`
	Parameters     map[string]any `json:"parameters,omitzero"`
	Name           string         `json:"name"`
	Description    Opt[string]    `json:"description,omitzero"`
	DeferLoading   Opt[bool]      `json:"defer_loading,omitzero"`
	AllowedCallers []string       `json:"allowed_callers,omitzero"`
	OutputSchema   map[string]any `json:"output_schema,omitzero"`
	Type           string         `json:"type"`
	paramObj
}

func (p FunctionToolParam) MarshalJSON() ([]byte, error) {
	type raw FunctionToolParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "function"})
}

type InlineSkillParam struct {
	Description string                 `json:"description"`
	Name        string                 `json:"name"`
	Source      InlineSkillSourceParam `json:"source,omitzero"`
	Type        string                 `json:"type"`
	paramObj
}

func (p InlineSkillParam) MarshalJSON() ([]byte, error) {
	type raw InlineSkillParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "inline"})
}

type InlineSkillSource struct {
	Data      string `json:"data"`
	MediaType string `json:"media_type"`
	Type      string `json:"type"`
}

type InlineSkillSourceParam struct {
	Data      string `json:"data"`
	MediaType string `json:"media_type"`
	Type      string `json:"type"`
	paramObj
}

func (p InlineSkillSourceParam) MarshalJSON() ([]byte, error) {
	type raw InlineSkillSourceParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"media_type": "application/zip", "type": "base64"})
}

type LocalEnvironmentParam struct {
	Skills []LocalSkillParam `json:"skills,omitzero"`
	Type   string            `json:"type"`
	paramObj
}

func (p LocalEnvironmentParam) MarshalJSON() ([]byte, error) {
	type raw LocalEnvironmentParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "local"})
}

type LocalSkill struct {
	Description string `json:"description"`
	Name        string `json:"name"`
	Path        string `json:"path"`
}

type LocalSkillParam struct {
	Description string `json:"description"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	paramObj
}

func (p LocalSkillParam) MarshalJSON() ([]byte, error) {
	type raw LocalSkillParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type McpToolCallErrorHTTPErrorParam struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
	paramObj
}

func (p McpToolCallErrorHTTPErrorParam) MarshalJSON() ([]byte, error) {
	type raw McpToolCallErrorHTTPErrorParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "http_error"})
}

type McpToolCallErrorMcpProtocolErrorParam struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
	paramObj
}

func (p McpToolCallErrorMcpProtocolErrorParam) MarshalJSON() ([]byte, error) {
	type raw McpToolCallErrorMcpProtocolErrorParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "mcp_protocol_error"})
}

type McpToolCallErrorMcpToolExecutionErrorParam struct {
	Content any    `json:"content,omitzero"`
	Type    string `json:"type"`
	paramObj
}

func (p McpToolCallErrorMcpToolExecutionErrorParam) MarshalJSON() ([]byte, error) {
	type raw McpToolCallErrorMcpToolExecutionErrorParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "mcp_tool_execution_error"})
}

type McpToolCallErrorUnionParam struct {
	OfMcpProtocolError      *McpToolCallErrorMcpProtocolErrorParam      `json:",omitzero,inline"`
	OfMcpToolExecutionError *McpToolCallErrorMcpToolExecutionErrorParam `json:",omitzero,inline"`
	OfHTTPError             *McpToolCallErrorHTTPErrorParam             `json:",omitzero,inline"`
}

func (u McpToolCallErrorUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfMcpProtocolError, u.OfMcpToolExecutionError, u.OfHTTPError)
}

func (u *McpToolCallErrorUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "mcp_protocol_error":
		return json.Unmarshal(data, &u.OfMcpProtocolError)
	case "mcp_tool_execution_error":
		return json.Unmarshal(data, &u.OfMcpToolExecutionError)
	case "http_error":
		return json.Unmarshal(data, &u.OfHTTPError)
	}
	return tryUnmarshal(data)
}

type NamespaceToolParam struct {
	Description string                        `json:"description"`
	Name        string                        `json:"name"`
	Tools       []NamespaceToolToolUnionParam `json:"tools,omitzero"`
	Type        string                        `json:"type"`
	paramObj
}

func (p NamespaceToolParam) MarshalJSON() ([]byte, error) {
	type raw NamespaceToolParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "namespace"})
}

type NamespaceToolToolFunctionParam struct {
	Name           string         `json:"name"`
	Description    Opt[string]    `json:"description,omitzero"`
	Strict         Opt[bool]      `json:"strict,omitzero"`
	DeferLoading   Opt[bool]      `json:"defer_loading,omitzero"`
	AllowedCallers []string       `json:"allowed_callers,omitzero"`
	OutputSchema   map[string]any `json:"output_schema,omitzero"`
	Parameters     any            `json:"parameters,omitzero"`
	Type           string         `json:"type"`
	paramObj
}

func (p NamespaceToolToolFunctionParam) MarshalJSON() ([]byte, error) {
	type raw NamespaceToolToolFunctionParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "function"})
}

type NamespaceToolToolUnion struct {
	Name           string                     `json:"name"`
	Type           string                     `json:"type"`
	AllowedCallers []string                   `json:"allowed_callers"`
	DeferLoading   bool                       `json:"defer_loading"`
	Description    string                     `json:"description"`
	OutputSchema   map[string]any             `json:"output_schema"`
	Parameters     any                        `json:"parameters"`
	Strict         bool                       `json:"strict"`
	Format         CustomToolInputFormatUnion `json:"format"`
}

type NamespaceToolToolUnionParam struct {
	OfFunction *NamespaceToolToolFunctionParam `json:",omitzero,inline"`
	OfCustom   *CustomToolParam                `json:",omitzero,inline"`
}

func (u NamespaceToolToolUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfFunction, u.OfCustom)
}

func (u *NamespaceToolToolUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "function":
		return json.Unmarshal(data, &u.OfFunction)
	case "custom":
		return json.Unmarshal(data, &u.OfCustom)
	}
	return tryUnmarshal(data)
}

type Response struct {
	ID                   string                       `json:"id"`
	CreatedAt            float64                      `json:"created_at"`
	Error                ResponseError                `json:"error"`
	IncompleteDetails    ResponseIncompleteDetails    `json:"incomplete_details"`
	Instructions         ResponseInstructionsUnion    `json:"instructions"`
	Metadata             Metadata                     `json:"metadata"`
	Model                ResponsesModel               `json:"model"`
	Object               string                       `json:"object"`
	Output               []ResponseOutputItemUnion    `json:"output"`
	ParallelToolCalls    bool                         `json:"parallel_tool_calls"`
	Temperature          float64                      `json:"temperature"`
	ToolChoice           ResponseToolChoiceUnion      `json:"tool_choice"`
	Tools                []ToolUnion                  `json:"tools"`
	TopP                 float64                      `json:"top_p"`
	Background           bool                         `json:"background"`
	CompletedAt          float64                      `json:"completed_at"`
	Conversation         ResponseConversation         `json:"conversation"`
	MaxOutputTokens      int64                        `json:"max_output_tokens"`
	MaxToolCalls         int64                        `json:"max_tool_calls"`
	Moderation           ResponseModeration           `json:"moderation"`
	PreviousResponseID   string                       `json:"previous_response_id"`
	Prompt               ResponsePrompt               `json:"prompt"`
	PromptCacheKey       string                       `json:"prompt_cache_key"`
	PromptCacheOptions   ResponsePromptCacheOptions   `json:"prompt_cache_options"`
	PromptCacheRetention ResponsePromptCacheRetention `json:"prompt_cache_retention"`
	Reasoning            Reasoning                    `json:"reasoning"`
	SafetyIdentifier     string                       `json:"safety_identifier"`
	ServiceTier          ResponseServiceTier          `json:"service_tier"`
	Status               ResponseStatus               `json:"status"`
	Text                 ResponseTextConfig           `json:"text"`
	TopLogprobs          int64                        `json:"top_logprobs"`
	Truncation           ResponseTruncation           `json:"truncation"`
	Usage                ResponseUsage                `json:"usage"`
	User                 string                       `json:"user"`
}

type ResponseApplyPatchToolCallOperationUnion struct {
	Diff string `json:"diff"`
	Path string `json:"path"`
	Type string `json:"type"`
}

type ResponseCodeInterpreterToolCallOutputImageParam struct {
	URL  string `json:"url"`
	Type string `json:"type"`
	paramObj
}

func (p ResponseCodeInterpreterToolCallOutputImageParam) MarshalJSON() ([]byte, error) {
	type raw ResponseCodeInterpreterToolCallOutputImageParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "image"})
}

type ResponseCodeInterpreterToolCallOutputLogsParam struct {
	Logs string `json:"logs"`
	Type string `json:"type"`
	paramObj
}

func (p ResponseCodeInterpreterToolCallOutputLogsParam) MarshalJSON() ([]byte, error) {
	type raw ResponseCodeInterpreterToolCallOutputLogsParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "logs"})
}

type ResponseCodeInterpreterToolCallOutputUnion struct {
	Logs string `json:"logs"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

type ResponseCodeInterpreterToolCallOutputUnionParam struct {
	OfLogs  *ResponseCodeInterpreterToolCallOutputLogsParam  `json:",omitzero,inline"`
	OfImage *ResponseCodeInterpreterToolCallOutputImageParam `json:",omitzero,inline"`
}

func (u ResponseCodeInterpreterToolCallOutputUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfLogs, u.OfImage)
}

func (u *ResponseCodeInterpreterToolCallOutputUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "logs":
		return json.Unmarshal(data, &u.OfLogs)
	case "image":
		return json.Unmarshal(data, &u.OfImage)
	}
	return tryUnmarshal(data)
}

type ResponseCodeInterpreterToolCallParam struct {
	Code        Opt[string]                                       `json:"code,omitzero"`
	Outputs     []ResponseCodeInterpreterToolCallOutputUnionParam `json:"outputs,omitzero"`
	ID          string                                            `json:"id"`
	ContainerID string                                            `json:"container_id"`
	Status      ResponseCodeInterpreterToolCallStatus             `json:"status,omitzero"`
	Type        string                                            `json:"type"`
	paramObj
}

func (p ResponseCodeInterpreterToolCallParam) MarshalJSON() ([]byte, error) {
	type raw ResponseCodeInterpreterToolCallParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "code_interpreter_call"})
}

type ResponseCodeInterpreterToolCallStatus string

type ResponseCompactionItemParam struct {
	EncryptedContent string      `json:"encrypted_content"`
	ID               Opt[string] `json:"id,omitzero"`
	Type             string      `json:"type"`
	paramObj
}

func (p ResponseCompactionItemParam) MarshalJSON() ([]byte, error) {
	type raw ResponseCompactionItemParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "compaction"})
}

type ResponseComputerToolCallActionClickParam struct {
	Button string   `json:"button,omitzero"`
	X      int64    `json:"x"`
	Y      int64    `json:"y"`
	Keys   []string `json:"keys,omitzero"`
	Type   string   `json:"type"`
	paramObj
}

func (p ResponseComputerToolCallActionClickParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallActionClickParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "click"})
}

type ResponseComputerToolCallActionDoubleClickParam struct {
	Keys []string `json:"keys,omitzero"`
	X    int64    `json:"x"`
	Y    int64    `json:"y"`
	Type string   `json:"type"`
	paramObj
}

func (p ResponseComputerToolCallActionDoubleClickParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallActionDoubleClickParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "double_click"})
}

type ResponseComputerToolCallActionDragParam struct {
	Path []ResponseComputerToolCallActionDragPathParam `json:"path,omitzero"`
	Keys []string                                      `json:"keys,omitzero"`
	Type string                                        `json:"type"`
	paramObj
}

func (p ResponseComputerToolCallActionDragParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallActionDragParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "drag"})
}

type ResponseComputerToolCallActionDragPath struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

type ResponseComputerToolCallActionDragPathParam struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
	paramObj
}

func (p ResponseComputerToolCallActionDragPathParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallActionDragPathParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseComputerToolCallActionKeypressParam struct {
	Keys []string `json:"keys,omitzero"`
	Type string   `json:"type"`
	paramObj
}

func (p ResponseComputerToolCallActionKeypressParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallActionKeypressParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "keypress"})
}

type ResponseComputerToolCallActionMoveParam struct {
	X    int64    `json:"x"`
	Y    int64    `json:"y"`
	Keys []string `json:"keys,omitzero"`
	Type string   `json:"type"`
	paramObj
}

func (p ResponseComputerToolCallActionMoveParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallActionMoveParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "move"})
}

type ResponseComputerToolCallActionScreenshotParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseComputerToolCallActionScreenshotParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallActionScreenshotParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "screenshot"})
}

type ResponseComputerToolCallActionScrollParam struct {
	ScrollX int64    `json:"scroll_x"`
	ScrollY int64    `json:"scroll_y"`
	X       int64    `json:"x"`
	Y       int64    `json:"y"`
	Keys    []string `json:"keys,omitzero"`
	Type    string   `json:"type"`
	paramObj
}

func (p ResponseComputerToolCallActionScrollParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallActionScrollParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "scroll"})
}

type ResponseComputerToolCallActionTypeParam struct {
	Text string `json:"text"`
	Type string `json:"type"`
	paramObj
}

func (p ResponseComputerToolCallActionTypeParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallActionTypeParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "type"})
}

type ResponseComputerToolCallActionUnionParam struct {
	OfClick       *ResponseComputerToolCallActionClickParam       `json:",omitzero,inline"`
	OfDoubleClick *ResponseComputerToolCallActionDoubleClickParam `json:",omitzero,inline"`
	OfDrag        *ResponseComputerToolCallActionDragParam        `json:",omitzero,inline"`
	OfKeypress    *ResponseComputerToolCallActionKeypressParam    `json:",omitzero,inline"`
	OfMove        *ResponseComputerToolCallActionMoveParam        `json:",omitzero,inline"`
	OfScreenshot  *ResponseComputerToolCallActionScreenshotParam  `json:",omitzero,inline"`
	OfScroll      *ResponseComputerToolCallActionScrollParam      `json:",omitzero,inline"`
	OfType        *ResponseComputerToolCallActionTypeParam        `json:",omitzero,inline"`
	OfWait        *ResponseComputerToolCallActionWaitParam        `json:",omitzero,inline"`
}

func (u ResponseComputerToolCallActionUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfClick, u.OfDoubleClick, u.OfDrag, u.OfKeypress, u.OfMove, u.OfScreenshot, u.OfScroll, u.OfType, u.OfWait)
}

func (u *ResponseComputerToolCallActionUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "click":
		return json.Unmarshal(data, &u.OfClick)
	case "double_click":
		return json.Unmarshal(data, &u.OfDoubleClick)
	case "drag":
		return json.Unmarshal(data, &u.OfDrag)
	case "keypress":
		return json.Unmarshal(data, &u.OfKeypress)
	case "move":
		return json.Unmarshal(data, &u.OfMove)
	case "screenshot":
		return json.Unmarshal(data, &u.OfScreenshot)
	case "scroll":
		return json.Unmarshal(data, &u.OfScroll)
	case "type":
		return json.Unmarshal(data, &u.OfType)
	case "wait":
		return json.Unmarshal(data, &u.OfWait)
	}
	return tryUnmarshal(data)
}

type ResponseComputerToolCallActionWaitParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseComputerToolCallActionWaitParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallActionWaitParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "wait"})
}

type ResponseComputerToolCallOutputItemAcknowledgedSafetyCheck struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ResponseComputerToolCallOutputScreenshotParam struct {
	FileID   Opt[string] `json:"file_id,omitzero"`
	ImageURL Opt[string] `json:"image_url,omitzero"`
	Type     string      `json:"type"`
	paramObj
}

func (p ResponseComputerToolCallOutputScreenshotParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallOutputScreenshotParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "computer_screenshot"})
}

type ResponseComputerToolCallParam struct {
	ID                  string                                            `json:"id"`
	CallID              string                                            `json:"call_id"`
	PendingSafetyChecks []ResponseComputerToolCallPendingSafetyCheckParam `json:"pending_safety_checks,omitzero"`
	Status              ResponseComputerToolCallStatus                    `json:"status,omitzero"`
	Type                ResponseComputerToolCallType                      `json:"type,omitzero"`
	Action              ResponseComputerToolCallActionUnionParam          `json:"action,omitzero"`
	Actions             ComputerActionListParam                           `json:"actions,omitzero"`
	paramObj
}

func (p ResponseComputerToolCallParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseComputerToolCallPendingSafetyCheck struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ResponseComputerToolCallPendingSafetyCheckParam struct {
	ID      string      `json:"id"`
	Code    Opt[string] `json:"code,omitzero"`
	Message Opt[string] `json:"message,omitzero"`
	paramObj
}

func (p ResponseComputerToolCallPendingSafetyCheckParam) MarshalJSON() ([]byte, error) {
	type raw ResponseComputerToolCallPendingSafetyCheckParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseComputerToolCallStatus string

type ResponseComputerToolCallType string

type ResponseContentPartAddedEventPartUnion struct {
	Annotations []ResponseOutputTextAnnotationUnion `json:"annotations"`
	Text        string                              `json:"text"`
	Type        string                              `json:"type"`
	Logprobs    []ResponseOutputTextLogprob         `json:"logprobs"`
	Refusal     string                              `json:"refusal"`
}

type ResponseContentPartDoneEventPartUnion struct {
	Annotations []ResponseOutputTextAnnotationUnion `json:"annotations"`
	Text        string                              `json:"text"`
	Type        string                              `json:"type"`
	Logprobs    []ResponseOutputTextLogprob         `json:"logprobs"`
	Refusal     string                              `json:"refusal"`
}

type ResponseConversation struct {
	ID string `json:"id"`
}

type ResponseConversationParam struct {
	ID string `json:"id"`
	paramObj
}

func (p ResponseConversationParam) MarshalJSON() ([]byte, error) {
	type raw ResponseConversationParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseCustomToolCallCallerDirectParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseCustomToolCallCallerDirectParam) MarshalJSON() ([]byte, error) {
	type raw ResponseCustomToolCallCallerDirectParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "direct"})
}

type ResponseCustomToolCallCallerProgramParam struct {
	CallerID string `json:"caller_id"`
	Type     string `json:"type"`
	paramObj
}

func (p ResponseCustomToolCallCallerProgramParam) MarshalJSON() ([]byte, error) {
	type raw ResponseCustomToolCallCallerProgramParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "program"})
}

type ResponseCustomToolCallCallerUnionParam struct {
	OfDirect  *ResponseCustomToolCallCallerDirectParam  `json:",omitzero,inline"`
	OfProgram *ResponseCustomToolCallCallerProgramParam `json:",omitzero,inline"`
}

func (u ResponseCustomToolCallCallerUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfDirect, u.OfProgram)
}

func (u *ResponseCustomToolCallCallerUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "direct":
		return json.Unmarshal(data, &u.OfDirect)
	case "program":
		return json.Unmarshal(data, &u.OfProgram)
	}
	return tryUnmarshal(data)
}

type ResponseCustomToolCallOutputCallerDirectParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseCustomToolCallOutputCallerDirectParam) MarshalJSON() ([]byte, error) {
	type raw ResponseCustomToolCallOutputCallerDirectParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "direct"})
}

type ResponseCustomToolCallOutputCallerProgramParam struct {
	CallerID string `json:"caller_id"`
	Type     string `json:"type"`
	paramObj
}

func (p ResponseCustomToolCallOutputCallerProgramParam) MarshalJSON() ([]byte, error) {
	type raw ResponseCustomToolCallOutputCallerProgramParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "program"})
}

type ResponseCustomToolCallOutputCallerUnionParam struct {
	OfDirect  *ResponseCustomToolCallOutputCallerDirectParam  `json:",omitzero,inline"`
	OfProgram *ResponseCustomToolCallOutputCallerProgramParam `json:",omitzero,inline"`
}

func (u ResponseCustomToolCallOutputCallerUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfDirect, u.OfProgram)
}

func (u *ResponseCustomToolCallOutputCallerUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "direct":
		return json.Unmarshal(data, &u.OfDirect)
	case "program":
		return json.Unmarshal(data, &u.OfProgram)
	}
	return tryUnmarshal(data)
}

type ResponseCustomToolCallOutputOutputOutputContentListItemUnion struct {
	Text                  string                                                                            `json:"text"`
	Type                  string                                                                            `json:"type"`
	PromptCacheBreakpoint ResponseCustomToolCallOutputOutputOutputContentListItemUnionPromptCacheBreakpoint `json:"prompt_cache_breakpoint"`
	Detail                string                                                                            `json:"detail"`
	FileID                string                                                                            `json:"file_id"`
	ImageURL              string                                                                            `json:"image_url"`
	FileData              string                                                                            `json:"file_data"`
	FileURL               string                                                                            `json:"file_url"`
	Filename              string                                                                            `json:"filename"`
}

type ResponseCustomToolCallOutputOutputOutputContentListItemUnionParam struct {
	OfInputText  *ResponseInputTextParam  `json:",omitzero,inline"`
	OfInputImage *ResponseInputImageParam `json:",omitzero,inline"`
	OfInputFile  *ResponseInputFileParam  `json:",omitzero,inline"`
}

func (u ResponseCustomToolCallOutputOutputOutputContentListItemUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfInputText, u.OfInputImage, u.OfInputFile)
}

func (u *ResponseCustomToolCallOutputOutputOutputContentListItemUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "input_text":
		return json.Unmarshal(data, &u.OfInputText)
	case "input_image":
		return json.Unmarshal(data, &u.OfInputImage)
	case "input_file":
		return json.Unmarshal(data, &u.OfInputFile)
	}
	return tryUnmarshal(data)
}

type ResponseCustomToolCallOutputOutputOutputContentListItemUnionPromptCacheBreakpoint struct {
	Mode string `json:"mode"`
}

type ResponseCustomToolCallOutputOutputUnionParam struct {
	OfString            Opt[string]                                                         `json:",omitzero,inline"`
	OfOutputContentList []ResponseCustomToolCallOutputOutputOutputContentListItemUnionParam `json:",omitzero,inline"`
}

func (u ResponseCustomToolCallOutputOutputUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfOutputContentList)
}

func (u *ResponseCustomToolCallOutputOutputUnionParam) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfOutputContentList)
}

type ResponseCustomToolCallOutputParam struct {
	CallID string                                       `json:"call_id"`
	Output ResponseCustomToolCallOutputOutputUnionParam `json:"output,omitzero"`
	ID     Opt[string]                                  `json:"id,omitzero"`
	Caller ResponseCustomToolCallOutputCallerUnionParam `json:"caller,omitzero"`
	Type   string                                       `json:"type"`
	paramObj
}

func (p ResponseCustomToolCallOutputParam) MarshalJSON() ([]byte, error) {
	type raw ResponseCustomToolCallOutputParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "custom_tool_call_output"})
}

type ResponseCustomToolCallParam struct {
	CallID    string                                 `json:"call_id"`
	Input     string                                 `json:"input"`
	Name      string                                 `json:"name"`
	ID        Opt[string]                            `json:"id,omitzero"`
	Namespace Opt[string]                            `json:"namespace,omitzero"`
	Caller    ResponseCustomToolCallCallerUnionParam `json:"caller,omitzero"`
	Type      string                                 `json:"type"`
	paramObj
}

func (p ResponseCustomToolCallParam) MarshalJSON() ([]byte, error) {
	type raw ResponseCustomToolCallParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "custom_tool_call"})
}

type ResponseError struct {
	Code    ResponseErrorCode `json:"code"`
	Message string            `json:"message"`
}

type ResponseErrorCode string

type ResponseFileSearchToolCall struct {
	ID      string                             `json:"id"`
	Queries []string                           `json:"queries"`
	Status  ResponseFileSearchToolCallStatus   `json:"status"`
	Type    string                             `json:"type"`
	Results []ResponseFileSearchToolCallResult `json:"results"`
}

type ResponseFileSearchToolCallParam struct {
	ID      string                                  `json:"id"`
	Queries []string                                `json:"queries,omitzero"`
	Status  ResponseFileSearchToolCallStatus        `json:"status,omitzero"`
	Results []ResponseFileSearchToolCallResultParam `json:"results,omitzero"`
	Type    string                                  `json:"type"`
	paramObj
}

func (p ResponseFileSearchToolCallParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFileSearchToolCallParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "file_search_call"})
}

type ResponseFileSearchToolCallResult struct {
	Attributes map[string]ResponseFileSearchToolCallResultAttributeUnion `json:"attributes"`
	FileID     string                                                    `json:"file_id"`
	Filename   string                                                    `json:"filename"`
	Score      float64                                                   `json:"score"`
	Text       string                                                    `json:"text"`
}

type ResponseFileSearchToolCallResultAttributeUnion struct {
	OfString string  `json:",inline"`
	OfFloat  float64 `json:",inline"`
	OfBool   bool    `json:",inline"`
}

func (u ResponseFileSearchToolCallResultAttributeUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfFloat, u.OfBool)
}

func (u *ResponseFileSearchToolCallResultAttributeUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfFloat, &u.OfBool)
}

type ResponseFileSearchToolCallResultAttributeUnionParam struct {
	OfString Opt[string]  `json:",omitzero,inline"`
	OfFloat  Opt[float64] `json:",omitzero,inline"`
	OfBool   Opt[bool]    `json:",omitzero,inline"`
}

func (u ResponseFileSearchToolCallResultAttributeUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfFloat, u.OfBool)
}

func (u *ResponseFileSearchToolCallResultAttributeUnionParam) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfFloat, &u.OfBool)
}

type ResponseFileSearchToolCallResultParam struct {
	FileID     Opt[string]                                                    `json:"file_id,omitzero"`
	Filename   Opt[string]                                                    `json:"filename,omitzero"`
	Score      Opt[float64]                                                   `json:"score,omitzero"`
	Text       Opt[string]                                                    `json:"text,omitzero"`
	Attributes map[string]ResponseFileSearchToolCallResultAttributeUnionParam `json:"attributes,omitzero"`
	paramObj
}

func (p ResponseFileSearchToolCallResultParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFileSearchToolCallResultParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseFileSearchToolCallStatus string

type ResponseFormatTextConfigUnion struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Schema      map[string]any `json:"schema"`
	Description string         `json:"description"`
	Strict      bool           `json:"strict"`
}

type ResponseFormatTextConfigUnionParam struct {
	OfText       *ResponseFormatTextParam                 `json:",omitzero,inline"`
	OfJSONSchema *ResponseFormatTextJSONSchemaConfigParam `json:",omitzero,inline"`
	OfJSONObject *ResponseFormatJSONObjectParam           `json:",omitzero,inline"`
}

func (u ResponseFormatTextConfigUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfText, u.OfJSONSchema, u.OfJSONObject)
}

func (u *ResponseFormatTextConfigUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "json_schema":
		return json.Unmarshal(data, &u.OfJSONSchema)
	}
	return tryUnmarshal(data, &u.OfText, &u.OfJSONObject)
}

type ResponseFormatTextJSONSchemaConfigParam struct {
	Name        string         `json:"name"`
	Schema      map[string]any `json:"schema,omitzero"`
	Strict      Opt[bool]      `json:"strict,omitzero"`
	Description Opt[string]    `json:"description,omitzero"`
	Type        string         `json:"type"`
	paramObj
}

func (p ResponseFormatTextJSONSchemaConfigParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFormatTextJSONSchemaConfigParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "json_schema"})
}

type ResponseFunctionCallOutputItemList []ResponseFunctionCallOutputItemUnion

type ResponseFunctionCallOutputItemListParam []ResponseFunctionCallOutputItemUnionParam

type ResponseFunctionCallOutputItemUnion struct {
	Text                  string                                                   `json:"text"`
	Type                  string                                                   `json:"type"`
	PromptCacheBreakpoint ResponseFunctionCallOutputItemUnionPromptCacheBreakpoint `json:"prompt_cache_breakpoint"`
	Detail                string                                                   `json:"detail"`
	FileID                string                                                   `json:"file_id"`
	ImageURL              string                                                   `json:"image_url"`
	FileData              string                                                   `json:"file_data"`
	FileURL               string                                                   `json:"file_url"`
	Filename              string                                                   `json:"filename"`
}

type ResponseFunctionCallOutputItemUnionParam struct {
	OfInputText  *ResponseInputTextContentParam  `json:",omitzero,inline"`
	OfInputImage *ResponseInputImageContentParam `json:",omitzero,inline"`
	OfInputFile  *ResponseInputFileContentParam  `json:",omitzero,inline"`
}

func (u ResponseFunctionCallOutputItemUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfInputText, u.OfInputImage, u.OfInputFile)
}

func (u *ResponseFunctionCallOutputItemUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "input_text":
		return json.Unmarshal(data, &u.OfInputText)
	case "input_image":
		return json.Unmarshal(data, &u.OfInputImage)
	case "input_file":
		return json.Unmarshal(data, &u.OfInputFile)
	}
	return tryUnmarshal(data)
}

type ResponseFunctionCallOutputItemUnionPromptCacheBreakpoint struct {
	Mode string `json:"mode"`
}

type ResponseFunctionShellCallOutputContent struct {
	Outcome ResponseFunctionShellCallOutputContentOutcomeUnion `json:"outcome"`
	Stderr  string                                             `json:"stderr"`
	Stdout  string                                             `json:"stdout"`
}

type ResponseFunctionShellCallOutputContentOutcomeExitParam struct {
	ExitCode int64  `json:"exit_code"`
	Type     string `json:"type"`
	paramObj
}

func (p ResponseFunctionShellCallOutputContentOutcomeExitParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFunctionShellCallOutputContentOutcomeExitParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "exit"})
}

type ResponseFunctionShellCallOutputContentOutcomeTimeoutParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseFunctionShellCallOutputContentOutcomeTimeoutParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFunctionShellCallOutputContentOutcomeTimeoutParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "timeout"})
}

type ResponseFunctionShellCallOutputContentOutcomeUnion struct {
	Type     string `json:"type"`
	ExitCode int64  `json:"exit_code"`
}

type ResponseFunctionShellCallOutputContentOutcomeUnionParam struct {
	OfTimeout *ResponseFunctionShellCallOutputContentOutcomeTimeoutParam `json:",omitzero,inline"`
	OfExit    *ResponseFunctionShellCallOutputContentOutcomeExitParam    `json:",omitzero,inline"`
}

func (u ResponseFunctionShellCallOutputContentOutcomeUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfTimeout, u.OfExit)
}

func (u *ResponseFunctionShellCallOutputContentOutcomeUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "timeout":
		return json.Unmarshal(data, &u.OfTimeout)
	case "exit":
		return json.Unmarshal(data, &u.OfExit)
	}
	return tryUnmarshal(data)
}

type ResponseFunctionShellCallOutputContentParam struct {
	Outcome ResponseFunctionShellCallOutputContentOutcomeUnionParam `json:"outcome,omitzero"`
	Stderr  string                                                  `json:"stderr"`
	Stdout  string                                                  `json:"stdout"`
	paramObj
}

func (p ResponseFunctionShellCallOutputContentParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFunctionShellCallOutputContentParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseFunctionShellToolCallEnvironmentUnion struct {
	Type        string `json:"type"`
	ContainerID string `json:"container_id"`
}

type ResponseFunctionShellToolCallOutputOutput struct {
	Outcome   ResponseFunctionShellToolCallOutputOutputOutcomeUnion `json:"outcome"`
	Stderr    string                                                `json:"stderr"`
	Stdout    string                                                `json:"stdout"`
	CreatedBy string                                                `json:"created_by"`
}

type ResponseFunctionShellToolCallOutputOutputOutcomeUnion struct {
	Type     string `json:"type"`
	ExitCode int64  `json:"exit_code"`
}

type ResponseFunctionToolCall struct {
	Arguments string                              `json:"arguments"`
	CallID    string                              `json:"call_id"`
	Name      string                              `json:"name"`
	Type      string                              `json:"type"`
	ID        string                              `json:"id"`
	Caller    ResponseFunctionToolCallCallerUnion `json:"caller"`
	Namespace string                              `json:"namespace"`
	Status    ResponseFunctionToolCallStatus      `json:"status"`
}

type ResponseFunctionToolCallCallerDirectParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseFunctionToolCallCallerDirectParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFunctionToolCallCallerDirectParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "direct"})
}

type ResponseFunctionToolCallCallerProgramParam struct {
	CallerID string `json:"caller_id"`
	Type     string `json:"type"`
	paramObj
}

func (p ResponseFunctionToolCallCallerProgramParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFunctionToolCallCallerProgramParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "program"})
}

type ResponseFunctionToolCallCallerUnion struct {
	Type     string `json:"type"`
	CallerID string `json:"caller_id"`
}

type ResponseFunctionToolCallCallerUnionParam struct {
	OfDirect  *ResponseFunctionToolCallCallerDirectParam  `json:",omitzero,inline"`
	OfProgram *ResponseFunctionToolCallCallerProgramParam `json:",omitzero,inline"`
}

func (u ResponseFunctionToolCallCallerUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfDirect, u.OfProgram)
}

func (u *ResponseFunctionToolCallCallerUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "direct":
		return json.Unmarshal(data, &u.OfDirect)
	case "program":
		return json.Unmarshal(data, &u.OfProgram)
	}
	return tryUnmarshal(data)
}

type ResponseFunctionToolCallOutputItemOutputOutputContentListItemUnion struct {
	Text                  string                                                                                  `json:"text"`
	Type                  string                                                                                  `json:"type"`
	PromptCacheBreakpoint ResponseFunctionToolCallOutputItemOutputOutputContentListItemUnionPromptCacheBreakpoint `json:"prompt_cache_breakpoint"`
	Detail                string                                                                                  `json:"detail"`
	FileID                string                                                                                  `json:"file_id"`
	ImageURL              string                                                                                  `json:"image_url"`
	FileData              string                                                                                  `json:"file_data"`
	FileURL               string                                                                                  `json:"file_url"`
	Filename              string                                                                                  `json:"filename"`
}

type ResponseFunctionToolCallOutputItemOutputOutputContentListItemUnionPromptCacheBreakpoint struct {
	Mode string `json:"mode"`
}

type ResponseFunctionToolCallParam struct {
	Arguments string                                   `json:"arguments"`
	CallID    string                                   `json:"call_id"`
	Name      string                                   `json:"name"`
	ID        Opt[string]                              `json:"id,omitzero"`
	Namespace Opt[string]                              `json:"namespace,omitzero"`
	Caller    ResponseFunctionToolCallCallerUnionParam `json:"caller,omitzero"`
	Status    ResponseFunctionToolCallStatus           `json:"status,omitzero"`
	Type      string                                   `json:"type"`
	paramObj
}

func (p ResponseFunctionToolCallParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFunctionToolCallParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "function_call"})
}

type ResponseFunctionToolCallStatus string

type ResponseFunctionWebSearch struct {
	ID     string                               `json:"id"`
	Action ResponseFunctionWebSearchActionUnion `json:"action"`
	Status ResponseFunctionWebSearchStatus      `json:"status"`
	Type   string                               `json:"type"`
}

type ResponseFunctionWebSearchActionFindParam struct {
	Pattern string `json:"pattern"`
	URL     string `json:"url"`
	Type    string `json:"type"`
	paramObj
}

func (p ResponseFunctionWebSearchActionFindParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFunctionWebSearchActionFindParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "find_in_page"})
}

type ResponseFunctionWebSearchActionOpenPageParam struct {
	URL  Opt[string] `json:"url,omitzero"`
	Type string      `json:"type"`
	paramObj
}

func (p ResponseFunctionWebSearchActionOpenPageParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFunctionWebSearchActionOpenPageParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "open_page"})
}

type ResponseFunctionWebSearchActionSearchParam struct {
	Query   Opt[string]                                        `json:"query,omitzero"`
	Queries []string                                           `json:"queries,omitzero"`
	Sources []ResponseFunctionWebSearchActionSearchSourceParam `json:"sources,omitzero"`
	Type    string                                             `json:"type"`
	paramObj
}

func (p ResponseFunctionWebSearchActionSearchParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFunctionWebSearchActionSearchParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "search"})
}

type ResponseFunctionWebSearchActionSearchSource struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type ResponseFunctionWebSearchActionSearchSourceParam struct {
	URL  string `json:"url"`
	Type string `json:"type"`
	paramObj
}

func (p ResponseFunctionWebSearchActionSearchSourceParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFunctionWebSearchActionSearchSourceParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "url"})
}

type ResponseFunctionWebSearchActionUnion struct {
	Type    string                                        `json:"type"`
	Queries []string                                      `json:"queries"`
	Query   string                                        `json:"query"`
	Sources []ResponseFunctionWebSearchActionSearchSource `json:"sources"`
	URL     string                                        `json:"url"`
	Pattern string                                        `json:"pattern"`
}

type ResponseFunctionWebSearchActionUnionParam struct {
	OfSearch   *ResponseFunctionWebSearchActionSearchParam   `json:",omitzero,inline"`
	OfOpenPage *ResponseFunctionWebSearchActionOpenPageParam `json:",omitzero,inline"`
	OfFind     *ResponseFunctionWebSearchActionFindParam     `json:",omitzero,inline"`
}

func (u ResponseFunctionWebSearchActionUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfSearch, u.OfOpenPage, u.OfFind)
}

func (u *ResponseFunctionWebSearchActionUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "search":
		return json.Unmarshal(data, &u.OfSearch)
	case "open_page":
		return json.Unmarshal(data, &u.OfOpenPage)
	case "find_in_page":
		return json.Unmarshal(data, &u.OfFind)
	}
	return tryUnmarshal(data)
}

type ResponseFunctionWebSearchParam struct {
	ID     string                                    `json:"id"`
	Action ResponseFunctionWebSearchActionUnionParam `json:"action,omitzero"`
	Status ResponseFunctionWebSearchStatus           `json:"status,omitzero"`
	Type   string                                    `json:"type"`
	paramObj
}

func (p ResponseFunctionWebSearchParam) MarshalJSON() ([]byte, error) {
	type raw ResponseFunctionWebSearchParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "web_search_call"})
}

type ResponseFunctionWebSearchStatus string

type ResponseIncludable string

type ResponseIncompleteDetails struct {
	Reason string `json:"reason"`
}

type ResponseInputAudioInputAudioParam struct {
	Data   string `json:"data"`
	Format string `json:"format,omitzero"`
	paramObj
}

func (p ResponseInputAudioInputAudioParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputAudioInputAudioParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseInputAudioParam struct {
	InputAudio ResponseInputAudioInputAudioParam `json:"input_audio,omitzero"`
	Type       string                            `json:"type"`
	paramObj
}

func (p ResponseInputAudioParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputAudioParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "input_audio"})
}

type ResponseInputContentUnion struct {
	Text                  string                                         `json:"text"`
	Type                  string                                         `json:"type"`
	PromptCacheBreakpoint ResponseInputContentUnionPromptCacheBreakpoint `json:"prompt_cache_breakpoint"`
	Detail                string                                         `json:"detail"`
	FileID                string                                         `json:"file_id"`
	ImageURL              string                                         `json:"image_url"`
	FileData              string                                         `json:"file_data"`
	FileURL               string                                         `json:"file_url"`
	Filename              string                                         `json:"filename"`
}

type ResponseInputContentUnionParam struct {
	OfInputText  *ResponseInputTextParam  `json:",omitzero,inline"`
	OfInputImage *ResponseInputImageParam `json:",omitzero,inline"`
	OfInputFile  *ResponseInputFileParam  `json:",omitzero,inline"`
}

func (u ResponseInputContentUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfInputText, u.OfInputImage, u.OfInputFile)
}

func (u *ResponseInputContentUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "input_text":
		return json.Unmarshal(data, &u.OfInputText)
	case "input_image":
		return json.Unmarshal(data, &u.OfInputImage)
	case "input_file":
		return json.Unmarshal(data, &u.OfInputFile)
	}
	return tryUnmarshal(data)
}

type ResponseInputContentUnionPromptCacheBreakpoint struct {
	Mode string `json:"mode"`
}

type ResponseInputFileContentDetail string

type ResponseInputFileContentParam struct {
	FileData              Opt[string]                                        `json:"file_data,omitzero"`
	FileID                Opt[string]                                        `json:"file_id,omitzero"`
	FileURL               Opt[string]                                        `json:"file_url,omitzero"`
	Filename              Opt[string]                                        `json:"filename,omitzero"`
	PromptCacheBreakpoint ResponseInputFileContentPromptCacheBreakpointParam `json:"prompt_cache_breakpoint,omitzero"`
	Detail                ResponseInputFileContentDetail                     `json:"detail,omitzero"`
	Type                  string                                             `json:"type"`
	paramObj
}

func (p ResponseInputFileContentParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputFileContentParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "input_file"})
}

type ResponseInputFileContentPromptCacheBreakpointParam struct {
	Mode string `json:"mode"`
	paramObj
}

func (p ResponseInputFileContentPromptCacheBreakpointParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputFileContentPromptCacheBreakpointParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"mode": "explicit"})
}

type ResponseInputFileDetail string

type ResponseInputFileParam struct {
	FileID                Opt[string]                                 `json:"file_id,omitzero"`
	FileData              Opt[string]                                 `json:"file_data,omitzero"`
	FileURL               Opt[string]                                 `json:"file_url,omitzero"`
	Filename              Opt[string]                                 `json:"filename,omitzero"`
	Detail                ResponseInputFileDetail                     `json:"detail,omitzero"`
	PromptCacheBreakpoint ResponseInputFilePromptCacheBreakpointParam `json:"prompt_cache_breakpoint,omitzero"`
	Type                  string                                      `json:"type"`
	paramObj
}

func (p ResponseInputFileParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputFileParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "input_file"})
}

type ResponseInputFilePromptCacheBreakpointParam struct {
	Mode string `json:"mode"`
	paramObj
}

func (p ResponseInputFilePromptCacheBreakpointParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputFilePromptCacheBreakpointParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"mode": "explicit"})
}

type ResponseInputImageContentDetail string

type ResponseInputImageContentParam struct {
	FileID                Opt[string]                                         `json:"file_id,omitzero"`
	ImageURL              Opt[string]                                         `json:"image_url,omitzero"`
	Detail                ResponseInputImageContentDetail                     `json:"detail,omitzero"`
	PromptCacheBreakpoint ResponseInputImageContentPromptCacheBreakpointParam `json:"prompt_cache_breakpoint,omitzero"`
	Type                  string                                              `json:"type"`
	paramObj
}

func (p ResponseInputImageContentParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputImageContentParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "input_image"})
}

type ResponseInputImageContentPromptCacheBreakpointParam struct {
	Mode string `json:"mode"`
	paramObj
}

func (p ResponseInputImageContentPromptCacheBreakpointParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputImageContentPromptCacheBreakpointParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"mode": "explicit"})
}

type ResponseInputImageDetail string

type ResponseInputImageParam struct {
	Detail                ResponseInputImageDetail                     `json:"detail,omitzero"`
	FileID                Opt[string]                                  `json:"file_id,omitzero"`
	ImageURL              Opt[string]                                  `json:"image_url,omitzero"`
	PromptCacheBreakpoint ResponseInputImagePromptCacheBreakpointParam `json:"prompt_cache_breakpoint,omitzero"`
	Type                  string                                       `json:"type"`
	paramObj
}

func (p ResponseInputImageParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputImageParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "input_image"})
}

type ResponseInputImagePromptCacheBreakpointParam struct {
	Mode string `json:"mode"`
	paramObj
}

func (p ResponseInputImagePromptCacheBreakpointParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputImagePromptCacheBreakpointParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"mode": "explicit"})
}

type ResponseInputItemAdditionalToolsParam struct {
	Tools []ToolUnionParam `json:"tools,omitzero"`
	ID    Opt[string]      `json:"id,omitzero"`
	Role  string           `json:"role"`
	Type  string           `json:"type"`
	paramObj
}

func (p ResponseInputItemAdditionalToolsParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemAdditionalToolsParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"role": "developer", "type": "additional_tools"})
}

type ResponseInputItemApplyPatchCallCallerDirectParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseInputItemApplyPatchCallCallerDirectParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemApplyPatchCallCallerDirectParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "direct"})
}

type ResponseInputItemApplyPatchCallCallerProgramParam struct {
	CallerID string `json:"caller_id"`
	Type     string `json:"type"`
	paramObj
}

func (p ResponseInputItemApplyPatchCallCallerProgramParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemApplyPatchCallCallerProgramParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "program"})
}

type ResponseInputItemApplyPatchCallCallerUnionParam struct {
	OfDirect  *ResponseInputItemApplyPatchCallCallerDirectParam  `json:",omitzero,inline"`
	OfProgram *ResponseInputItemApplyPatchCallCallerProgramParam `json:",omitzero,inline"`
}

func (u ResponseInputItemApplyPatchCallCallerUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfDirect, u.OfProgram)
}

func (u *ResponseInputItemApplyPatchCallCallerUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "direct":
		return json.Unmarshal(data, &u.OfDirect)
	case "program":
		return json.Unmarshal(data, &u.OfProgram)
	}
	return tryUnmarshal(data)
}

type ResponseInputItemApplyPatchCallOperationCreateFileParam struct {
	Diff string `json:"diff"`
	Path string `json:"path"`
	Type string `json:"type"`
	paramObj
}

func (p ResponseInputItemApplyPatchCallOperationCreateFileParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemApplyPatchCallOperationCreateFileParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "create_file"})
}

type ResponseInputItemApplyPatchCallOperationDeleteFileParam struct {
	Path string `json:"path"`
	Type string `json:"type"`
	paramObj
}

func (p ResponseInputItemApplyPatchCallOperationDeleteFileParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemApplyPatchCallOperationDeleteFileParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "delete_file"})
}

type ResponseInputItemApplyPatchCallOperationUnion struct {
	Diff string `json:"diff"`
	Path string `json:"path"`
	Type string `json:"type"`
}

type ResponseInputItemApplyPatchCallOperationUnionParam struct {
	OfCreateFile *ResponseInputItemApplyPatchCallOperationCreateFileParam `json:",omitzero,inline"`
	OfDeleteFile *ResponseInputItemApplyPatchCallOperationDeleteFileParam `json:",omitzero,inline"`
	OfUpdateFile *ResponseInputItemApplyPatchCallOperationUpdateFileParam `json:",omitzero,inline"`
}

func (u ResponseInputItemApplyPatchCallOperationUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfCreateFile, u.OfDeleteFile, u.OfUpdateFile)
}

func (u *ResponseInputItemApplyPatchCallOperationUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "create_file":
		return json.Unmarshal(data, &u.OfCreateFile)
	case "delete_file":
		return json.Unmarshal(data, &u.OfDeleteFile)
	case "update_file":
		return json.Unmarshal(data, &u.OfUpdateFile)
	}
	return tryUnmarshal(data)
}

type ResponseInputItemApplyPatchCallOperationUpdateFileParam struct {
	Diff string `json:"diff"`
	Path string `json:"path"`
	Type string `json:"type"`
	paramObj
}

func (p ResponseInputItemApplyPatchCallOperationUpdateFileParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemApplyPatchCallOperationUpdateFileParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "update_file"})
}

type ResponseInputItemApplyPatchCallOutputCallerDirectParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseInputItemApplyPatchCallOutputCallerDirectParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemApplyPatchCallOutputCallerDirectParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "direct"})
}

type ResponseInputItemApplyPatchCallOutputCallerProgramParam struct {
	CallerID string `json:"caller_id"`
	Type     string `json:"type"`
	paramObj
}

func (p ResponseInputItemApplyPatchCallOutputCallerProgramParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemApplyPatchCallOutputCallerProgramParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "program"})
}

type ResponseInputItemApplyPatchCallOutputCallerUnionParam struct {
	OfDirect  *ResponseInputItemApplyPatchCallOutputCallerDirectParam  `json:",omitzero,inline"`
	OfProgram *ResponseInputItemApplyPatchCallOutputCallerProgramParam `json:",omitzero,inline"`
}

func (u ResponseInputItemApplyPatchCallOutputCallerUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfDirect, u.OfProgram)
}

func (u *ResponseInputItemApplyPatchCallOutputCallerUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "direct":
		return json.Unmarshal(data, &u.OfDirect)
	case "program":
		return json.Unmarshal(data, &u.OfProgram)
	}
	return tryUnmarshal(data)
}

type ResponseInputItemApplyPatchCallOutputParam struct {
	CallID string                                                `json:"call_id"`
	Status string                                                `json:"status,omitzero"`
	ID     Opt[string]                                           `json:"id,omitzero"`
	Output Opt[string]                                           `json:"output,omitzero"`
	Caller ResponseInputItemApplyPatchCallOutputCallerUnionParam `json:"caller,omitzero"`
	Type   string                                                `json:"type"`
	paramObj
}

func (p ResponseInputItemApplyPatchCallOutputParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemApplyPatchCallOutputParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "apply_patch_call_output"})
}

type ResponseInputItemApplyPatchCallParam struct {
	CallID    string                                             `json:"call_id"`
	Operation ResponseInputItemApplyPatchCallOperationUnionParam `json:"operation,omitzero"`
	Status    string                                             `json:"status,omitzero"`
	ID        Opt[string]                                        `json:"id,omitzero"`
	Caller    ResponseInputItemApplyPatchCallCallerUnionParam    `json:"caller,omitzero"`
	Type      string                                             `json:"type"`
	paramObj
}

func (p ResponseInputItemApplyPatchCallParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemApplyPatchCallParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "apply_patch_call"})
}

type ResponseInputItemCompactionTriggerParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseInputItemCompactionTriggerParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemCompactionTriggerParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "compaction_trigger"})
}

type ResponseInputItemComputerCallOutputAcknowledgedSafetyCheck struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ResponseInputItemComputerCallOutputAcknowledgedSafetyCheckParam struct {
	ID      string      `json:"id"`
	Code    Opt[string] `json:"code,omitzero"`
	Message Opt[string] `json:"message,omitzero"`
	paramObj
}

func (p ResponseInputItemComputerCallOutputAcknowledgedSafetyCheckParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemComputerCallOutputAcknowledgedSafetyCheckParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseInputItemComputerCallOutputParam struct {
	CallID                   string                                                            `json:"call_id"`
	Output                   ResponseComputerToolCallOutputScreenshotParam                     `json:"output,omitzero"`
	ID                       Opt[string]                                                       `json:"id,omitzero"`
	AcknowledgedSafetyChecks []ResponseInputItemComputerCallOutputAcknowledgedSafetyCheckParam `json:"acknowledged_safety_checks,omitzero"`
	Status                   string                                                            `json:"status,omitzero"`
	Type                     string                                                            `json:"type"`
	paramObj
}

func (p ResponseInputItemComputerCallOutputParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemComputerCallOutputParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "computer_call_output"})
}

type ResponseInputItemFunctionCallOutputCallerDirectParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseInputItemFunctionCallOutputCallerDirectParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemFunctionCallOutputCallerDirectParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "direct"})
}

type ResponseInputItemFunctionCallOutputCallerProgramParam struct {
	CallerID string `json:"caller_id"`
	Type     string `json:"type"`
	paramObj
}

func (p ResponseInputItemFunctionCallOutputCallerProgramParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemFunctionCallOutputCallerProgramParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "program"})
}

type ResponseInputItemFunctionCallOutputCallerUnionParam struct {
	OfDirect  *ResponseInputItemFunctionCallOutputCallerDirectParam  `json:",omitzero,inline"`
	OfProgram *ResponseInputItemFunctionCallOutputCallerProgramParam `json:",omitzero,inline"`
}

func (u ResponseInputItemFunctionCallOutputCallerUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfDirect, u.OfProgram)
}

func (u *ResponseInputItemFunctionCallOutputCallerUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "direct":
		return json.Unmarshal(data, &u.OfDirect)
	case "program":
		return json.Unmarshal(data, &u.OfProgram)
	}
	return tryUnmarshal(data)
}

type ResponseInputItemFunctionCallOutputOutputUnionParam struct {
	OfString                              Opt[string]                             `json:",omitzero,inline"`
	OfResponseFunctionCallOutputItemArray ResponseFunctionCallOutputItemListParam `json:",omitzero,inline"`
}

func (u ResponseInputItemFunctionCallOutputOutputUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfResponseFunctionCallOutputItemArray)
}

func (u *ResponseInputItemFunctionCallOutputOutputUnionParam) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfResponseFunctionCallOutputItemArray)
}

type ResponseInputItemFunctionCallOutputParam struct {
	Output    ResponseInputItemFunctionCallOutputOutputUnionParam `json:"output,omitzero"`
	ID        Opt[string]                                         `json:"id,omitzero"`
	CallID    Opt[string]                                         `json:"call_id,omitzero"`
	Name      Opt[string]                                         `json:"name,omitzero"`
	Namespace Opt[string]                                         `json:"namespace,omitzero"`
	Caller    ResponseInputItemFunctionCallOutputCallerUnionParam `json:"caller,omitzero"`
	Status    string                                              `json:"status,omitzero"`
	Type      string                                              `json:"type"`
	paramObj
}

func (p ResponseInputItemFunctionCallOutputParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemFunctionCallOutputParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "function_call_output"})
}

type ResponseInputItemImageGenerationCallParam struct {
	Result Opt[string] `json:"result,omitzero"`
	ID     string      `json:"id"`
	Status string      `json:"status,omitzero"`
	Type   string      `json:"type"`
	paramObj
}

func (p ResponseInputItemImageGenerationCallParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemImageGenerationCallParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "image_generation_call"})
}

type ResponseInputItemItemReferenceParam struct {
	ID   string `json:"id"`
	Type string `json:"type,omitzero"`
	paramObj
}

func (p ResponseInputItemItemReferenceParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemItemReferenceParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseInputItemLocalShellCallActionParam struct {
	Command          []string          `json:"command,omitzero"`
	Env              map[string]string `json:"env,omitzero"`
	TimeoutMs        Opt[int64]        `json:"timeout_ms,omitzero"`
	User             Opt[string]       `json:"user,omitzero"`
	WorkingDirectory Opt[string]       `json:"working_directory,omitzero"`
	Type             string            `json:"type"`
	paramObj
}

func (p ResponseInputItemLocalShellCallActionParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemLocalShellCallActionParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "exec"})
}

type ResponseInputItemLocalShellCallOutputParam struct {
	ID     string `json:"id"`
	Output string `json:"output"`
	Status string `json:"status,omitzero"`
	Type   string `json:"type"`
	paramObj
}

func (p ResponseInputItemLocalShellCallOutputParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemLocalShellCallOutputParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "local_shell_call_output"})
}

type ResponseInputItemLocalShellCallParam struct {
	ID     string                                     `json:"id"`
	Action ResponseInputItemLocalShellCallActionParam `json:"action,omitzero"`
	CallID string                                     `json:"call_id"`
	Status string                                     `json:"status,omitzero"`
	Type   string                                     `json:"type"`
	paramObj
}

func (p ResponseInputItemLocalShellCallParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemLocalShellCallParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "local_shell_call"})
}

type ResponseInputItemMcpApprovalRequestParam struct {
	ID          string `json:"id"`
	Arguments   string `json:"arguments"`
	Name        string `json:"name"`
	ServerLabel string `json:"server_label"`
	Type        string `json:"type"`
	paramObj
}

func (p ResponseInputItemMcpApprovalRequestParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemMcpApprovalRequestParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "mcp_approval_request"})
}

type ResponseInputItemMcpApprovalResponseParam struct {
	ApprovalRequestID string      `json:"approval_request_id"`
	Approve           bool        `json:"approve"`
	ID                Opt[string] `json:"id,omitzero"`
	Reason            Opt[string] `json:"reason,omitzero"`
	Type              string      `json:"type"`
	paramObj
}

func (p ResponseInputItemMcpApprovalResponseParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemMcpApprovalResponseParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "mcp_approval_response"})
}

type ResponseInputItemMcpCallParam struct {
	ID                string                     `json:"id"`
	Arguments         string                     `json:"arguments"`
	Name              string                     `json:"name"`
	ServerLabel       string                     `json:"server_label"`
	ApprovalRequestID Opt[string]                `json:"approval_request_id,omitzero"`
	Output            Opt[string]                `json:"output,omitzero"`
	Error             McpToolCallErrorUnionParam `json:"error,omitzero"`
	Status            string                     `json:"status,omitzero"`
	Type              string                     `json:"type"`
	paramObj
}

func (p ResponseInputItemMcpCallParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemMcpCallParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "mcp_call"})
}

type ResponseInputItemMcpListToolsParam struct {
	ID          string                                   `json:"id"`
	ServerLabel string                                   `json:"server_label"`
	Tools       []ResponseInputItemMcpListToolsToolParam `json:"tools,omitzero"`
	Error       Opt[string]                              `json:"error,omitzero"`
	Type        string                                   `json:"type"`
	paramObj
}

func (p ResponseInputItemMcpListToolsParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemMcpListToolsParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "mcp_list_tools"})
}

type ResponseInputItemMcpListToolsTool struct {
	InputSchema any    `json:"input_schema"`
	Name        string `json:"name"`
	Annotations any    `json:"annotations"`
	Description string `json:"description"`
}

type ResponseInputItemMcpListToolsToolParam struct {
	InputSchema any         `json:"input_schema,omitzero"`
	Name        string      `json:"name"`
	Description Opt[string] `json:"description,omitzero"`
	Annotations any         `json:"annotations,omitzero"`
	paramObj
}

func (p ResponseInputItemMcpListToolsToolParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemMcpListToolsToolParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseInputItemMessageParam struct {
	Content ResponseInputMessageContentListParam `json:"content,omitzero"`
	Role    string                               `json:"role,omitzero"`
	Status  string                               `json:"status,omitzero"`
	Type    string                               `json:"type,omitzero"`
	paramObj
}

func (p ResponseInputItemMessageParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemMessageParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseInputItemProgramOutputParam struct {
	ID     string `json:"id"`
	CallID string `json:"call_id"`
	Result string `json:"result"`
	Status string `json:"status,omitzero"`
	Type   string `json:"type"`
	paramObj
}

func (p ResponseInputItemProgramOutputParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemProgramOutputParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "program_output"})
}

type ResponseInputItemProgramParam struct {
	ID          string `json:"id"`
	CallID      string `json:"call_id"`
	Code        string `json:"code"`
	Fingerprint string `json:"fingerprint"`
	Type        string `json:"type"`
	paramObj
}

func (p ResponseInputItemProgramParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemProgramParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "program"})
}

type ResponseInputItemShellCallActionParam struct {
	Commands        []string   `json:"commands,omitzero"`
	MaxOutputLength Opt[int64] `json:"max_output_length,omitzero"`
	TimeoutMs       Opt[int64] `json:"timeout_ms,omitzero"`
	paramObj
}

func (p ResponseInputItemShellCallActionParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemShellCallActionParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseInputItemShellCallCallerDirectParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseInputItemShellCallCallerDirectParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemShellCallCallerDirectParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "direct"})
}

type ResponseInputItemShellCallCallerProgramParam struct {
	CallerID string `json:"caller_id"`
	Type     string `json:"type"`
	paramObj
}

func (p ResponseInputItemShellCallCallerProgramParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemShellCallCallerProgramParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "program"})
}

type ResponseInputItemShellCallCallerUnionParam struct {
	OfDirect  *ResponseInputItemShellCallCallerDirectParam  `json:",omitzero,inline"`
	OfProgram *ResponseInputItemShellCallCallerProgramParam `json:",omitzero,inline"`
}

func (u ResponseInputItemShellCallCallerUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfDirect, u.OfProgram)
}

func (u *ResponseInputItemShellCallCallerUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "direct":
		return json.Unmarshal(data, &u.OfDirect)
	case "program":
		return json.Unmarshal(data, &u.OfProgram)
	}
	return tryUnmarshal(data)
}

type ResponseInputItemShellCallEnvironmentUnion struct {
	Type        string       `json:"type"`
	Skills      []LocalSkill `json:"skills"`
	ContainerID string       `json:"container_id"`
}

type ResponseInputItemShellCallEnvironmentUnionParam struct {
	OfLocal              *LocalEnvironmentParam   `json:",omitzero,inline"`
	OfContainerReference *ContainerReferenceParam `json:",omitzero,inline"`
}

func (u ResponseInputItemShellCallEnvironmentUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfLocal, u.OfContainerReference)
}

func (u *ResponseInputItemShellCallEnvironmentUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "local":
		return json.Unmarshal(data, &u.OfLocal)
	case "container_reference":
		return json.Unmarshal(data, &u.OfContainerReference)
	}
	return tryUnmarshal(data)
}

type ResponseInputItemShellCallOutputCallerDirectParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseInputItemShellCallOutputCallerDirectParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemShellCallOutputCallerDirectParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "direct"})
}

type ResponseInputItemShellCallOutputCallerProgramParam struct {
	CallerID string `json:"caller_id"`
	Type     string `json:"type"`
	paramObj
}

func (p ResponseInputItemShellCallOutputCallerProgramParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemShellCallOutputCallerProgramParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "program"})
}

type ResponseInputItemShellCallOutputCallerUnionParam struct {
	OfDirect  *ResponseInputItemShellCallOutputCallerDirectParam  `json:",omitzero,inline"`
	OfProgram *ResponseInputItemShellCallOutputCallerProgramParam `json:",omitzero,inline"`
}

func (u ResponseInputItemShellCallOutputCallerUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfDirect, u.OfProgram)
}

func (u *ResponseInputItemShellCallOutputCallerUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "direct":
		return json.Unmarshal(data, &u.OfDirect)
	case "program":
		return json.Unmarshal(data, &u.OfProgram)
	}
	return tryUnmarshal(data)
}

type ResponseInputItemShellCallOutputParam struct {
	CallID          string                                           `json:"call_id"`
	Output          []ResponseFunctionShellCallOutputContentParam    `json:"output,omitzero"`
	ID              Opt[string]                                      `json:"id,omitzero"`
	MaxOutputLength Opt[int64]                                       `json:"max_output_length,omitzero"`
	Caller          ResponseInputItemShellCallOutputCallerUnionParam `json:"caller,omitzero"`
	Status          string                                           `json:"status,omitzero"`
	Type            string                                           `json:"type"`
	paramObj
}

func (p ResponseInputItemShellCallOutputParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemShellCallOutputParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "shell_call_output"})
}

type ResponseInputItemShellCallParam struct {
	Action      ResponseInputItemShellCallActionParam           `json:"action,omitzero"`
	CallID      string                                          `json:"call_id"`
	ID          Opt[string]                                     `json:"id,omitzero"`
	Caller      ResponseInputItemShellCallCallerUnionParam      `json:"caller,omitzero"`
	Environment ResponseInputItemShellCallEnvironmentUnionParam `json:"environment,omitzero"`
	Status      string                                          `json:"status,omitzero"`
	Type        string                                          `json:"type"`
	paramObj
}

func (p ResponseInputItemShellCallParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemShellCallParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "shell_call"})
}

type ResponseInputItemToolSearchCallParam struct {
	Arguments any         `json:"arguments,omitzero"`
	ID        Opt[string] `json:"id,omitzero"`
	CallID    Opt[string] `json:"call_id,omitzero"`
	Status    string      `json:"status,omitzero"`
	Execution string      `json:"execution,omitzero"`
	Type      string      `json:"type"`
	paramObj
}

func (p ResponseInputItemToolSearchCallParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputItemToolSearchCallParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "tool_search_call"})
}

type ResponseInputItemUnion struct {
	Content                  ResponseInputItemUnionContent                                `json:"content"`
	Role                     string                                                       `json:"role"`
	Phase                    string                                                       `json:"phase"`
	Type                     string                                                       `json:"type"`
	Status                   string                                                       `json:"status"`
	ID                       string                                                       `json:"id"`
	Queries                  []string                                                     `json:"queries"`
	Results                  []ResponseFileSearchToolCallResult                           `json:"results"`
	CallID                   string                                                       `json:"call_id"`
	PendingSafetyChecks      []ResponseComputerToolCallPendingSafetyCheck                 `json:"pending_safety_checks"`
	Action                   ResponseInputItemUnionAction                                 `json:"action"`
	Actions                  ComputerActionList                                           `json:"actions"`
	Output                   ResponseInputItemUnionOutput                                 `json:"output"`
	AcknowledgedSafetyChecks []ResponseInputItemComputerCallOutputAcknowledgedSafetyCheck `json:"acknowledged_safety_checks"`
	Arguments                ResponseInputItemUnionArguments                              `json:"arguments"`
	Name                     string                                                       `json:"name"`
	Caller                   ResponseInputItemUnionCaller                                 `json:"caller"`
	Namespace                string                                                       `json:"namespace"`
	Execution                string                                                       `json:"execution"`
	Tools                    ResponseInputItemUnionTools                                  `json:"tools"`
	Summary                  []ResponseReasoningItemSummary                               `json:"summary"`
	EncryptedContent         string                                                       `json:"encrypted_content"`
	Result                   string                                                       `json:"result"`
	Code                     string                                                       `json:"code"`
	ContainerID              string                                                       `json:"container_id"`
	Outputs                  []ResponseCodeInterpreterToolCallOutputUnion                 `json:"outputs"`
	Environment              ResponseInputItemShellCallEnvironmentUnion                   `json:"environment"`
	MaxOutputLength          int64                                                        `json:"max_output_length"`
	Operation                ResponseInputItemApplyPatchCallOperationUnion                `json:"operation"`
	ServerLabel              string                                                       `json:"server_label"`
	Error                    ResponseInputItemUnionError                                  `json:"error"`
	ApprovalRequestID        string                                                       `json:"approval_request_id"`
	Approve                  bool                                                         `json:"approve"`
	Reason                   string                                                       `json:"reason"`
	Input                    string                                                       `json:"input"`
	Fingerprint              string                                                       `json:"fingerprint"`
}

type ResponseInputItemUnionAction struct {
	Button           string                                        `json:"button"`
	Type             string                                        `json:"type"`
	X                int64                                         `json:"x"`
	Y                int64                                         `json:"y"`
	Keys             []string                                      `json:"keys"`
	Path             []ResponseComputerToolCallActionDragPath      `json:"path"`
	ScrollX          int64                                         `json:"scroll_x"`
	ScrollY          int64                                         `json:"scroll_y"`
	Text             string                                        `json:"text"`
	Queries          []string                                      `json:"queries"`
	Query            string                                        `json:"query"`
	Sources          []ResponseFunctionWebSearchActionSearchSource `json:"sources"`
	URL              string                                        `json:"url"`
	Pattern          string                                        `json:"pattern"`
	Command          []string                                      `json:"command"`
	Env              map[string]string                             `json:"env"`
	TimeoutMs        int64                                         `json:"timeout_ms"`
	User             string                                        `json:"user"`
	WorkingDirectory string                                        `json:"working_directory"`
	Commands         []string                                      `json:"commands"`
	MaxOutputLength  int64                                         `json:"max_output_length"`
}

type ResponseInputItemUnionArguments struct {
	OfString                                   string `json:",inline"`
	OfResponseInputItemToolSearchCallArguments any    `json:",inline"`
}

func (u ResponseInputItemUnionArguments) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfResponseInputItemToolSearchCallArguments)
}

func (u *ResponseInputItemUnionArguments) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfResponseInputItemToolSearchCallArguments)
}

type ResponseInputItemUnionCaller struct {
	Type     string `json:"type"`
	CallerID string `json:"caller_id"`
}

type ResponseInputItemUnionContent struct {
	OfString                            string                              `json:",inline"`
	OfInputItemContentList              ResponseInputMessageContentList     `json:",inline"`
	OfResponseOutputMessageContentArray []ResponseOutputMessageContentUnion `json:",inline"`
	OfResponseReasoningItemContentArray []ResponseReasoningItemContent      `json:",inline"`
}

func (u ResponseInputItemUnionContent) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfInputItemContentList, u.OfResponseOutputMessageContentArray, u.OfResponseReasoningItemContentArray)
}

func (u *ResponseInputItemUnionContent) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfInputItemContentList, &u.OfResponseOutputMessageContentArray, &u.OfResponseReasoningItemContentArray)
}

type ResponseInputItemUnionError struct {
	OfString string `json:",inline"`
	Code     int64  `json:"code"`
	Message  string `json:"message"`
	Type     string `json:"type"`
	Content  any    `json:"content"`
}

func (u ResponseInputItemUnionError) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString)
}

func (u *ResponseInputItemUnionError) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString)
}

type ResponseInputItemUnionOutput struct {
	OfString                                      string                                                         `json:",inline"`
	OfResponseFunctionCallOutputItemArray         ResponseFunctionCallOutputItemList                             `json:",inline"`
	OfResponseFunctionShellCallOutputContentArray []ResponseFunctionShellCallOutputContent                       `json:",inline"`
	OfOutputContentList                           []ResponseCustomToolCallOutputOutputOutputContentListItemUnion `json:",inline"`
	Type                                          string                                                         `json:"type"`
	FileID                                        string                                                         `json:"file_id"`
	ImageURL                                      string                                                         `json:"image_url"`
}

func (u ResponseInputItemUnionOutput) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfResponseFunctionCallOutputItemArray, u.OfResponseFunctionShellCallOutputContentArray, u.OfOutputContentList)
}

func (u *ResponseInputItemUnionOutput) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfResponseFunctionCallOutputItemArray, &u.OfResponseFunctionShellCallOutputContentArray, &u.OfOutputContentList)
}

type ResponseInputItemUnionParam struct {
	OfMessage              *EasyInputMessageParam                      `json:",omitzero,inline"`
	OfInputMessage         *ResponseInputItemMessageParam              `json:",omitzero,inline"`
	OfOutputMessage        *ResponseOutputMessageParam                 `json:",omitzero,inline"`
	OfFileSearchCall       *ResponseFileSearchToolCallParam            `json:",omitzero,inline"`
	OfComputerCall         *ResponseComputerToolCallParam              `json:",omitzero,inline"`
	OfComputerCallOutput   *ResponseInputItemComputerCallOutputParam   `json:",omitzero,inline"`
	OfWebSearchCall        *ResponseFunctionWebSearchParam             `json:",omitzero,inline"`
	OfFunctionCall         *ResponseFunctionToolCallParam              `json:",omitzero,inline"`
	OfFunctionCallOutput   *ResponseInputItemFunctionCallOutputParam   `json:",omitzero,inline"`
	OfToolSearchCall       *ResponseInputItemToolSearchCallParam       `json:",omitzero,inline"`
	OfToolSearchOutput     *ResponseToolSearchOutputItemParam          `json:",omitzero,inline"`
	OfAdditionalTools      *ResponseInputItemAdditionalToolsParam      `json:",omitzero,inline"`
	OfReasoning            *ResponseReasoningItemParam                 `json:",omitzero,inline"`
	OfCompaction           *ResponseCompactionItemParam                `json:",omitzero,inline"`
	OfImageGenerationCall  *ResponseInputItemImageGenerationCallParam  `json:",omitzero,inline"`
	OfCodeInterpreterCall  *ResponseCodeInterpreterToolCallParam       `json:",omitzero,inline"`
	OfLocalShellCall       *ResponseInputItemLocalShellCallParam       `json:",omitzero,inline"`
	OfLocalShellCallOutput *ResponseInputItemLocalShellCallOutputParam `json:",omitzero,inline"`
	OfShellCall            *ResponseInputItemShellCallParam            `json:",omitzero,inline"`
	OfShellCallOutput      *ResponseInputItemShellCallOutputParam      `json:",omitzero,inline"`
	OfApplyPatchCall       *ResponseInputItemApplyPatchCallParam       `json:",omitzero,inline"`
	OfApplyPatchCallOutput *ResponseInputItemApplyPatchCallOutputParam `json:",omitzero,inline"`
	OfMcpListTools         *ResponseInputItemMcpListToolsParam         `json:",omitzero,inline"`
	OfMcpApprovalRequest   *ResponseInputItemMcpApprovalRequestParam   `json:",omitzero,inline"`
	OfMcpApprovalResponse  *ResponseInputItemMcpApprovalResponseParam  `json:",omitzero,inline"`
	OfMcpCall              *ResponseInputItemMcpCallParam              `json:",omitzero,inline"`
	OfCustomToolCallOutput *ResponseCustomToolCallOutputParam          `json:",omitzero,inline"`
	OfCustomToolCall       *ResponseCustomToolCallParam                `json:",omitzero,inline"`
	OfCompactionTrigger    *ResponseInputItemCompactionTriggerParam    `json:",omitzero,inline"`
	OfItemReference        *ResponseInputItemItemReferenceParam        `json:",omitzero,inline"`
	OfProgram              *ResponseInputItemProgramParam              `json:",omitzero,inline"`
	OfProgramOutput        *ResponseInputItemProgramOutputParam        `json:",omitzero,inline"`
}

func (u ResponseInputItemUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfMessage, u.OfInputMessage, u.OfOutputMessage, u.OfFileSearchCall, u.OfComputerCall, u.OfComputerCallOutput, u.OfWebSearchCall, u.OfFunctionCall, u.OfFunctionCallOutput, u.OfToolSearchCall, u.OfToolSearchOutput, u.OfAdditionalTools, u.OfReasoning, u.OfCompaction, u.OfImageGenerationCall, u.OfCodeInterpreterCall, u.OfLocalShellCall, u.OfLocalShellCallOutput, u.OfShellCall, u.OfShellCallOutput, u.OfApplyPatchCall, u.OfApplyPatchCallOutput, u.OfMcpListTools, u.OfMcpApprovalRequest, u.OfMcpApprovalResponse, u.OfMcpCall, u.OfCustomToolCallOutput, u.OfCustomToolCall, u.OfCompactionTrigger, u.OfItemReference, u.OfProgram, u.OfProgramOutput)
}

func (u *ResponseInputItemUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "assistant":
		return json.Unmarshal(data, &u.OfOutputMessage)
	case "file_search_call":
		return json.Unmarshal(data, &u.OfFileSearchCall)
	case "computer_call_output":
		return json.Unmarshal(data, &u.OfComputerCallOutput)
	case "web_search_call":
		return json.Unmarshal(data, &u.OfWebSearchCall)
	case "function_call":
		return json.Unmarshal(data, &u.OfFunctionCall)
	case "function_call_output":
		return json.Unmarshal(data, &u.OfFunctionCallOutput)
	case "tool_search_call":
		return json.Unmarshal(data, &u.OfToolSearchCall)
	case "tool_search_output":
		return json.Unmarshal(data, &u.OfToolSearchOutput)
	case "developer":
		return json.Unmarshal(data, &u.OfAdditionalTools)
	case "reasoning":
		return json.Unmarshal(data, &u.OfReasoning)
	case "compaction":
		return json.Unmarshal(data, &u.OfCompaction)
	case "image_generation_call":
		return json.Unmarshal(data, &u.OfImageGenerationCall)
	case "code_interpreter_call":
		return json.Unmarshal(data, &u.OfCodeInterpreterCall)
	case "local_shell_call":
		return json.Unmarshal(data, &u.OfLocalShellCall)
	case "local_shell_call_output":
		return json.Unmarshal(data, &u.OfLocalShellCallOutput)
	case "shell_call":
		return json.Unmarshal(data, &u.OfShellCall)
	case "shell_call_output":
		return json.Unmarshal(data, &u.OfShellCallOutput)
	case "apply_patch_call":
		return json.Unmarshal(data, &u.OfApplyPatchCall)
	case "apply_patch_call_output":
		return json.Unmarshal(data, &u.OfApplyPatchCallOutput)
	case "mcp_list_tools":
		return json.Unmarshal(data, &u.OfMcpListTools)
	case "mcp_approval_request":
		return json.Unmarshal(data, &u.OfMcpApprovalRequest)
	case "mcp_approval_response":
		return json.Unmarshal(data, &u.OfMcpApprovalResponse)
	case "mcp_call":
		return json.Unmarshal(data, &u.OfMcpCall)
	case "custom_tool_call_output":
		return json.Unmarshal(data, &u.OfCustomToolCallOutput)
	case "custom_tool_call":
		return json.Unmarshal(data, &u.OfCustomToolCall)
	case "compaction_trigger":
		return json.Unmarshal(data, &u.OfCompactionTrigger)
	case "program":
		return json.Unmarshal(data, &u.OfProgram)
	case "program_output":
		return json.Unmarshal(data, &u.OfProgramOutput)
	}
	return tryUnmarshal(data, &u.OfMessage, &u.OfInputMessage, &u.OfComputerCall, &u.OfItemReference)
}

type ResponseInputItemUnionTools struct {
	OfToolArray                          []ToolUnion                         `json:",inline"`
	OfResponseInputItemMcpListToolsTools []ResponseInputItemMcpListToolsTool `json:",inline"`
}

func (u ResponseInputItemUnionTools) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfToolArray, u.OfResponseInputItemMcpListToolsTools)
}

func (u *ResponseInputItemUnionTools) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfToolArray, &u.OfResponseInputItemMcpListToolsTools)
}

type ResponseInputMessageContentList []ResponseInputContentUnion

type ResponseInputMessageContentListParam []ResponseInputContentUnionParam

type ResponseInputParam []ResponseInputItemUnionParam

type ResponseInputTextContentParam struct {
	Text                  string                                             `json:"text"`
	PromptCacheBreakpoint ResponseInputTextContentPromptCacheBreakpointParam `json:"prompt_cache_breakpoint,omitzero"`
	Type                  string                                             `json:"type"`
	paramObj
}

func (p ResponseInputTextContentParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputTextContentParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "input_text"})
}

type ResponseInputTextContentPromptCacheBreakpointParam struct {
	Mode string `json:"mode"`
	paramObj
}

func (p ResponseInputTextContentPromptCacheBreakpointParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputTextContentPromptCacheBreakpointParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"mode": "explicit"})
}

type ResponseInputTextParam struct {
	Text                  string                                      `json:"text"`
	PromptCacheBreakpoint ResponseInputTextPromptCacheBreakpointParam `json:"prompt_cache_breakpoint,omitzero"`
	Type                  string                                      `json:"type"`
	paramObj
}

func (p ResponseInputTextParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputTextParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "input_text"})
}

type ResponseInputTextPromptCacheBreakpointParam struct {
	Mode string `json:"mode"`
	paramObj
}

func (p ResponseInputTextPromptCacheBreakpointParam) MarshalJSON() ([]byte, error) {
	type raw ResponseInputTextPromptCacheBreakpointParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"mode": "explicit"})
}

type ResponseInstructionsUnion struct {
	OfString        string                   `json:",inline"`
	OfInputItemList []ResponseInputItemUnion `json:",inline"`
}

func (u ResponseInstructionsUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfInputItemList)
}

func (u *ResponseInstructionsUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfInputItemList)
}

type ResponseModeration struct {
	Input  ResponseModerationInputUnion  `json:"input"`
	Output ResponseModerationOutputUnion `json:"output"`
}

type ResponseModerationInputUnion struct {
	Categories                map[string]bool     `json:"categories"`
	CategoryAppliedInputTypes map[string][]string `json:"category_applied_input_types"`
	CategoryScores            map[string]float64  `json:"category_scores"`
	Flagged                   bool                `json:"flagged"`
	Model                     string              `json:"model"`
	Type                      string              `json:"type"`
	Code                      string              `json:"code"`
	Message                   string              `json:"message"`
}

type ResponseModerationOutputUnion struct {
	Categories                map[string]bool     `json:"categories"`
	CategoryAppliedInputTypes map[string][]string `json:"category_applied_input_types"`
	CategoryScores            map[string]float64  `json:"category_scores"`
	Flagged                   bool                `json:"flagged"`
	Model                     string              `json:"model"`
	Type                      string              `json:"type"`
	Code                      string              `json:"code"`
	Message                   string              `json:"message"`
}

type ResponseNewParams struct {
	Background           Opt[bool]                             `json:"background,omitzero"`
	Instructions         Opt[string]                           `json:"instructions,omitzero"`
	MaxOutputTokens      Opt[int64]                            `json:"max_output_tokens,omitzero"`
	MaxToolCalls         Opt[int64]                            `json:"max_tool_calls,omitzero"`
	ParallelToolCalls    Opt[bool]                             `json:"parallel_tool_calls,omitzero"`
	PreviousResponseID   Opt[string]                           `json:"previous_response_id,omitzero"`
	PromptCacheKey       Opt[string]                           `json:"prompt_cache_key,omitzero"`
	SafetyIdentifier     Opt[string]                           `json:"safety_identifier,omitzero"`
	Store                Opt[bool]                             `json:"store,omitzero"`
	Temperature          Opt[float64]                          `json:"temperature,omitzero"`
	TopLogprobs          Opt[int64]                            `json:"top_logprobs,omitzero"`
	TopP                 Opt[float64]                          `json:"top_p,omitzero"`
	User                 Opt[string]                           `json:"user,omitzero"`
	ContextManagement    []ResponseNewParamsContextManagement  `json:"context_management,omitzero"`
	Conversation         ResponseNewParamsConversationUnion    `json:"conversation,omitzero"`
	Include              []ResponseIncludable                  `json:"include,omitzero"`
	Metadata             Metadata                              `json:"metadata,omitzero"`
	Moderation           ResponseNewParamsModeration           `json:"moderation,omitzero"`
	Prompt               ResponsePromptParam                   `json:"prompt,omitzero"`
	PromptCacheRetention ResponseNewParamsPromptCacheRetention `json:"prompt_cache_retention,omitzero"`
	ServiceTier          ResponseNewParamsServiceTier          `json:"service_tier,omitzero"`
	StreamOptions        ResponseNewParamsStreamOptions        `json:"stream_options,omitzero"`
	Truncation           ResponseNewParamsTruncation           `json:"truncation,omitzero"`
	Input                ResponseNewParamsInputUnion           `json:"input,omitzero"`
	Model                ResponsesModel                        `json:"model,omitzero"`
	PromptCacheOptions   ResponseNewParamsPromptCacheOptions   `json:"prompt_cache_options,omitzero"`
	Reasoning            ReasoningParam                        `json:"reasoning,omitzero"`
	Text                 ResponseTextConfigParam               `json:"text,omitzero"`
	ToolChoice           ResponseNewParamsToolChoiceUnion      `json:"tool_choice,omitzero"`
	Tools                []ToolUnionParam                      `json:"tools,omitzero"`
	paramObj
}

func (p ResponseNewParams) MarshalJSON() ([]byte, error) {
	type raw ResponseNewParams
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseNewParamsContextManagement struct {
	Type             string     `json:"type"`
	CompactThreshold Opt[int64] `json:"compact_threshold,omitzero"`
	paramObj
}

func (p ResponseNewParamsContextManagement) MarshalJSON() ([]byte, error) {
	type raw ResponseNewParamsContextManagement
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseNewParamsConversationUnion struct {
	OfString             Opt[string]                `json:",omitzero,inline"`
	OfConversationObject *ResponseConversationParam `json:",omitzero,inline"`
}

func (u ResponseNewParamsConversationUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfConversationObject)
}

func (u *ResponseNewParamsConversationUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfConversationObject)
}

type ResponseNewParamsInputUnion struct {
	OfString        Opt[string]        `json:",omitzero,inline"`
	OfInputItemList ResponseInputParam `json:",omitzero,inline"`
}

func (u ResponseNewParamsInputUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfInputItemList)
}

func (u *ResponseNewParamsInputUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfInputItemList)
}

type ResponseNewParamsModeration struct {
	Model  string                            `json:"model"`
	Policy ResponseNewParamsModerationPolicy `json:"policy,omitzero"`
	paramObj
}

func (p ResponseNewParamsModeration) MarshalJSON() ([]byte, error) {
	type raw ResponseNewParamsModeration
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseNewParamsModerationPolicy struct {
	Input  ResponseNewParamsModerationPolicyInput  `json:"input,omitzero"`
	Output ResponseNewParamsModerationPolicyOutput `json:"output,omitzero"`
	paramObj
}

func (p ResponseNewParamsModerationPolicy) MarshalJSON() ([]byte, error) {
	type raw ResponseNewParamsModerationPolicy
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseNewParamsModerationPolicyInput struct {
	Mode string `json:"mode,omitzero"`
	paramObj
}

func (p ResponseNewParamsModerationPolicyInput) MarshalJSON() ([]byte, error) {
	type raw ResponseNewParamsModerationPolicyInput
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseNewParamsModerationPolicyOutput struct {
	Mode string `json:"mode,omitzero"`
	paramObj
}

func (p ResponseNewParamsModerationPolicyOutput) MarshalJSON() ([]byte, error) {
	type raw ResponseNewParamsModerationPolicyOutput
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseNewParamsPromptCacheOptions struct {
	Mode string `json:"mode,omitzero"`
	Ttl  string `json:"ttl,omitzero"`
	paramObj
}

func (p ResponseNewParamsPromptCacheOptions) MarshalJSON() ([]byte, error) {
	type raw ResponseNewParamsPromptCacheOptions
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseNewParamsPromptCacheRetention string

type ResponseNewParamsServiceTier string

type ResponseNewParamsStreamOptions struct {
	IncludeObfuscation Opt[bool] `json:"include_obfuscation,omitzero"`
	paramObj
}

func (p ResponseNewParamsStreamOptions) MarshalJSON() ([]byte, error) {
	type raw ResponseNewParamsStreamOptions
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseNewParamsToolChoiceSpecificProgrammaticToolCallingParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ResponseNewParamsToolChoiceSpecificProgrammaticToolCallingParam) MarshalJSON() ([]byte, error) {
	type raw ResponseNewParamsToolChoiceSpecificProgrammaticToolCallingParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "programmatic_tool_calling"})
}

type ResponseNewParamsToolChoiceUnion struct {
	OfToolChoiceMode                                             Opt[ToolChoiceOptions]                                           `json:",omitzero,inline"`
	OfAllowedTools                                               *ToolChoiceAllowedParam                                          `json:",omitzero,inline"`
	OfHostedTool                                                 *ToolChoiceTypesParam                                            `json:",omitzero,inline"`
	OfFunctionTool                                               *ToolChoiceFunctionParam                                         `json:",omitzero,inline"`
	OfMcpTool                                                    *ToolChoiceMcpParam                                              `json:",omitzero,inline"`
	OfCustomTool                                                 *ToolChoiceCustomParam                                           `json:",omitzero,inline"`
	OfResponseNewsToolChoiceSpecificProgrammaticToolCallingParam *ResponseNewParamsToolChoiceSpecificProgrammaticToolCallingParam `json:",omitzero,inline"`
	OfSpecificApplyPatchToolChoice                               *ToolChoiceApplyPatchParam                                       `json:",omitzero,inline"`
	OfSpecificShellToolChoice                                    *ToolChoiceShellParam                                            `json:",omitzero,inline"`
}

func (u ResponseNewParamsToolChoiceUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfToolChoiceMode, u.OfAllowedTools, u.OfHostedTool, u.OfFunctionTool, u.OfMcpTool, u.OfCustomTool, u.OfResponseNewsToolChoiceSpecificProgrammaticToolCallingParam, u.OfSpecificApplyPatchToolChoice, u.OfSpecificShellToolChoice)
}

func (u *ResponseNewParamsToolChoiceUnion) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "allowed_tools":
		return json.Unmarshal(data, &u.OfAllowedTools)
	case "function":
		return json.Unmarshal(data, &u.OfFunctionTool)
	case "mcp":
		return json.Unmarshal(data, &u.OfMcpTool)
	case "custom":
		return json.Unmarshal(data, &u.OfCustomTool)
	case "programmatic_tool_calling":
		return json.Unmarshal(data, &u.OfResponseNewsToolChoiceSpecificProgrammaticToolCallingParam)
	case "apply_patch":
		return json.Unmarshal(data, &u.OfSpecificApplyPatchToolChoice)
	case "shell":
		return json.Unmarshal(data, &u.OfSpecificShellToolChoice)
	}
	return tryUnmarshal(data, &u.OfToolChoiceMode, &u.OfHostedTool)
}

type ResponseNewParamsTruncation string

type ResponseOutputItemMcpListToolsTool struct {
	InputSchema any    `json:"input_schema"`
	Name        string `json:"name"`
	Annotations any    `json:"annotations"`
	Description string `json:"description"`
}

type ResponseOutputItemUnion struct {
	ID                       string                                                      `json:"id"`
	Content                  []ResponseOutputMessageContentUnion                         `json:"content"`
	Role                     string                                                      `json:"role"`
	Status                   string                                                      `json:"status"`
	Type                     string                                                      `json:"type"`
	Phase                    ResponseOutputMessagePhase                                  `json:"phase"`
	Queries                  []string                                                    `json:"queries"`
	Results                  []ResponseFileSearchToolCallResult                          `json:"results"`
	Arguments                ResponseOutputItemUnionArguments                            `json:"arguments"`
	CallID                   string                                                      `json:"call_id"`
	Name                     string                                                      `json:"name"`
	Caller                   ResponseOutputItemUnionCaller                               `json:"caller"`
	Namespace                string                                                      `json:"namespace"`
	Output                   ResponseOutputItemUnionOutput                               `json:"output"`
	CreatedBy                string                                                      `json:"created_by"`
	Action                   ResponseOutputItemUnionAction                               `json:"action"`
	PendingSafetyChecks      []ResponseComputerToolCallPendingSafetyCheck                `json:"pending_safety_checks"`
	Actions                  ComputerActionList                                          `json:"actions"`
	AcknowledgedSafetyChecks []ResponseComputerToolCallOutputItemAcknowledgedSafetyCheck `json:"acknowledged_safety_checks"`
	Summary                  []ResponseReasoningItemSummary                              `json:"summary"`
	EncryptedContent         string                                                      `json:"encrypted_content"`
	Code                     string                                                      `json:"code"`
	Fingerprint              string                                                      `json:"fingerprint"`
	Result                   string                                                      `json:"result"`
	Execution                string                                                      `json:"execution"`
	Tools                    ResponseOutputItemUnionTools                                `json:"tools"`
	ContainerID              string                                                      `json:"container_id"`
	Outputs                  []ResponseCodeInterpreterToolCallOutputUnion                `json:"outputs"`
	Environment              ResponseFunctionShellToolCallEnvironmentUnion               `json:"environment"`
	MaxOutputLength          int64                                                       `json:"max_output_length"`
	Operation                ResponseApplyPatchToolCallOperationUnion                    `json:"operation"`
	ServerLabel              string                                                      `json:"server_label"`
	ApprovalRequestID        string                                                      `json:"approval_request_id"`
	Error                    ResponseOutputItemUnionError                                `json:"error"`
	Approve                  bool                                                        `json:"approve"`
	Reason                   string                                                      `json:"reason"`
	Input                    string                                                      `json:"input"`
}

type ResponseOutputItemUnionAction struct {
	Type             string                                        `json:"type"`
	Queries          []string                                      `json:"queries"`
	Query            string                                        `json:"query"`
	Sources          []ResponseFunctionWebSearchActionSearchSource `json:"sources"`
	URL              string                                        `json:"url"`
	Pattern          string                                        `json:"pattern"`
	Button           string                                        `json:"button"`
	X                int64                                         `json:"x"`
	Y                int64                                         `json:"y"`
	Keys             []string                                      `json:"keys"`
	Path             []ResponseComputerToolCallActionDragPath      `json:"path"`
	ScrollX          int64                                         `json:"scroll_x"`
	ScrollY          int64                                         `json:"scroll_y"`
	Text             string                                        `json:"text"`
	Command          []string                                      `json:"command"`
	Env              map[string]string                             `json:"env"`
	TimeoutMs        int64                                         `json:"timeout_ms"`
	User             string                                        `json:"user"`
	WorkingDirectory string                                        `json:"working_directory"`
	Commands         []string                                      `json:"commands"`
	MaxOutputLength  int64                                         `json:"max_output_length"`
}

type ResponseOutputItemUnionArguments struct {
	OfString                          string `json:",inline"`
	OfResponseToolSearchCallArguments any    `json:",inline"`
}

func (u ResponseOutputItemUnionArguments) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfResponseToolSearchCallArguments)
}

func (u *ResponseOutputItemUnionArguments) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfResponseToolSearchCallArguments)
}

type ResponseOutputItemUnionCaller struct {
	Type     string `json:"type"`
	CallerID string `json:"caller_id"`
}

type ResponseOutputItemUnionError struct {
	OfString string `json:",inline"`
	Code     int64  `json:"code"`
	Message  string `json:"message"`
	Type     string `json:"type"`
	Content  any    `json:"content"`
}

func (u ResponseOutputItemUnionError) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString)
}

func (u *ResponseOutputItemUnionError) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString)
}

type ResponseOutputItemUnionOutput struct {
	OfString                                         string                                                               `json:",inline"`
	OfOutputContentList                              []ResponseFunctionToolCallOutputItemOutputOutputContentListItemUnion `json:",inline"`
	OfResponseFunctionShellToolCallOutputOutputArray []ResponseFunctionShellToolCallOutputOutput                          `json:",inline"`
	Type                                             string                                                               `json:"type"`
	FileID                                           string                                                               `json:"file_id"`
	ImageURL                                         string                                                               `json:"image_url"`
}

func (u ResponseOutputItemUnionOutput) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfOutputContentList, u.OfResponseFunctionShellToolCallOutputOutputArray)
}

func (u *ResponseOutputItemUnionOutput) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfOutputContentList, &u.OfResponseFunctionShellToolCallOutputOutputArray)
}

type ResponseOutputItemUnionTools struct {
	OfToolArray                           []ToolUnion                          `json:",inline"`
	OfResponseOutputItemMcpListToolsTools []ResponseOutputItemMcpListToolsTool `json:",inline"`
}

func (u ResponseOutputItemUnionTools) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfToolArray, u.OfResponseOutputItemMcpListToolsTools)
}

func (u *ResponseOutputItemUnionTools) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfToolArray, &u.OfResponseOutputItemMcpListToolsTools)
}

type ResponseOutputMessage struct {
	ID      string                              `json:"id"`
	Content []ResponseOutputMessageContentUnion `json:"content"`
	Role    string                              `json:"role"`
	Status  ResponseOutputMessageStatus         `json:"status"`
	Type    string                              `json:"type"`
	Phase   ResponseOutputMessagePhase          `json:"phase"`
}

type ResponseOutputMessageContentUnion struct {
	Annotations []ResponseOutputTextAnnotationUnion `json:"annotations"`
	Text        string                              `json:"text"`
	Type        string                              `json:"type"`
	Logprobs    []ResponseOutputTextLogprob         `json:"logprobs"`
	Refusal     string                              `json:"refusal"`
}

type ResponseOutputMessageContentUnionParam struct {
	OfOutputText *ResponseOutputTextParam    `json:",omitzero,inline"`
	OfRefusal    *ResponseOutputRefusalParam `json:",omitzero,inline"`
}

func (u ResponseOutputMessageContentUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfOutputText, u.OfRefusal)
}

func (u *ResponseOutputMessageContentUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "output_text":
		return json.Unmarshal(data, &u.OfOutputText)
	case "refusal":
		return json.Unmarshal(data, &u.OfRefusal)
	}
	return tryUnmarshal(data)
}

type ResponseOutputMessageParam struct {
	ID      string                                   `json:"id,omitzero"`
	Content []ResponseOutputMessageContentUnionParam `json:"content,omitzero"`
	Status  ResponseOutputMessageStatus              `json:"status,omitzero"`
	Phase   ResponseOutputMessagePhase               `json:"phase,omitzero"`
	Role    string                                   `json:"role"`
	Type    string                                   `json:"type"`
	paramObj
}

func (p ResponseOutputMessageParam) MarshalJSON() ([]byte, error) {
	type raw ResponseOutputMessageParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"role": "assistant", "type": "message"})
}

type ResponseOutputMessagePhase string

type ResponseOutputMessageStatus string

type ResponseOutputRefusal struct {
	Refusal string `json:"refusal"`
	Type    string `json:"type"`
}

type ResponseOutputRefusalParam struct {
	Refusal string `json:"refusal"`
	Type    string `json:"type"`
	paramObj
}

func (p ResponseOutputRefusalParam) MarshalJSON() ([]byte, error) {
	type raw ResponseOutputRefusalParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "refusal"})
}

type ResponseOutputText struct {
	Annotations []ResponseOutputTextAnnotationUnion `json:"annotations"`
	Text        string                              `json:"text"`
	Type        string                              `json:"type"`
	Logprobs    []ResponseOutputTextLogprob         `json:"logprobs"`
}

type ResponseOutputTextAnnotationAddedEventAnnotationUnion struct {
	FileID      string `json:"file_id"`
	Filename    string `json:"filename"`
	Index       int64  `json:"index"`
	Type        string `json:"type"`
	EndIndex    int64  `json:"end_index"`
	StartIndex  int64  `json:"start_index"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	ContainerID string `json:"container_id"`
}

type ResponseOutputTextAnnotationContainerFileCitationParam struct {
	ContainerID string `json:"container_id"`
	EndIndex    int64  `json:"end_index"`
	FileID      string `json:"file_id"`
	Filename    string `json:"filename"`
	StartIndex  int64  `json:"start_index"`
	Type        string `json:"type"`
	paramObj
}

func (p ResponseOutputTextAnnotationContainerFileCitationParam) MarshalJSON() ([]byte, error) {
	type raw ResponseOutputTextAnnotationContainerFileCitationParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "container_file_citation"})
}

type ResponseOutputTextAnnotationFileCitationParam struct {
	FileID   string `json:"file_id"`
	Filename string `json:"filename"`
	Index    int64  `json:"index"`
	Type     string `json:"type"`
	paramObj
}

func (p ResponseOutputTextAnnotationFileCitationParam) MarshalJSON() ([]byte, error) {
	type raw ResponseOutputTextAnnotationFileCitationParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "file_citation"})
}

type ResponseOutputTextAnnotationFilePathParam struct {
	FileID string `json:"file_id"`
	Index  int64  `json:"index"`
	Type   string `json:"type"`
	paramObj
}

func (p ResponseOutputTextAnnotationFilePathParam) MarshalJSON() ([]byte, error) {
	type raw ResponseOutputTextAnnotationFilePathParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "file_path"})
}

type ResponseOutputTextAnnotationURLCitationParam struct {
	EndIndex   int64  `json:"end_index"`
	StartIndex int64  `json:"start_index"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Type       string `json:"type"`
	paramObj
}

func (p ResponseOutputTextAnnotationURLCitationParam) MarshalJSON() ([]byte, error) {
	type raw ResponseOutputTextAnnotationURLCitationParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "url_citation"})
}

type ResponseOutputTextAnnotationUnion struct {
	FileID      string `json:"file_id"`
	Filename    string `json:"filename"`
	Index       int64  `json:"index"`
	Type        string `json:"type"`
	EndIndex    int64  `json:"end_index"`
	StartIndex  int64  `json:"start_index"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	ContainerID string `json:"container_id"`
}

type ResponseOutputTextAnnotationUnionParam struct {
	OfFileCitation          *ResponseOutputTextAnnotationFileCitationParam          `json:",omitzero,inline"`
	OfURLCitation           *ResponseOutputTextAnnotationURLCitationParam           `json:",omitzero,inline"`
	OfContainerFileCitation *ResponseOutputTextAnnotationContainerFileCitationParam `json:",omitzero,inline"`
	OfFilePath              *ResponseOutputTextAnnotationFilePathParam              `json:",omitzero,inline"`
}

func (u ResponseOutputTextAnnotationUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfFileCitation, u.OfURLCitation, u.OfContainerFileCitation, u.OfFilePath)
}

func (u *ResponseOutputTextAnnotationUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "file_citation":
		return json.Unmarshal(data, &u.OfFileCitation)
	case "url_citation":
		return json.Unmarshal(data, &u.OfURLCitation)
	case "container_file_citation":
		return json.Unmarshal(data, &u.OfContainerFileCitation)
	case "file_path":
		return json.Unmarshal(data, &u.OfFilePath)
	}
	return tryUnmarshal(data)
}

type ResponseOutputTextLogprob struct {
	Token       string                                `json:"token"`
	Bytes       []int64                               `json:"bytes"`
	Logprob     float64                               `json:"logprob"`
	TopLogprobs []ResponseOutputTextLogprobTopLogprob `json:"top_logprobs"`
}

type ResponseOutputTextLogprobParam struct {
	Token       string                                     `json:"token"`
	Bytes       []int64                                    `json:"bytes,omitzero"`
	Logprob     float64                                    `json:"logprob"`
	TopLogprobs []ResponseOutputTextLogprobTopLogprobParam `json:"top_logprobs,omitzero"`
	paramObj
}

func (p ResponseOutputTextLogprobParam) MarshalJSON() ([]byte, error) {
	type raw ResponseOutputTextLogprobParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseOutputTextLogprobTopLogprob struct {
	Token   string  `json:"token"`
	Bytes   []int64 `json:"bytes"`
	Logprob float64 `json:"logprob"`
}

type ResponseOutputTextLogprobTopLogprobParam struct {
	Token   string  `json:"token"`
	Bytes   []int64 `json:"bytes,omitzero"`
	Logprob float64 `json:"logprob"`
	paramObj
}

func (p ResponseOutputTextLogprobTopLogprobParam) MarshalJSON() ([]byte, error) {
	type raw ResponseOutputTextLogprobTopLogprobParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseOutputTextParam struct {
	Annotations []ResponseOutputTextAnnotationUnionParam `json:"annotations,omitzero"`
	Text        string                                   `json:"text"`
	Logprobs    []ResponseOutputTextLogprobParam         `json:"logprobs,omitzero"`
	Type        string                                   `json:"type"`
	paramObj
}

func (p ResponseOutputTextParam) MarshalJSON() ([]byte, error) {
	type raw ResponseOutputTextParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "output_text"})
}

type ResponsePrompt struct {
	ID        string                                 `json:"id"`
	Variables map[string]ResponsePromptVariableUnion `json:"variables"`
	Version   string                                 `json:"version"`
}

type ResponsePromptCacheOptions struct {
	Mode string `json:"mode"`
	Ttl  string `json:"ttl"`
}

type ResponsePromptCacheRetention string

type ResponsePromptParam struct {
	ID        string                                      `json:"id"`
	Version   Opt[string]                                 `json:"version,omitzero"`
	Variables map[string]ResponsePromptVariableUnionParam `json:"variables,omitzero"`
	paramObj
}

func (p ResponsePromptParam) MarshalJSON() ([]byte, error) {
	type raw ResponsePromptParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponsePromptVariableUnion struct {
	OfString              string                                           `json:",inline"`
	Text                  string                                           `json:"text"`
	Type                  string                                           `json:"type"`
	PromptCacheBreakpoint ResponsePromptVariableUnionPromptCacheBreakpoint `json:"prompt_cache_breakpoint"`
	Detail                string                                           `json:"detail"`
	FileID                string                                           `json:"file_id"`
	ImageURL              string                                           `json:"image_url"`
	FileData              string                                           `json:"file_data"`
	FileURL               string                                           `json:"file_url"`
	Filename              string                                           `json:"filename"`
}

func (u ResponsePromptVariableUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString)
}

func (u *ResponsePromptVariableUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString)
}

type ResponsePromptVariableUnionParam struct {
	OfString     Opt[string]              `json:",omitzero,inline"`
	OfInputText  *ResponseInputTextParam  `json:",omitzero,inline"`
	OfInputImage *ResponseInputImageParam `json:",omitzero,inline"`
	OfInputFile  *ResponseInputFileParam  `json:",omitzero,inline"`
}

func (u ResponsePromptVariableUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfInputText, u.OfInputImage, u.OfInputFile)
}

func (u *ResponsePromptVariableUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "input_text":
		return json.Unmarshal(data, &u.OfInputText)
	case "input_image":
		return json.Unmarshal(data, &u.OfInputImage)
	case "input_file":
		return json.Unmarshal(data, &u.OfInputFile)
	}
	return tryUnmarshal(data, &u.OfString)
}

type ResponsePromptVariableUnionPromptCacheBreakpoint struct {
	Mode string `json:"mode"`
}

type ResponseReasoningItem struct {
	ID               string                         `json:"id"`
	Summary          []ResponseReasoningItemSummary `json:"summary"`
	Type             string                         `json:"type"`
	Content          []ResponseReasoningItemContent `json:"content"`
	EncryptedContent string                         `json:"encrypted_content"`
	Status           ResponseReasoningItemStatus    `json:"status"`
}

type ResponseReasoningItemContent struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

type ResponseReasoningItemContentParam struct {
	Text string `json:"text"`
	Type string `json:"type"`
	paramObj
}

func (p ResponseReasoningItemContentParam) MarshalJSON() ([]byte, error) {
	type raw ResponseReasoningItemContentParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "reasoning_text"})
}

type ResponseReasoningItemParam struct {
	ID               string                              `json:"id"`
	Summary          []ResponseReasoningItemSummaryParam `json:"summary,omitzero"`
	EncryptedContent Opt[string]                         `json:"encrypted_content,omitzero"`
	Content          []ResponseReasoningItemContentParam `json:"content,omitzero"`
	Status           ResponseReasoningItemStatus         `json:"status,omitzero"`
	Type             string                              `json:"type"`
	paramObj
}

func (p ResponseReasoningItemParam) MarshalJSON() ([]byte, error) {
	type raw ResponseReasoningItemParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "reasoning"})
}

type ResponseReasoningItemStatus string

type ResponseReasoningItemSummary struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

type ResponseReasoningItemSummaryParam struct {
	Text string `json:"text"`
	Type string `json:"type"`
	paramObj
}

func (p ResponseReasoningItemSummaryParam) MarshalJSON() ([]byte, error) {
	type raw ResponseReasoningItemSummaryParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "summary_text"})
}

type ResponseReasoningSummaryPartAddedEventPart struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

type ResponseReasoningSummaryPartDoneEventPart struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

type ResponseReasoningSummaryPartDoneEventStatus string

type ResponseServiceTier string

type ResponseShellCallOutputContentDeltaEventDelta struct {
	Stderr string `json:"stderr"`
	Stdout string `json:"stdout"`
}

type ResponseShellCallOutputContentDoneEventOutput struct {
	Outcome   ResponseShellCallOutputContentDoneEventOutputOutcomeUnion `json:"outcome"`
	Stderr    string                                                    `json:"stderr"`
	Stdout    string                                                    `json:"stdout"`
	CreatedBy string                                                    `json:"created_by"`
}

type ResponseShellCallOutputContentDoneEventOutputOutcomeUnion struct {
	Type     string `json:"type"`
	ExitCode int64  `json:"exit_code"`
}

type ResponseStatus string

type ResponseTextConfig struct {
	Format    ResponseFormatTextConfigUnion `json:"format"`
	Verbosity ResponseTextConfigVerbosity   `json:"verbosity"`
}

type ResponseTextConfigParam struct {
	Verbosity ResponseTextConfigVerbosity        `json:"verbosity,omitzero"`
	Format    ResponseFormatTextConfigUnionParam `json:"format,omitzero"`
	paramObj
}

func (p ResponseTextConfigParam) MarshalJSON() ([]byte, error) {
	type raw ResponseTextConfigParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ResponseTextConfigVerbosity string

type ResponseTextDeltaEventLogprob struct {
	Token       string                                    `json:"token"`
	Logprob     float64                                   `json:"logprob"`
	TopLogprobs []ResponseTextDeltaEventLogprobTopLogprob `json:"top_logprobs"`
}

type ResponseTextDeltaEventLogprobTopLogprob struct {
	Token   string  `json:"token"`
	Logprob float64 `json:"logprob"`
}

type ResponseTextDoneEventLogprob struct {
	Token       string                                   `json:"token"`
	Logprob     float64                                  `json:"logprob"`
	TopLogprobs []ResponseTextDoneEventLogprobTopLogprob `json:"top_logprobs"`
}

type ResponseTextDoneEventLogprobTopLogprob struct {
	Token   string  `json:"token"`
	Logprob float64 `json:"logprob"`
}

type ResponseToolChoiceUnion struct {
	OfToolChoiceMode ToolChoiceOptions     `json:",inline"`
	Mode             ToolChoiceAllowedMode `json:"mode"`
	Tools            []map[string]any      `json:"tools"`
	Type             string                `json:"type"`
	Name             string                `json:"name"`
	ServerLabel      string                `json:"server_label"`
}

func (u ResponseToolChoiceUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfToolChoiceMode)
}

func (u *ResponseToolChoiceUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfToolChoiceMode)
}

type ResponseToolSearchOutputItemParam struct {
	Tools     []ToolUnionParam                           `json:"tools,omitzero"`
	ID        Opt[string]                                `json:"id,omitzero"`
	CallID    Opt[string]                                `json:"call_id,omitzero"`
	Status    ResponseToolSearchOutputItemParamStatus    `json:"status,omitzero"`
	Execution ResponseToolSearchOutputItemParamExecution `json:"execution,omitzero"`
	Type      string                                     `json:"type"`
	paramObj
}

func (p ResponseToolSearchOutputItemParam) MarshalJSON() ([]byte, error) {
	type raw ResponseToolSearchOutputItemParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "tool_search_output"})
}

type ResponseToolSearchOutputItemParamExecution string

type ResponseToolSearchOutputItemParamStatus string

type ResponseTruncation string

type ResponseUsage struct {
	InputTokens         int64                            `json:"input_tokens"`
	InputTokensDetails  ResponseUsageInputTokensDetails  `json:"input_tokens_details"`
	OutputTokens        int64                            `json:"output_tokens"`
	OutputTokensDetails ResponseUsageOutputTokensDetails `json:"output_tokens_details"`
	TotalTokens         int64                            `json:"total_tokens"`
}

type ResponseUsageInputTokensDetails struct {
	CacheWriteTokens     int64 `json:"cache_write_tokens"`
	CachedTokens         int64 `json:"cached_tokens"`
	CachedTokensReported bool  `json:"-"`
}

type ResponseUsageOutputTokensDetails struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

type SkillReferenceParam struct {
	SkillID string      `json:"skill_id"`
	Version Opt[string] `json:"version,omitzero"`
	Type    string      `json:"type"`
	paramObj
}

func (p SkillReferenceParam) MarshalJSON() ([]byte, error) {
	type raw SkillReferenceParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "skill_reference"})
}

type ToolChoiceAllowedMode string

type ToolChoiceAllowedParam struct {
	Mode  ToolChoiceAllowedMode `json:"mode,omitzero"`
	Tools []map[string]any      `json:"tools,omitzero"`
	Type  string                `json:"type"`
	paramObj
}

func (p ToolChoiceAllowedParam) MarshalJSON() ([]byte, error) {
	type raw ToolChoiceAllowedParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "allowed_tools"})
}

type ToolChoiceApplyPatchParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ToolChoiceApplyPatchParam) MarshalJSON() ([]byte, error) {
	type raw ToolChoiceApplyPatchParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "apply_patch"})
}

type ToolChoiceCustomParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
	paramObj
}

func (p ToolChoiceCustomParam) MarshalJSON() ([]byte, error) {
	type raw ToolChoiceCustomParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "custom"})
}

type ToolChoiceFunctionParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
	paramObj
}

func (p ToolChoiceFunctionParam) MarshalJSON() ([]byte, error) {
	type raw ToolChoiceFunctionParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "function"})
}

type ToolChoiceMcpParam struct {
	ServerLabel string      `json:"server_label"`
	Name        Opt[string] `json:"name,omitzero"`
	Type        string      `json:"type"`
	paramObj
}

func (p ToolChoiceMcpParam) MarshalJSON() ([]byte, error) {
	type raw ToolChoiceMcpParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "mcp"})
}

type ToolChoiceOptions string

type ToolChoiceShellParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ToolChoiceShellParam) MarshalJSON() ([]byte, error) {
	type raw ToolChoiceShellParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "shell"})
}

type ToolChoiceTypesParam struct {
	Type ToolChoiceTypesType `json:"type,omitzero"`
	paramObj
}

func (p ToolChoiceTypesParam) MarshalJSON() ([]byte, error) {
	type raw ToolChoiceTypesParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ToolChoiceTypesType string

type ToolCodeInterpreterContainerCodeInterpreterContainerAutoParam struct {
	MemoryLimit   string                                                                     `json:"memory_limit,omitzero"`
	FileIDs       []string                                                                   `json:"file_ids,omitzero"`
	NetworkPolicy ToolCodeInterpreterContainerCodeInterpreterToolAutoNetworkPolicyUnionParam `json:"network_policy,omitzero"`
	Type          string                                                                     `json:"type"`
	paramObj
}

func (p ToolCodeInterpreterContainerCodeInterpreterContainerAutoParam) MarshalJSON() ([]byte, error) {
	type raw ToolCodeInterpreterContainerCodeInterpreterContainerAutoParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "auto"})
}

type ToolCodeInterpreterContainerCodeInterpreterToolAutoNetworkPolicyUnion struct {
	Type           string                               `json:"type"`
	AllowedDomains []string                             `json:"allowed_domains"`
	DomainSecrets  []ContainerNetworkPolicyDomainSecret `json:"domain_secrets"`
}

type ToolCodeInterpreterContainerCodeInterpreterToolAutoNetworkPolicyUnionParam struct {
	OfDisabled  *ContainerNetworkPolicyDisabledParam  `json:",omitzero,inline"`
	OfAllowlist *ContainerNetworkPolicyAllowlistParam `json:",omitzero,inline"`
}

func (u ToolCodeInterpreterContainerCodeInterpreterToolAutoNetworkPolicyUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfDisabled, u.OfAllowlist)
}

func (u *ToolCodeInterpreterContainerCodeInterpreterToolAutoNetworkPolicyUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "disabled":
		return json.Unmarshal(data, &u.OfDisabled)
	case "allowlist":
		return json.Unmarshal(data, &u.OfAllowlist)
	}
	return tryUnmarshal(data)
}

type ToolCodeInterpreterContainerUnion struct {
	OfString      string                                                                `json:",inline"`
	Type          string                                                                `json:"type"`
	FileIDs       []string                                                              `json:"file_ids"`
	MemoryLimit   string                                                                `json:"memory_limit"`
	NetworkPolicy ToolCodeInterpreterContainerCodeInterpreterToolAutoNetworkPolicyUnion `json:"network_policy"`
}

func (u ToolCodeInterpreterContainerUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString)
}

func (u *ToolCodeInterpreterContainerUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString)
}

type ToolCodeInterpreterContainerUnionParam struct {
	OfString                  Opt[string]                                                    `json:",omitzero,inline"`
	OfCodeInterpreterToolAuto *ToolCodeInterpreterContainerCodeInterpreterContainerAutoParam `json:",omitzero,inline"`
}

func (u ToolCodeInterpreterContainerUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfCodeInterpreterToolAuto)
}

func (u *ToolCodeInterpreterContainerUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "auto":
		return json.Unmarshal(data, &u.OfCodeInterpreterToolAuto)
	}
	return tryUnmarshal(data, &u.OfString)
}

type ToolCodeInterpreterParam struct {
	Container      ToolCodeInterpreterContainerUnionParam `json:"container,omitzero"`
	AllowedCallers []string                               `json:"allowed_callers,omitzero"`
	Type           string                                 `json:"type"`
	paramObj
}

func (p ToolCodeInterpreterParam) MarshalJSON() ([]byte, error) {
	type raw ToolCodeInterpreterParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "code_interpreter"})
}

type ToolImageGenerationInputImageMask struct {
	FileID   string `json:"file_id"`
	ImageURL string `json:"image_url"`
}

type ToolImageGenerationInputImageMaskParam struct {
	FileID   Opt[string] `json:"file_id,omitzero"`
	ImageURL Opt[string] `json:"image_url,omitzero"`
	paramObj
}

func (p ToolImageGenerationInputImageMaskParam) MarshalJSON() ([]byte, error) {
	type raw ToolImageGenerationInputImageMaskParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ToolImageGenerationParam struct {
	OutputCompression Opt[int64]                             `json:"output_compression,omitzero"`
	PartialImages     Opt[int64]                             `json:"partial_images,omitzero"`
	InputFidelity     string                                 `json:"input_fidelity,omitzero"`
	Action            string                                 `json:"action,omitzero"`
	Background        string                                 `json:"background,omitzero"`
	InputImageMask    ToolImageGenerationInputImageMaskParam `json:"input_image_mask,omitzero"`
	Model             string                                 `json:"model,omitzero"`
	Moderation        string                                 `json:"moderation,omitzero"`
	OutputFormat      string                                 `json:"output_format,omitzero"`
	Quality           string                                 `json:"quality,omitzero"`
	Size              string                                 `json:"size,omitzero"`
	Type              string                                 `json:"type"`
	paramObj
}

func (p ToolImageGenerationParam) MarshalJSON() ([]byte, error) {
	type raw ToolImageGenerationParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "image_generation"})
}

type ToolLocalShellParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ToolLocalShellParam) MarshalJSON() ([]byte, error) {
	type raw ToolLocalShellParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "local_shell"})
}

type ToolMcpAllowedToolsMcpToolFilterParam struct {
	ReadOnly  Opt[bool] `json:"read_only,omitzero"`
	ToolNames []string  `json:"tool_names,omitzero"`
	paramObj
}

func (p ToolMcpAllowedToolsMcpToolFilterParam) MarshalJSON() ([]byte, error) {
	type raw ToolMcpAllowedToolsMcpToolFilterParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ToolMcpAllowedToolsUnion struct {
	OfMcpAllowedTools []string `json:",inline"`
	ReadOnly          bool     `json:"read_only"`
	ToolNames         []string `json:"tool_names"`
}

func (u ToolMcpAllowedToolsUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfMcpAllowedTools)
}

func (u *ToolMcpAllowedToolsUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfMcpAllowedTools)
}

type ToolMcpAllowedToolsUnionParam struct {
	OfMcpAllowedTools []string                               `json:",omitzero,inline"`
	OfMcpToolFilter   *ToolMcpAllowedToolsMcpToolFilterParam `json:",omitzero,inline"`
}

func (u ToolMcpAllowedToolsUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfMcpAllowedTools, u.OfMcpToolFilter)
}

func (u *ToolMcpAllowedToolsUnionParam) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfMcpAllowedTools, &u.OfMcpToolFilter)
}

type ToolMcpParam struct {
	ServerLabel       string                           `json:"server_label"`
	Authorization     Opt[string]                      `json:"authorization,omitzero"`
	DeferLoading      Opt[bool]                        `json:"defer_loading,omitzero"`
	ServerDescription Opt[string]                      `json:"server_description,omitzero"`
	ServerURL         Opt[string]                      `json:"server_url,omitzero"`
	TunnelID          Opt[string]                      `json:"tunnel_id,omitzero"`
	AllowedCallers    []string                         `json:"allowed_callers,omitzero"`
	AllowedTools      ToolMcpAllowedToolsUnionParam    `json:"allowed_tools,omitzero"`
	Headers           map[string]string                `json:"headers,omitzero"`
	RequireApproval   ToolMcpRequireApprovalUnionParam `json:"require_approval,omitzero"`
	ConnectorID       string                           `json:"connector_id,omitzero"`
	Type              string                           `json:"type"`
	paramObj
}

func (p ToolMcpParam) MarshalJSON() ([]byte, error) {
	type raw ToolMcpParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "mcp"})
}

type ToolMcpRequireApprovalMcpToolApprovalFilterAlways struct {
	ReadOnly  bool     `json:"read_only"`
	ToolNames []string `json:"tool_names"`
}

type ToolMcpRequireApprovalMcpToolApprovalFilterAlwaysParam struct {
	ReadOnly  Opt[bool] `json:"read_only,omitzero"`
	ToolNames []string  `json:"tool_names,omitzero"`
	paramObj
}

func (p ToolMcpRequireApprovalMcpToolApprovalFilterAlwaysParam) MarshalJSON() ([]byte, error) {
	type raw ToolMcpRequireApprovalMcpToolApprovalFilterAlwaysParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ToolMcpRequireApprovalMcpToolApprovalFilterNever struct {
	ReadOnly  bool     `json:"read_only"`
	ToolNames []string `json:"tool_names"`
}

type ToolMcpRequireApprovalMcpToolApprovalFilterNeverParam struct {
	ReadOnly  Opt[bool] `json:"read_only,omitzero"`
	ToolNames []string  `json:"tool_names,omitzero"`
	paramObj
}

func (p ToolMcpRequireApprovalMcpToolApprovalFilterNeverParam) MarshalJSON() ([]byte, error) {
	type raw ToolMcpRequireApprovalMcpToolApprovalFilterNeverParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ToolMcpRequireApprovalMcpToolApprovalFilterParam struct {
	Always ToolMcpRequireApprovalMcpToolApprovalFilterAlwaysParam `json:"always,omitzero"`
	Never  ToolMcpRequireApprovalMcpToolApprovalFilterNeverParam  `json:"never,omitzero"`
	paramObj
}

func (p ToolMcpRequireApprovalMcpToolApprovalFilterParam) MarshalJSON() ([]byte, error) {
	type raw ToolMcpRequireApprovalMcpToolApprovalFilterParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ToolMcpRequireApprovalUnion struct {
	OfMcpToolApprovalSetting string                                            `json:",inline"`
	Always                   ToolMcpRequireApprovalMcpToolApprovalFilterAlways `json:"always"`
	Never                    ToolMcpRequireApprovalMcpToolApprovalFilterNever  `json:"never"`
}

func (u ToolMcpRequireApprovalUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfMcpToolApprovalSetting)
}

func (u *ToolMcpRequireApprovalUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfMcpToolApprovalSetting)
}

type ToolMcpRequireApprovalUnionParam struct {
	OfMcpToolApprovalFilter  *ToolMcpRequireApprovalMcpToolApprovalFilterParam `json:",omitzero,inline"`
	OfMcpToolApprovalSetting Opt[string]                                       `json:",omitzero,inline"`
}

func (u ToolMcpRequireApprovalUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfMcpToolApprovalFilter, u.OfMcpToolApprovalSetting)
}

func (u *ToolMcpRequireApprovalUnionParam) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfMcpToolApprovalFilter, &u.OfMcpToolApprovalSetting)
}

type ToolProgrammaticToolCallingParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ToolProgrammaticToolCallingParam) MarshalJSON() ([]byte, error) {
	type raw ToolProgrammaticToolCallingParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "programmatic_tool_calling"})
}

type ToolSearchToolExecution string

type ToolSearchToolParam struct {
	Description Opt[string]             `json:"description,omitzero"`
	Parameters  any                     `json:"parameters,omitzero"`
	Execution   ToolSearchToolExecution `json:"execution,omitzero"`
	Type        string                  `json:"type"`
	paramObj
}

func (p ToolSearchToolParam) MarshalJSON() ([]byte, error) {
	type raw ToolSearchToolParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "tool_search"})
}

type ToolUnion struct {
	Name               string                            `json:"name"`
	Parameters         any                               `json:"parameters"`
	Strict             bool                              `json:"strict"`
	Type               string                            `json:"type"`
	AllowedCallers     []string                          `json:"allowed_callers"`
	DeferLoading       bool                              `json:"defer_loading"`
	Description        string                            `json:"description"`
	OutputSchema       map[string]any                    `json:"output_schema"`
	VectorStoreIDs     []string                          `json:"vector_store_ids"`
	Filters            ToolUnionFilters                  `json:"filters"`
	MaxNumResults      int64                             `json:"max_num_results"`
	RankingOptions     FileSearchToolRankingOptions      `json:"ranking_options"`
	DisplayHeight      int64                             `json:"display_height"`
	DisplayWidth       int64                             `json:"display_width"`
	Environment        ToolUnionEnvironment              `json:"environment"`
	ExternalWebAccess  bool                              `json:"external_web_access"`
	SearchContextSize  string                            `json:"search_context_size"`
	UserLocation       ToolUnionUserLocation             `json:"user_location"`
	ServerLabel        string                            `json:"server_label"`
	AllowedTools       ToolMcpAllowedToolsUnion          `json:"allowed_tools"`
	Authorization      string                            `json:"authorization"`
	ConnectorID        string                            `json:"connector_id"`
	Headers            map[string]string                 `json:"headers"`
	RequireApproval    ToolMcpRequireApprovalUnion       `json:"require_approval"`
	ServerDescription  string                            `json:"server_description"`
	ServerURL          string                            `json:"server_url"`
	TunnelID           string                            `json:"tunnel_id"`
	Container          ToolCodeInterpreterContainerUnion `json:"container"`
	Action             string                            `json:"action"`
	Background         string                            `json:"background"`
	InputFidelity      string                            `json:"input_fidelity"`
	InputImageMask     ToolImageGenerationInputImageMask `json:"input_image_mask"`
	Model              string                            `json:"model"`
	Moderation         string                            `json:"moderation"`
	OutputCompression  int64                             `json:"output_compression"`
	OutputFormat       string                            `json:"output_format"`
	PartialImages      int64                             `json:"partial_images"`
	Quality            string                            `json:"quality"`
	Size               string                            `json:"size"`
	Format             CustomToolInputFormatUnion        `json:"format"`
	Tools              []NamespaceToolToolUnion          `json:"tools"`
	Execution          ToolSearchToolExecution           `json:"execution"`
	SearchContentTypes []string                          `json:"search_content_types"`
}

type ToolUnionEnvironment struct {
	OfComputerUsePreviewToolEnvironment ComputerUsePreviewToolEnvironment `json:",inline"`
	Type                                string                            `json:"type"`
	FileIDs                             []string                          `json:"file_ids"`
	MemoryLimit                         ContainerAutoMemoryLimit          `json:"memory_limit"`
	NetworkPolicy                       ContainerAutoNetworkPolicyUnion   `json:"network_policy"`
	Skills                              ToolUnionEnvironmentSkills        `json:"skills"`
	ContainerID                         string                            `json:"container_id"`
}

func (u ToolUnionEnvironment) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfComputerUsePreviewToolEnvironment)
}

func (u *ToolUnionEnvironment) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfComputerUsePreviewToolEnvironment)
}

type ToolUnionEnvironmentSkills struct {
	OfContainerAutoSkills []ContainerAutoSkillUnion `json:",inline"`
	OfLocalSkillArray     []LocalSkill              `json:",inline"`
}

func (u ToolUnionEnvironmentSkills) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfContainerAutoSkills, u.OfLocalSkillArray)
}

func (u *ToolUnionEnvironmentSkills) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfContainerAutoSkills, &u.OfLocalSkillArray)
}

type ToolUnionFilters struct {
	Key            string                      `json:"key"`
	Type           string                      `json:"type"`
	Value          ComparisonFilterValueUnion  `json:"value"`
	Filters        []CompoundFilterFilterUnion `json:"filters"`
	AllowedDomains []string                    `json:"allowed_domains"`
}

type ToolUnionParam struct {
	OfFunction                *FunctionToolParam                `json:",omitzero,inline"`
	OfFileSearch              *FileSearchToolParam              `json:",omitzero,inline"`
	OfComputer                *ComputerToolParam                `json:",omitzero,inline"`
	OfComputerUsePreview      *ComputerUsePreviewToolParam      `json:",omitzero,inline"`
	OfWebSearch               *WebSearchToolParam               `json:",omitzero,inline"`
	OfMcp                     *ToolMcpParam                     `json:",omitzero,inline"`
	OfCodeInterpreter         *ToolCodeInterpreterParam         `json:",omitzero,inline"`
	OfProgrammaticToolCalling *ToolProgrammaticToolCallingParam `json:",omitzero,inline"`
	OfImageGeneration         *ToolImageGenerationParam         `json:",omitzero,inline"`
	OfLocalShell              *ToolLocalShellParam              `json:",omitzero,inline"`
	OfShell                   *FunctionShellToolParam           `json:",omitzero,inline"`
	OfCustom                  *CustomToolParam                  `json:",omitzero,inline"`
	OfNamespace               *NamespaceToolParam               `json:",omitzero,inline"`
	OfToolSearch              *ToolSearchToolParam              `json:",omitzero,inline"`
	OfWebSearchPreview        *WebSearchPreviewToolParam        `json:",omitzero,inline"`
	OfApplyPatch              *ApplyPatchToolParam              `json:",omitzero,inline"`
}

func (u ToolUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfFunction, u.OfFileSearch, u.OfComputer, u.OfComputerUsePreview, u.OfWebSearch, u.OfMcp, u.OfCodeInterpreter, u.OfProgrammaticToolCalling, u.OfImageGeneration, u.OfLocalShell, u.OfShell, u.OfCustom, u.OfNamespace, u.OfToolSearch, u.OfWebSearchPreview, u.OfApplyPatch)
}

func (u *ToolUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "function":
		return json.Unmarshal(data, &u.OfFunction)
	case "file_search":
		return json.Unmarshal(data, &u.OfFileSearch)
	case "computer":
		return json.Unmarshal(data, &u.OfComputer)
	case "computer_use_preview":
		return json.Unmarshal(data, &u.OfComputerUsePreview)
	case "mcp":
		return json.Unmarshal(data, &u.OfMcp)
	case "code_interpreter":
		return json.Unmarshal(data, &u.OfCodeInterpreter)
	case "programmatic_tool_calling":
		return json.Unmarshal(data, &u.OfProgrammaticToolCalling)
	case "image_generation":
		return json.Unmarshal(data, &u.OfImageGeneration)
	case "local_shell":
		return json.Unmarshal(data, &u.OfLocalShell)
	case "shell":
		return json.Unmarshal(data, &u.OfShell)
	case "custom":
		return json.Unmarshal(data, &u.OfCustom)
	case "namespace":
		return json.Unmarshal(data, &u.OfNamespace)
	case "tool_search":
		return json.Unmarshal(data, &u.OfToolSearch)
	case "apply_patch":
		return json.Unmarshal(data, &u.OfApplyPatch)
	}
	return tryUnmarshal(data, &u.OfWebSearch, &u.OfWebSearchPreview)
}

type ToolUnionUserLocation struct {
	City     string `json:"city"`
	Country  string `json:"country"`
	Region   string `json:"region"`
	Timezone string `json:"timezone"`
	Type     string `json:"type"`
}

type WebSearchPreviewToolParam struct {
	Type               WebSearchPreviewToolType              `json:"type,omitzero"`
	UserLocation       WebSearchPreviewToolUserLocationParam `json:"user_location,omitzero"`
	SearchContentTypes []string                              `json:"search_content_types,omitzero"`
	SearchContextSize  WebSearchPreviewToolSearchContextSize `json:"search_context_size,omitzero"`
	paramObj
}

func (p WebSearchPreviewToolParam) MarshalJSON() ([]byte, error) {
	type raw WebSearchPreviewToolParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type WebSearchPreviewToolSearchContextSize string

type WebSearchPreviewToolType string

type WebSearchPreviewToolUserLocationParam struct {
	City     Opt[string] `json:"city,omitzero"`
	Country  Opt[string] `json:"country,omitzero"`
	Region   Opt[string] `json:"region,omitzero"`
	Timezone Opt[string] `json:"timezone,omitzero"`
	Type     string      `json:"type"`
	paramObj
}

func (p WebSearchPreviewToolUserLocationParam) MarshalJSON() ([]byte, error) {
	type raw WebSearchPreviewToolUserLocationParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "approximate"})
}

type WebSearchToolFiltersParam struct {
	AllowedDomains []string `json:"allowed_domains,omitzero"`
	paramObj
}

func (p WebSearchToolFiltersParam) MarshalJSON() ([]byte, error) {
	type raw WebSearchToolFiltersParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type WebSearchToolParam struct {
	Type              WebSearchToolType              `json:"type,omitzero"`
	ExternalWebAccess Opt[bool]                      `json:"external_web_access,omitzero"`
	Filters           WebSearchToolFiltersParam      `json:"filters,omitzero"`
	UserLocation      WebSearchToolUserLocationParam `json:"user_location,omitzero"`
	SearchContextSize WebSearchToolSearchContextSize `json:"search_context_size,omitzero"`
	paramObj
}

func (p WebSearchToolParam) MarshalJSON() ([]byte, error) {
	type raw WebSearchToolParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type WebSearchToolSearchContextSize string

type WebSearchToolType string

type WebSearchToolUserLocationParam struct {
	City     Opt[string] `json:"city,omitzero"`
	Country  Opt[string] `json:"country,omitzero"`
	Region   Opt[string] `json:"region,omitzero"`
	Timezone Opt[string] `json:"timezone,omitzero"`
	Type     string      `json:"type,omitzero"`
	paramObj
}

func (p WebSearchToolUserLocationParam) MarshalJSON() ([]byte, error) {
	type raw WebSearchToolUserLocationParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}
