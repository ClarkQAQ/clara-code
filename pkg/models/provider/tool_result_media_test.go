package provider

import (
	yuzu "clara-code/pkg/models"
	openai "clara-code/pkg/models/internal/wire"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

// Tool messages in the OpenAI Chat Completions API cannot carry image or audio
// content directly — the SDK's content union only accepts text. When a tool
// returns media, DefaultToPrompt must still emit a text tool message so the
// tool_call/tool_result pairing stays valid, and attach the media to a
// synthetic follow-up user message so vision- and audio-capable models can see
// it.

func TestDefaultToPrompt_MediaToolResult_ImagePNG(t *testing.T) {
	t.Parallel()

	imageData := base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 3})
	prompt := yuzu.Prompt{
		{
			Role: yuzu.MessageRoleAssistant,
			Content: []yuzu.MessagePart{
				yuzu.ToolCallPart{ToolCallID: "img-1", ToolName: "view", Input: "{}"},
			},
		},
		{
			Role: yuzu.MessageRoleTool,
			Content: []yuzu.MessagePart{
				yuzu.ToolResultPart{
					ToolCallID: "img-1",
					Output: yuzu.ToolResultOutputContentMedia{
						Data:      imageData,
						MediaType: "image/png",
					},
				},
			},
		},
	}

	messages, warnings := DefaultToPrompt(prompt, Name, "gpt-5")

	require.Empty(t, warnings)
	// Assistant tool call + text tool message + synthetic user image message.
	require.Len(t, messages, 3)

	toolMsg := messages[1].OfTool
	require.NotNil(t, toolMsg)
	require.Equal(t, "img-1", toolMsg.ToolCallID)
	require.Contains(t, toolMsg.Content.OfString.Value, "image/png")

	userMsg := messages[2].OfUser
	require.NotNil(t, userMsg)
	require.Len(t, userMsg.Content.OfArrayOfContentParts, 1)
	imagePart := userMsg.Content.OfArrayOfContentParts[0].OfImageURL
	require.NotNil(t, imagePart)
	require.Equal(t, "data:image/png;base64,"+imageData, imagePart.ImageURL.URL)
}

func TestDefaultToPrompt_MediaToolResult_PrefersAccompanyingText(t *testing.T) {
	t.Parallel()

	imageData := base64.StdEncoding.EncodeToString([]byte{9, 9, 9})
	prompt := yuzu.Prompt{
		{
			Role: yuzu.MessageRoleAssistant,
			Content: []yuzu.MessagePart{
				yuzu.ToolCallPart{ToolCallID: "img-2", ToolName: "view", Input: "{}"},
			},
		},
		{
			Role: yuzu.MessageRoleTool,
			Content: []yuzu.MessagePart{
				yuzu.ToolResultPart{
					ToolCallID: "img-2",
					Output: yuzu.ToolResultOutputContentMedia{
						Data:      imageData,
						MediaType: "image/jpeg",
						Text:      "Screenshot of the blockquote element.",
					},
				},
			},
		},
	}

	messages, warnings := DefaultToPrompt(prompt, Name, "gpt-5")

	require.Empty(t, warnings)
	require.Len(t, messages, 3)
	require.Equal(t, "Screenshot of the blockquote element.", messages[1].OfTool.Content.OfString.Value)
}

// Tool messages must stay contiguous right after the assistant turn that
// requested them; the synthetic media messages follow the whole group.
func TestDefaultToPrompt_MediaToolResults_KeepToolMessagesContiguous(t *testing.T) {
	t.Parallel()

	imageData := base64.StdEncoding.EncodeToString([]byte{11, 11})
	mediaResult := func(callID string) yuzu.MessagePart {
		return yuzu.ToolResultPart{
			ToolCallID: callID,
			Output: yuzu.ToolResultOutputContentMedia{
				Data:      imageData,
				MediaType: "image/png",
			},
		}
	}
	prompt := yuzu.Prompt{
		{
			Role: yuzu.MessageRoleAssistant,
			Content: []yuzu.MessagePart{
				yuzu.ToolCallPart{ToolCallID: "img-a", ToolName: "view", Input: "{}"},
				yuzu.ToolCallPart{ToolCallID: "img-b", ToolName: "view", Input: "{}"},
			},
		},
		{
			Role:    yuzu.MessageRoleTool,
			Content: []yuzu.MessagePart{mediaResult("img-a"), mediaResult("img-b")},
		},
	}

	messages, warnings := DefaultToPrompt(prompt, Name, "gpt-5")

	require.Empty(t, warnings)
	require.Len(t, messages, 5)
	require.Equal(t, "img-a", messages[1].OfTool.ToolCallID)
	require.Equal(t, "img-b", messages[2].OfTool.ToolCallID)
	require.NotNil(t, messages[3].OfUser)
	require.NotNil(t, messages[4].OfUser)
}

