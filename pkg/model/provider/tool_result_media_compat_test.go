package provider

import (
	"encoding/base64"
	"testing"

	yuzu "clara-code/pkg/model"

	"github.com/stretchr/testify/require"
)

func TestToPromptFunc_MediaToolResult_ImagePNG(t *testing.T) {
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

	messages, warnings := DefaultToPrompt(prompt, "", "")

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

func TestToPromptFunc_MediaToolResult_PrefersAccompanyingText(t *testing.T) {
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

	messages, warnings := DefaultToPrompt(prompt, "", "")

	require.Empty(t, warnings)
	require.Len(t, messages, 3)
	require.Equal(t, "Screenshot of the blockquote element.", messages[1].OfTool.Content.OfString.Value)
}

func TestToPromptFunc_MediaToolResult_AudioWAV(t *testing.T) {
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

	messages, warnings := DefaultToPrompt(prompt, "", "")

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

func TestToPromptFunc_MediaToolResult_AudioMP3(t *testing.T) {
	t.Parallel()

	audio := base64.StdEncoding.EncodeToString([]byte("fake-mp3-bytes"))
	prompt := yuzu.Prompt{
		{
			Role: yuzu.MessageRoleAssistant,
			Content: []yuzu.MessagePart{
				yuzu.ToolCallPart{ToolCallID: "audio-2", ToolName: "record", Input: "{}"},
			},
		},
		{
			Role: yuzu.MessageRoleTool,
			Content: []yuzu.MessagePart{
				yuzu.ToolResultPart{
					ToolCallID: "audio-2",
					Output: yuzu.ToolResultOutputContentMedia{
						Data:      audio,
						MediaType: "audio/mpeg",
					},
				},
			},
		},
	}

	messages, warnings := DefaultToPrompt(prompt, "", "")

	require.Empty(t, warnings)
	require.Len(t, messages, 3)
	require.NotNil(t, messages[1].OfTool)
	userMsg := messages[2].OfUser
	require.NotNil(t, userMsg)
	require.Len(t, userMsg.Content.OfArrayOfContentParts, 1)
	audioPart := userMsg.Content.OfArrayOfContentParts[0].OfInputAudio
	require.NotNil(t, audioPart)
	require.Equal(t, audio, audioPart.InputAudio.Data)
	require.Equal(t, "mp3", audioPart.InputAudio.Format)
}

func TestToPromptFunc_MediaToolResult_UnsupportedMediaType(t *testing.T) {
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

	messages, warnings := DefaultToPrompt(prompt, "", "")

	// Assistant tool call + text tool message, but no synthetic user image.
	require.Len(t, messages, 2)
	require.NotNil(t, messages[1].OfTool)
	require.Equal(t, "vid-1", messages[1].OfTool.ToolCallID)
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0].Message, "video/mp4")
}
