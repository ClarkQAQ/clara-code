// Code generated from the openai-go v3.54.0 wire definitions. DO NOT EDIT.

package openai

import "encoding/json"

type ResponseAudioDeltaEvent struct {
	Delta          string `json:"delta"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseAudioDoneEvent struct {
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseAudioTranscriptDeltaEvent struct {
	Delta          string `json:"delta"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseAudioTranscriptDoneEvent struct {
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseCodeInterpreterCallCodeDeltaEvent struct {
	Delta          string `json:"delta"`
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseCodeInterpreterCallCodeDoneEvent struct {
	Code           string `json:"code"`
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseCodeInterpreterCallCompletedEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseCodeInterpreterCallInProgressEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseCodeInterpreterCallInterpretingEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseCompletedEvent struct {
	Response       Response `json:"response"`
	SequenceNumber int64    `json:"sequence_number"`
	Type           string   `json:"type"`
}

type ResponseContentPartAddedEvent struct {
	ContentIndex   int64                                  `json:"content_index"`
	ItemID         string                                 `json:"item_id"`
	OutputIndex    int64                                  `json:"output_index"`
	Part           ResponseContentPartAddedEventPartUnion `json:"part"`
	SequenceNumber int64                                  `json:"sequence_number"`
	Type           string                                 `json:"type"`
}

type ResponseContentPartDoneEvent struct {
	ContentIndex   int64                                 `json:"content_index"`
	ItemID         string                                `json:"item_id"`
	OutputIndex    int64                                 `json:"output_index"`
	Part           ResponseContentPartDoneEventPartUnion `json:"part"`
	SequenceNumber int64                                 `json:"sequence_number"`
	Type           string                                `json:"type"`
}

type ResponseCreatedEvent struct {
	Response       Response `json:"response"`
	SequenceNumber int64    `json:"sequence_number"`
	Type           string   `json:"type"`
}

type ResponseCustomToolCallInputDeltaEvent struct {
	Delta          string `json:"delta"`
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseCustomToolCallInputDoneEvent struct {
	Input          string `json:"input"`
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseErrorEvent struct {
	Code           string `json:"code"`
	Message        string `json:"message"`
	Param          string `json:"param"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseFailedEvent struct {
	Response       Response `json:"response"`
	SequenceNumber int64    `json:"sequence_number"`
	Type           string   `json:"type"`
}

type ResponseFileSearchCallCompletedEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseFileSearchCallInProgressEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseFileSearchCallSearchingEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseFunctionCallArgumentsDeltaEvent struct {
	Delta          string `json:"delta"`
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseFunctionCallArgumentsDoneEvent struct {
	Arguments      string `json:"arguments"`
	ItemID         string `json:"item_id"`
	Name           string `json:"name"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseImageGenCallCompletedEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseImageGenCallGeneratingEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseImageGenCallInProgressEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseImageGenCallPartialImageEvent struct {
	ItemID            string `json:"item_id"`
	OutputIndex       int64  `json:"output_index"`
	PartialImageB64   string `json:"partial_image_b64"`
	PartialImageIndex int64  `json:"partial_image_index"`
	SequenceNumber    int64  `json:"sequence_number"`
	Type              string `json:"type"`
	Background        string `json:"background"`
	OutputFormat      string `json:"output_format"`
	Quality           string `json:"quality"`
	Size              string `json:"size"`
}

type ResponseInProgressEvent struct {
	Response       Response `json:"response"`
	SequenceNumber int64    `json:"sequence_number"`
	Type           string   `json:"type"`
}

type ResponseIncompleteEvent struct {
	Response       Response `json:"response"`
	SequenceNumber int64    `json:"sequence_number"`
	Type           string   `json:"type"`
}

type ResponseMcpCallArgumentsDeltaEvent struct {
	Delta          string `json:"delta"`
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseMcpCallArgumentsDoneEvent struct {
	Arguments      string `json:"arguments"`
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseMcpCallCompletedEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseMcpCallFailedEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseMcpCallInProgressEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseMcpListToolsCompletedEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseMcpListToolsFailedEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseMcpListToolsInProgressEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseOutputItemAddedEvent struct {
	Item           ResponseOutputItemUnion `json:"item"`
	OutputIndex    int64                   `json:"output_index"`
	SequenceNumber int64                   `json:"sequence_number"`
	Type           string                  `json:"type"`
}

type ResponseOutputItemDoneEvent struct {
	Item           ResponseOutputItemUnion `json:"item"`
	OutputIndex    int64                   `json:"output_index"`
	SequenceNumber int64                   `json:"sequence_number"`
	Type           string                  `json:"type"`
}

type ResponseOutputTextAnnotationAddedEvent struct {
	Annotation      ResponseOutputTextAnnotationAddedEventAnnotationUnion `json:"annotation"`
	AnnotationIndex int64                                                 `json:"annotation_index"`
	ContentIndex    int64                                                 `json:"content_index"`
	ItemID          string                                                `json:"item_id"`
	OutputIndex     int64                                                 `json:"output_index"`
	SequenceNumber  int64                                                 `json:"sequence_number"`
	Type            string                                                `json:"type"`
}

type ResponseQueuedEvent struct {
	Response       Response `json:"response"`
	SequenceNumber int64    `json:"sequence_number"`
	Type           string   `json:"type"`
}

type ResponseReasoningSummaryPartAddedEvent struct {
	ItemID         string                                     `json:"item_id"`
	OutputIndex    int64                                      `json:"output_index"`
	Part           ResponseReasoningSummaryPartAddedEventPart `json:"part"`
	SequenceNumber int64                                      `json:"sequence_number"`
	SummaryIndex   int64                                      `json:"summary_index"`
	Type           string                                     `json:"type"`
}

type ResponseReasoningSummaryPartDoneEvent struct {
	ItemID         string                                      `json:"item_id"`
	OutputIndex    int64                                       `json:"output_index"`
	Part           ResponseReasoningSummaryPartDoneEventPart   `json:"part"`
	SequenceNumber int64                                       `json:"sequence_number"`
	SummaryIndex   int64                                       `json:"summary_index"`
	Type           string                                      `json:"type"`
	Status         ResponseReasoningSummaryPartDoneEventStatus `json:"status"`
}

type ResponseReasoningSummaryTextDeltaEvent struct {
	Delta          string `json:"delta"`
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	SummaryIndex   int64  `json:"summary_index"`
	Type           string `json:"type"`
}

type ResponseReasoningSummaryTextDoneEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	SummaryIndex   int64  `json:"summary_index"`
	Text           string `json:"text"`
	Type           string `json:"type"`
}

type ResponseReasoningTextDeltaEvent struct {
	ContentIndex   int64  `json:"content_index"`
	Delta          string `json:"delta"`
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseReasoningTextDoneEvent struct {
	ContentIndex   int64  `json:"content_index"`
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Text           string `json:"text"`
	Type           string `json:"type"`
}

type ResponseRefusalDeltaEvent struct {
	ContentIndex   int64  `json:"content_index"`
	Delta          string `json:"delta"`
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseRefusalDoneEvent struct {
	ContentIndex   int64  `json:"content_index"`
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	Refusal        string `json:"refusal"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseShellCallCommandAddedEvent struct {
	Command        string `json:"command"`
	CommandIndex   int64  `json:"command_index"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseShellCallCommandDeltaEvent struct {
	CommandIndex   int64  `json:"command_index"`
	Delta          string `json:"delta"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
	Obfuscation    string `json:"obfuscation"`
}

type ResponseShellCallCommandDoneEvent struct {
	Command        string `json:"command"`
	CommandIndex   int64  `json:"command_index"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseShellCallOutputContentDeltaEvent struct {
	CommandIndex   int64                                         `json:"command_index"`
	Delta          ResponseShellCallOutputContentDeltaEventDelta `json:"delta"`
	ItemID         string                                        `json:"item_id"`
	OutputIndex    int64                                         `json:"output_index"`
	SequenceNumber int64                                         `json:"sequence_number"`
	Type           string                                        `json:"type"`
}

type ResponseShellCallOutputContentDoneEvent struct {
	CommandIndex   int64                                           `json:"command_index"`
	ItemID         string                                          `json:"item_id"`
	Output         []ResponseShellCallOutputContentDoneEventOutput `json:"output"`
	OutputIndex    int64                                           `json:"output_index"`
	SequenceNumber int64                                           `json:"sequence_number"`
	Type           string                                          `json:"type"`
}

type ResponseTextDeltaEvent struct {
	ContentIndex   int64                           `json:"content_index"`
	Delta          string                          `json:"delta"`
	ItemID         string                          `json:"item_id"`
	Logprobs       []ResponseTextDeltaEventLogprob `json:"logprobs"`
	OutputIndex    int64                           `json:"output_index"`
	SequenceNumber int64                           `json:"sequence_number"`
	Type           string                          `json:"type"`
}

type ResponseTextDoneEvent struct {
	ContentIndex   int64                          `json:"content_index"`
	ItemID         string                         `json:"item_id"`
	Logprobs       []ResponseTextDoneEventLogprob `json:"logprobs"`
	OutputIndex    int64                          `json:"output_index"`
	SequenceNumber int64                          `json:"sequence_number"`
	Text           string                         `json:"text"`
	Type           string                         `json:"type"`
}

type ResponseWebSearchCallCompletedEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseWebSearchCallInProgressEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

type ResponseWebSearchCallSearchingEvent struct {
	ItemID         string `json:"item_id"`
	OutputIndex    int64  `json:"output_index"`
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
}

// StreamEvent is a single server-sent event of the Responses API.
// Type is the event name, Raw is the untouched JSON payload.
type StreamEvent struct {
	Type           string          `json:"type"`
	SequenceNumber int64           `json:"sequence_number"`
	Raw            json.RawMessage `json:"-"`
}

// UnmarshalJSON keeps the raw payload so typed accessors can decode it on demand.
func (e *StreamEvent) UnmarshalJSON(data []byte) error {
	type raw StreamEvent
	var v raw
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*e = StreamEvent(v)
	e.Raw = append(json.RawMessage(nil), data...)
	return nil
}

// RawJSON returns the untouched event payload.
func (e StreamEvent) RawJSON() string { return string(e.Raw) }

func (e StreamEvent) AsError() ResponseErrorEvent {
	var v ResponseErrorEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseAudioDelta() ResponseAudioDeltaEvent {
	var v ResponseAudioDeltaEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseAudioDone() ResponseAudioDoneEvent {
	var v ResponseAudioDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseAudioTranscriptDelta() ResponseAudioTranscriptDeltaEvent {
	var v ResponseAudioTranscriptDeltaEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseAudioTranscriptDone() ResponseAudioTranscriptDoneEvent {
	var v ResponseAudioTranscriptDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseCodeInterpreterCallCodeDelta() ResponseCodeInterpreterCallCodeDeltaEvent {
	var v ResponseCodeInterpreterCallCodeDeltaEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseCodeInterpreterCallCodeDone() ResponseCodeInterpreterCallCodeDoneEvent {
	var v ResponseCodeInterpreterCallCodeDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseCodeInterpreterCallCompleted() ResponseCodeInterpreterCallCompletedEvent {
	var v ResponseCodeInterpreterCallCompletedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseCodeInterpreterCallInProgress() ResponseCodeInterpreterCallInProgressEvent {
	var v ResponseCodeInterpreterCallInProgressEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseCodeInterpreterCallInterpreting() ResponseCodeInterpreterCallInterpretingEvent {
	var v ResponseCodeInterpreterCallInterpretingEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseCompleted() ResponseCompletedEvent {
	var v ResponseCompletedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseContentPartAdded() ResponseContentPartAddedEvent {
	var v ResponseContentPartAddedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseContentPartDone() ResponseContentPartDoneEvent {
	var v ResponseContentPartDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseCreated() ResponseCreatedEvent {
	var v ResponseCreatedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseCustomToolCallInputDelta() ResponseCustomToolCallInputDeltaEvent {
	var v ResponseCustomToolCallInputDeltaEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseCustomToolCallInputDone() ResponseCustomToolCallInputDoneEvent {
	var v ResponseCustomToolCallInputDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseFailed() ResponseFailedEvent {
	var v ResponseFailedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseFileSearchCallCompleted() ResponseFileSearchCallCompletedEvent {
	var v ResponseFileSearchCallCompletedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseFileSearchCallInProgress() ResponseFileSearchCallInProgressEvent {
	var v ResponseFileSearchCallInProgressEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseFileSearchCallSearching() ResponseFileSearchCallSearchingEvent {
	var v ResponseFileSearchCallSearchingEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseFunctionCallArgumentsDelta() ResponseFunctionCallArgumentsDeltaEvent {
	var v ResponseFunctionCallArgumentsDeltaEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseFunctionCallArgumentsDone() ResponseFunctionCallArgumentsDoneEvent {
	var v ResponseFunctionCallArgumentsDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseImageGenerationCallCompleted() ResponseImageGenCallCompletedEvent {
	var v ResponseImageGenCallCompletedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseImageGenerationCallGenerating() ResponseImageGenCallGeneratingEvent {
	var v ResponseImageGenCallGeneratingEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseImageGenerationCallInProgress() ResponseImageGenCallInProgressEvent {
	var v ResponseImageGenCallInProgressEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseImageGenerationCallPartialImage() ResponseImageGenCallPartialImageEvent {
	var v ResponseImageGenCallPartialImageEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseInProgress() ResponseInProgressEvent {
	var v ResponseInProgressEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseIncomplete() ResponseIncompleteEvent {
	var v ResponseIncompleteEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseMcpCallArgumentsDelta() ResponseMcpCallArgumentsDeltaEvent {
	var v ResponseMcpCallArgumentsDeltaEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseMcpCallArgumentsDone() ResponseMcpCallArgumentsDoneEvent {
	var v ResponseMcpCallArgumentsDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseMcpCallCompleted() ResponseMcpCallCompletedEvent {
	var v ResponseMcpCallCompletedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseMcpCallFailed() ResponseMcpCallFailedEvent {
	var v ResponseMcpCallFailedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseMcpCallInProgress() ResponseMcpCallInProgressEvent {
	var v ResponseMcpCallInProgressEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseMcpListToolsCompleted() ResponseMcpListToolsCompletedEvent {
	var v ResponseMcpListToolsCompletedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseMcpListToolsFailed() ResponseMcpListToolsFailedEvent {
	var v ResponseMcpListToolsFailedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseMcpListToolsInProgress() ResponseMcpListToolsInProgressEvent {
	var v ResponseMcpListToolsInProgressEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseOutputItemAdded() ResponseOutputItemAddedEvent {
	var v ResponseOutputItemAddedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseOutputItemDone() ResponseOutputItemDoneEvent {
	var v ResponseOutputItemDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseOutputTextAnnotationAdded() ResponseOutputTextAnnotationAddedEvent {
	var v ResponseOutputTextAnnotationAddedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseOutputTextDelta() ResponseTextDeltaEvent {
	var v ResponseTextDeltaEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseOutputTextDone() ResponseTextDoneEvent {
	var v ResponseTextDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseQueued() ResponseQueuedEvent {
	var v ResponseQueuedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseReasoningSummaryPartAdded() ResponseReasoningSummaryPartAddedEvent {
	var v ResponseReasoningSummaryPartAddedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseReasoningSummaryPartDone() ResponseReasoningSummaryPartDoneEvent {
	var v ResponseReasoningSummaryPartDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseReasoningSummaryTextDelta() ResponseReasoningSummaryTextDeltaEvent {
	var v ResponseReasoningSummaryTextDeltaEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseReasoningSummaryTextDone() ResponseReasoningSummaryTextDoneEvent {
	var v ResponseReasoningSummaryTextDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseReasoningTextDelta() ResponseReasoningTextDeltaEvent {
	var v ResponseReasoningTextDeltaEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseReasoningTextDone() ResponseReasoningTextDoneEvent {
	var v ResponseReasoningTextDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseRefusalDelta() ResponseRefusalDeltaEvent {
	var v ResponseRefusalDeltaEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseRefusalDone() ResponseRefusalDoneEvent {
	var v ResponseRefusalDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseShellCallCommandAdded() ResponseShellCallCommandAddedEvent {
	var v ResponseShellCallCommandAddedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseShellCallCommandDelta() ResponseShellCallCommandDeltaEvent {
	var v ResponseShellCallCommandDeltaEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseShellCallCommandDone() ResponseShellCallCommandDoneEvent {
	var v ResponseShellCallCommandDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseShellCallOutputContentDelta() ResponseShellCallOutputContentDeltaEvent {
	var v ResponseShellCallOutputContentDeltaEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseShellCallOutputContentDone() ResponseShellCallOutputContentDoneEvent {
	var v ResponseShellCallOutputContentDoneEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseWebSearchCallCompleted() ResponseWebSearchCallCompletedEvent {
	var v ResponseWebSearchCallCompletedEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseWebSearchCallInProgress() ResponseWebSearchCallInProgressEvent {
	var v ResponseWebSearchCallInProgressEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}

func (e StreamEvent) AsResponseWebSearchCallSearching() ResponseWebSearchCallSearchingEvent {
	var v ResponseWebSearchCallSearchingEvent
	_ = json.Unmarshal(e.Raw, &v)
	return v
}