func TestDefaultToPrompt_MediaToolResult_AudioWAV(t *testing.T) {
	t.Parallel()

	audio := base64.StdEncoding.EncodeToString([]byte("fake-wav-bytes"))
	prompt := yuzu.Prompt{
		{
			Role: yuzu.MessageRoleAssistant,
			Content: []yuzu.MessagePart{
				yuzu.ToolCallPart{ToolCallID: "audio-1", ToolName: "record", Input: "{}"},
			},
		},
		{
			Role: yuzu.MessageRoleTool,
			Content: []yuzu.MessagePart{
				yuzu.ToolResultPart{
					ToolCallID: "audio-1",
					Output: yuzu.ToolResultOutputContentMedia{
						Data:      audio,
						MediaType: "audio/wav",
					},
				},
			},
		},
	}

	messages, warnings := DefaultToPrompt(prompt, "openai", "gpt-4o-audio")

	require.Empty(t, warnings)
	require.Len(t, messages, 3)
	require.NotNil(t, messages[1].OfTool)
	userMsg := messages[2].OfUser
	require.NotNil(t, userMsg)
	require.Len(t, userMsg.Content.OfArrayOfContentParts, 1)
	audioPart := userMsg.Content.OfArrayOfContentParts[0].OfInputAudio
	require.NotNil(t, audioPart)
	require.Equal(t, audio, audioPart.InputAudio.Data)
	require.Equal(t, "wav", audioPart.InputAudio.Format)
}

func TestDefaultToPrompt_MediaToolResult_UnsupportedMediaType(t *testing.T) {
	t.Parallel()

	prompt := yuzu.Prompt{
		{
			Role: yuzu.MessageRoleAssistant,
			Content: []yuzu.MessagePart{
				yuzu.ToolCallPart{ToolCallID: "vid-1", ToolName: "record", Input: "{}"},
			},
		},
		{
			Role: yuzu.MessageRoleTool,
			Content: []yuzu.MessagePart{
				yuzu.ToolResultPart{
					ToolCallID: "vid-1",
					Output: yuzu.ToolResultOutputContentMedia{
						Data:      "AAAA",
						MediaType: "video/mp4",
					},
				},
			},
		},
	}

	messages, warnings := DefaultToPrompt(prompt, "openai", "gpt-5")

	// Assistant tool call + text tool message, but no synthetic user image.
	require.Len(t, messages, 2)
	require.NotNil(t, messages[1].OfTool)
	require.Equal(t, "vid-1", messages[1].OfTool.ToolCallID)
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0].Message, "video/mp4")
}

func TestToResponsesPrompt_MediaToolResult_ImagePNG(t *testing.T) {
	t.Parallel()

	imageData := base64.StdEncoding.EncodeToString([]byte{7, 7, 7, 7})
	prompt := yuzu.Prompt{
		{
			Role: yuzu.MessageRoleAssistant,
			Content: []yuzu.MessagePart{
				yuzu.ToolCallPart{ToolCallID: "img-resp-1", ToolName: "view", Input: "{}"},
			},
		},
		{
			Role: yuzu.MessageRoleTool,
			Content: []yuzu.MessagePart{
				yuzu.ToolResultPart{
					ToolCallID: "img-resp-1",
					Output: yuzu.ToolResultOutputContentMedia{
						Data:      imageData,
						MediaType: "image/png",
					},
				},
			},
		},
	}

	input, warnings := toResponsesPrompt(prompt, responsesPromptOptions{systemMessageMode: "system", store: false})

	require.Empty(t, warnings)
	// Assistant function call + function_call_output carrying the image.
	require.Len(t, input, 2)

	funcOut := input[1].OfFunctionCallOutput
	require.NotNil(t, funcOut)
	require.Equal(t, "img-resp-1", funcOut.CallID.Value)

	parts := funcOut.Output.OfResponseFunctionCallOutputItemArray
	require.Len(t, parts, 1)
	imagePart := parts[0].OfInputImage
	require.NotNil(t, imagePart)
	require.Equal(t, "data:image/png;base64,"+imageData, imagePart.ImageURL.Value)
}

func TestToResponsesPrompt_MediaToolResult_PrefersAccompanyingText(t *testing.T) {
	t.Parallel()

	imageData := base64.StdEncoding.EncodeToString([]byte{3, 3, 3})
	prompt := yuzu.Prompt{
		{
			Role: yuzu.MessageRoleAssistant,
			Content: []yuzu.MessagePart{
				yuzu.ToolCallPart{ToolCallID: "img-resp-2", ToolName: "view", Input: "{}"},
			},
		},
		{
			Role: yuzu.MessageRoleTool,
			Content: []yuzu.MessagePart{
				yuzu.ToolResultPart{
					ToolCallID: "img-resp-2",
					Output: yuzu.ToolResultOutputContentMedia{
						Data:      imageData,
						MediaType: "image/jpeg",
						Text:      "Screenshot of the blockquote element.",
					},
				},
			},
		},
	}

	input, warnings := toResponsesPrompt(prompt, responsesPromptOptions{systemMessageMode: "system", store: false})

	require.Empty(t, warnings)
	require.Len(t, input, 2)

	parts := input[1].OfFunctionCallOutput.Output.OfResponseFunctionCallOutputItemArray
	require.Len(t, parts, 2)
	require.Equal(t, "Screenshot of the blockquote element.", parts[0].OfInputText.Text)
	require.NotNil(t, parts[1].OfInputImage)
}

// A step that returns more than one image must answer every call inside the
// same group of tool items: strict Responses API implementations (DeepSeek)
// answer "No tool output found for tool call ..." as soon as a message item
// separates a call from its output.
func TestToResponsesPrompt_MediaToolResults_NoMessageBetweenCallsAndOutputs(t *testing.T) {
	t.Parallel()

	imageData := base64.StdEncoding.EncodeToString([]byte{5, 5, 5})
	mediaResult := func(callID string) yuzu.MessagePart {
		return yuzu.ToolResultPart{
			ToolCallID: callID,
			Output: yuzu.ToolResultOutputContentMedia{
				Data:      imageData,
				MediaType: "image/jpeg",
			},
		}
	}
	prompt := yuzu.Prompt{
		{
			Role: yuzu.MessageRoleAssistant,
			Content: []yuzu.MessagePart{
				yuzu.ToolCallPart{ToolCallID: "img-a", ToolName: "view", Input: "{}"},
				yuzu.ToolCallPart{ToolCallID: "img-b", ToolName: "view", Input: "{}"},
			},
		},
		{
			Role:    yuzu.MessageRoleTool,
			Content: []yuzu.MessagePart{mediaResult("img-a"), mediaResult("img-b")},
		},
	}

	input, warnings := toResponsesPrompt(prompt, responsesPromptOptions{systemMessageMode: "system", store: false})

	require.Empty(t, warnings)
	require.Len(t, input, 4)
	require.Equal(t, "img-a", input[0].OfFunctionCall.CallID)
	require.Equal(t, "img-b", input[1].OfFunctionCall.CallID)
	require.Equal(t, "img-a", input[2].OfFunctionCallOutput.CallID.Value)
	require.Equal(t, "img-b", input[3].OfFunctionCallOutput.CallID.Value)
	require.Equal(t, []string{"call=img-a", "call=img-b", "output=img-a", "output=img-b"}, responseItemSignatures(input))
}

func TestToResponsesPrompt_MediaToolResult_UnsupportedMediaType(t *testing.T) {
	t.Parallel()

	prompt := yuzu.Prompt{
		{
			Role: yuzu.MessageRoleAssistant,
			Content: []yuzu.MessagePart{
				yuzu.ToolCallPart{ToolCallID: "vid-resp-1", ToolName: "record", Input: "{}"},
			},
		},
		{
			Role: yuzu.MessageRoleTool,
			Content: []yuzu.MessagePart{
				yuzu.ToolResultPart{
					ToolCallID: "vid-resp-1",
					Output: yuzu.ToolResultOutputContentMedia{
						Data:      "AAAA",
						MediaType: "video/mp4",
					},
				},
			},
		},
	}

	input, warnings := toResponsesPrompt(prompt, responsesPromptOptions{systemMessageMode: "system", store: false})

	// Assistant function call + function_call_output holding a text
	// placeholder.
	require.Len(t, input, 2)
	funcOut := input[1].OfFunctionCallOutput
	require.NotNil(t, funcOut)
	require.Equal(t, "vid-resp-1", funcOut.CallID.Value)
	parts := funcOut.Output.OfResponseFunctionCallOutputItemArray
	require.Len(t, parts, 1)
	require.Contains(t, parts[0].OfInputText.Text, "video/mp4")
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0].Message, "video/mp4")
}

func responseItemSignatures(items openai.ResponseInputParam) []string {
	signatures := make([]string, 0, len(items))
	for _, item := range items {
		switch {
		case item.OfFunctionCall != nil:
			signatures = append(signatures, "call="+item.OfFunctionCall.CallID)
		case item.OfFunctionCallOutput != nil:
			signatures = append(signatures, "output="+item.OfFunctionCallOutput.CallID.Value)
		case item.OfMessage != nil:
			signatures = append(signatures, "message")
		case item.OfReasoning != nil:
			signatures = append(signatures, "reasoning")
		case item.OfItemReference != nil:
			signatures = append(signatures, "reference="+item.OfItemReference.ID)
		default:
			signatures = append(signatures, "other")
		}
	}
	return signatures
}
