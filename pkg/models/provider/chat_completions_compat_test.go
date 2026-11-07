package provider

import (
	yuzu "clara-code/pkg/models"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultToPrompt_ReasoningContent(t *testing.T) {
	t.Parallel()

	t.Run("should add reasoning_content field to assistant messages", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "What is 2+2?"},
				},
			},
			{
				Role: yuzu.MessageRoleAssistant,
				Content: []yuzu.MessagePart{
					yuzu.ReasoningPart{Text: "Let me think... 2+2 equals 4."},
					yuzu.TextPart{Text: "The answer is 4."},
				},
			},
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "What about 3+3?"},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 3)

		// First message (user) - no reasoning
		msg1 := messages[0].OfUser
		require.NotNil(t, msg1)
		require.Equal(t, "What is 2+2?", msg1.Content.OfString.Value)

		// Second message (assistant) - with reasoning
		msg2 := messages[1].OfAssistant
		require.NotNil(t, msg2)
		require.Equal(t, "The answer is 4.", msg2.Content.OfString.Value)
		// Check reasoning_content in extra fields
		extraFields := msg2.ExtraFields()
		reasoningContent, hasReasoning := extraFields["reasoning_content"]
		require.True(t, hasReasoning)
		require.Equal(t, "Let me think... 2+2 equals 4.", reasoningContent)

		// Third message (user) - no reasoning
		msg3 := messages[2].OfUser
		require.NotNil(t, msg3)
		require.Equal(t, "What about 3+3?", msg3.Content.OfString.Value)
	})

	t.Run("should handle assistant messages with only reasoning content", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Hello"},
				},
			},
			{
				Role: yuzu.MessageRoleAssistant,
				Content: []yuzu.MessagePart{
					yuzu.ReasoningPart{Text: "Internal reasoning only..."},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		// Reasoning-only turns are visible content: reasoning_content must
		// round-trip for DeepSeek-family replay (CHARM-2020).
		require.Empty(t, warnings)
		require.Len(t, messages, 2)

		msg := messages[0].OfUser
		require.NotNil(t, msg)
		require.Equal(t, "Hello", msg.Content.OfString.Value)

		assistantMsg := messages[1].OfAssistant
		require.NotNil(t, assistantMsg)
		require.Equal(t, "Internal reasoning only...", assistantMsg.ExtraFields()["reasoning_content"])
	})

	t.Run("should not add reasoning_content to messages without reasoning", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Hello"},
				},
			},
			{
				Role: yuzu.MessageRoleAssistant,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Hi there!"},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 2)

		// Assistant message without reasoning
		msg := messages[1].OfAssistant
		require.NotNil(t, msg)
		require.Equal(t, "Hi there!", msg.Content.OfString.Value)
		extraFields := msg.ExtraFields()
		_, hasReasoning := extraFields["reasoning_content"]
		require.False(t, hasReasoning)
	})

	t.Run("should preserve system and user messages unchanged", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleSystem,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "You are helpful."},
				},
			},
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Hello"},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 2)

		// System message - unchanged
		systemMsg := messages[0].OfSystem
		require.NotNil(t, systemMsg)
		require.Equal(t, "You are helpful.", systemMsg.Content.OfString.Value)

		// User message - unchanged
		userMsg := messages[1].OfUser
		require.NotNil(t, userMsg)
		require.Equal(t, "Hello", userMsg.Content.OfString.Value)
	})

	t.Run("should concatenate assistant TextParts in order", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Hello"},
				},
			},
			{
				Role: yuzu.MessageRoleAssistant,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "First part."},
					yuzu.TextPart{Text: "Second part."},
					yuzu.TextPart{Text: "Third part."},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 2)

		// Multi-segment assistant turns concatenate (F2, CHARM-2020):
		// last-write-wins silently dropped every segment but the last.
		assistantMsg := messages[1].OfAssistant
		require.NotNil(t, assistantMsg)
		require.Equal(t, "First part.\nSecond part.\nThird part.", assistantMsg.Content.OfString.Value)
	})

	t.Run("should include user messages with only unsupported attachments", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Hello"},
				},
			},
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.FilePart{
						MediaType: "application/x-unsupported",
						Data:      []byte("unsupported data"),
					},
				},
			},
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "After unsupported"},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Len(t, warnings, 2) // unsupported type + empty message
		require.Contains(t, warnings[0].Message, "not supported")
		require.Contains(t, warnings[1].Message, "dropping empty user message")
		// Should have only 2 messages (empty content message is now dropped)
		require.Len(t, messages, 2)

		msg1 := messages[0].OfUser
		require.NotNil(t, msg1)
		require.Equal(t, "Hello", msg1.Content.OfString.Value)

		msg2 := messages[1].OfUser
		require.NotNil(t, msg2)
		require.Equal(t, "After unsupported", msg2.Content.OfString.Value)
	})

	t.Run("should detect PDF file IDs using strings.HasPrefix", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Check this PDF"},
					yuzu.FilePart{
						MediaType: "application/pdf",
						Data:      []byte("file-abc123xyz"),
						Filename:  "test.pdf",
					},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 1)

		userMsg := messages[0].OfUser
		require.NotNil(t, userMsg)

		content := userMsg.Content.OfArrayOfContentParts
		require.Len(t, content, 2)

		// Second content part should be file with file_id
		filePart := content[1].OfFile
		require.NotNil(t, filePart)
		require.Equal(t, "file-abc123xyz", filePart.File.FileID.Value)
	})
}

func TestDefaultToPrompt_ReasoningRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("should add reasoning_content to tool call messages only when the message itself reasoned", func(t *testing.T) {
		t.Parallel()

		// reasoning_content presence is decided per message from its own
		// reasoning part, never from conversation history. This keeps
		// resubmitted history byte-stable for prefix caches (see #330),
		// while still emitting the (possibly empty) field that providers
		// like Kimi require on assistant tool-call messages.
		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "What is 2+2?"},
				},
			},
			{
				// First turn has reasoning
				Role: yuzu.MessageRoleAssistant,
				Content: []yuzu.MessagePart{
					yuzu.ReasoningPart{Text: "Simple math."},
					yuzu.TextPart{Text: "Four"},
				},
			},
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Now try a tool call"},
				},
			},
			{
				// Tool call WITHOUT reasoning on this turn
				Role: yuzu.MessageRoleAssistant,
				Content: []yuzu.MessagePart{
					yuzu.ToolCallPart{
						ToolCallID: "call_1",
						ToolName:   "execute",
						Input:      `{"command":"echo 4"}`,
					},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 4)

		// The reasoning turn keeps its reasoning_content.
		reasoningMsg := messages[1].OfAssistant
		require.NotNil(t, reasoningMsg)
		rc, ok := reasoningMsg.ExtraFields()["reasoning_content"]
		require.True(t, ok)
		require.Equal(t, "Simple math.", rc)

		// The tool-call message has no reasoning part of its own, so no
		// reasoning_content is manufactured for it.
		msg := messages[3].OfAssistant
		require.NotNil(t, msg)
		_, hasReasoning := msg.ExtraFields()["reasoning_content"]
		require.False(t, hasReasoning, "reasoning_content must not be synthesized from history (see #330)")
	})

	t.Run("should emit empty reasoning_content when the tool call message has an empty reasoning part", func(t *testing.T) {
		t.Parallel()

		// A present-but-empty reasoning part on the message itself still
		// emits reasoning_content: "" (present), satisfying providers like
		// Kimi while staying byte-stable.
		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Run it"},
				},
			},
			{
				Role: yuzu.MessageRoleAssistant,
				Content: []yuzu.MessagePart{
					yuzu.ReasoningPart{Text: ""},
					yuzu.ToolCallPart{
						ToolCallID: "call_1",
						ToolName:   "execute",
						Input:      `{"command":"echo 4"}`,
					},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 2)

		msg := messages[1].OfAssistant
		require.NotNil(t, msg)
		reasoningContent, hasReasoning := msg.ExtraFields()["reasoning_content"]
		require.True(t, hasReasoning, "present-but-empty reasoning part must emit reasoning_content")
		require.Equal(t, "", reasoningContent)
	})
}

func TestDefaultToPrompt_ContentExtraFields(t *testing.T) {
	t.Parallel()

	t.Run("should add cache_control to user message with content array", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{
						Text: "Analyze this document.",
						ProviderOptions: yuzu.ProviderOptions{
							ExtraFields: map[string]any{"cache_control": map[string]string{"type": "ephemeral"}},
						},
					},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 1)

		msg := messages[0].OfUser
		require.NotNil(t, msg)
		// Should use content array format when cache_control is present
		require.Equal(t, "", msg.Content.OfString.Value)
		require.NotNil(t, msg.Content.OfArrayOfContentParts)
		require.Len(t, msg.Content.OfArrayOfContentParts, 1)

		textBlock := msg.Content.OfArrayOfContentParts[0].OfText
		require.NotNil(t, textBlock)
		require.Equal(t, "Analyze this document.", textBlock.Text)

		// Check cache_control in extra fields
		extraFields := textBlock.ExtraFields()
		cacheControl, hasCacheControl := extraFields["cache_control"]
		require.True(t, hasCacheControl)
		cacheMap, ok := cacheControl.(map[string]string)
		require.True(t, ok)
		require.Equal(t, "ephemeral", cacheMap["type"])
	})

	t.Run("should add cache_control to system message with content array", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleSystem,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{
						Text: "You are a helpful assistant.",
						ProviderOptions: yuzu.ProviderOptions{
							ExtraFields: map[string]any{"cache_control": map[string]string{"type": "ephemeral"}},
						},
					},
				},
			},
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Hello"},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 2)

		systemMsg := messages[0].OfSystem
		require.NotNil(t, systemMsg)
		// Should use content array format when cache_control is present
		require.Equal(t, "", systemMsg.Content.OfString.Value)
		require.NotNil(t, systemMsg.Content.OfArrayOfContentParts)
		require.Len(t, systemMsg.Content.OfArrayOfContentParts, 1)

		textBlock := systemMsg.Content.OfArrayOfContentParts[0]
		require.Equal(t, "You are a helpful assistant.", textBlock.Text)

		// Check cache_control in extra fields
		extraFields := textBlock.ExtraFields()
		cacheControl, hasCacheControl := extraFields["cache_control"]
		require.True(t, hasCacheControl)
		cacheMap, ok := cacheControl.(map[string]string)
		require.True(t, ok)
		require.Equal(t, "ephemeral", cacheMap["type"])
	})

	t.Run("should add cache_control to assistant message with content array", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Hello"},
				},
			},
			{
				Role: yuzu.MessageRoleAssistant,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{
						Text: "Hi there!",
						ProviderOptions: yuzu.ProviderOptions{
							ExtraFields: map[string]any{"cache_control": map[string]string{"type": "ephemeral"}},
						},
					},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 2)

		assistantMsg := messages[1].OfAssistant
		require.NotNil(t, assistantMsg)
		// Should use content array format when cache_control is present
		require.Equal(t, "", assistantMsg.Content.OfString.Value)
		require.NotNil(t, assistantMsg.Content.OfArrayOfContentParts)
		require.Len(t, assistantMsg.Content.OfArrayOfContentParts, 1)

		textBlock := assistantMsg.Content.OfArrayOfContentParts[0].OfText
		require.NotNil(t, textBlock)
		require.Equal(t, "Hi there!", textBlock.Text)

		// Check cache_control in extra fields
		extraFields := textBlock.ExtraFields()
		cacheControl, hasCacheControl := extraFields["cache_control"]
		require.True(t, hasCacheControl)
		cacheMap, ok := cacheControl.(map[string]string)
		require.True(t, ok)
		require.Equal(t, "ephemeral", cacheMap["type"])
	})

	t.Run("should not use content array for messages without cache_control", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Hello"},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 1)

		msg := messages[0].OfUser
		require.NotNil(t, msg)
		// Should use string format when no cache_control
		require.NotEqual(t, "", msg.Content.OfString.Value)
		require.Equal(t, "Hello", msg.Content.OfString.Value)
		require.Nil(t, msg.Content.OfArrayOfContentParts)
	})

	t.Run("should handle multiple content parts with mixed cache_control", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{Text: "Context: "},
					yuzu.TextPart{
						Text: "Large document here...",
						ProviderOptions: yuzu.ProviderOptions{
							ExtraFields: map[string]any{"cache_control": map[string]string{"type": "ephemeral"}},
						},
					},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 1)

		msg := messages[0].OfUser
		require.NotNil(t, msg)
		// Should use content array format since one part has cache_control
		require.Equal(t, "", msg.Content.OfString.Value)
		require.NotNil(t, msg.Content.OfArrayOfContentParts)
		require.Len(t, msg.Content.OfArrayOfContentParts, 2)

		// First part should not have cache_control
		firstBlock := msg.Content.OfArrayOfContentParts[0].OfText
		require.NotNil(t, firstBlock)
		require.Equal(t, "Context: ", firstBlock.Text)
		require.Empty(t, firstBlock.ExtraFields())

		// Second part should have cache_control
		secondBlock := msg.Content.OfArrayOfContentParts[1].OfText
		require.NotNil(t, secondBlock)
		require.Equal(t, "Large document here...", secondBlock.Text)
		extraFields := secondBlock.ExtraFields()
		cacheControl, hasCacheControl := extraFields["cache_control"]
		require.True(t, hasCacheControl)
		cacheMap, ok := cacheControl.(map[string]string)
		require.True(t, ok)
		require.Equal(t, "ephemeral", cacheMap["type"])
	})

	t.Run("should fall back to message-level cache_control when part has none", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{
						Text: "Hello",
						// No cache_control on this part
					},
				},
				// But cache_control on the message itself
				ProviderOptions: yuzu.ProviderOptions{
					ExtraFields: map[string]any{"cache_control": map[string]string{"type": "ephemeral"}},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 1)

		msg := messages[0].OfUser
		require.NotNil(t, msg)
		// Should use content array format due to message-level cache_control
		require.Equal(t, "", msg.Content.OfString.Value)
		require.NotNil(t, msg.Content.OfArrayOfContentParts)
		require.Len(t, msg.Content.OfArrayOfContentParts, 1)

		textBlock := msg.Content.OfArrayOfContentParts[0].OfText
		require.NotNil(t, textBlock)
		require.Equal(t, "Hello", textBlock.Text)

		// Check cache_control in extra fields
		extraFields := textBlock.ExtraFields()
		cacheControl, hasCacheControl := extraFields["cache_control"]
		require.True(t, hasCacheControl)
		cacheMap, ok := cacheControl.(map[string]string)
		require.True(t, ok)
		require.Equal(t, "ephemeral", cacheMap["type"])
	})

	t.Run("should prefer part-level cache_control over message-level", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleUser,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{
						Text: "First part",
						// Part-level cache_control with different type
						ProviderOptions: yuzu.ProviderOptions{
							ExtraFields: map[string]any{"cache_control": map[string]string{"type": "ephemeral"}},
						},
					},
					yuzu.TextPart{Text: "Second part"}, // No part-level
				},
				// Message-level cache_control
				ProviderOptions: yuzu.ProviderOptions{
					ExtraFields: map[string]any{"cache_control": map[string]string{"type": "persistent"}},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 1)

		msg := messages[0].OfUser
		require.NotNil(t, msg)
		require.NotNil(t, msg.Content.OfArrayOfContentParts)
		require.Len(t, msg.Content.OfArrayOfContentParts, 2)

		// First part should have ephemeral (part-level wins)
		firstBlock := msg.Content.OfArrayOfContentParts[0].OfText
		require.NotNil(t, firstBlock)
		extraFields := firstBlock.ExtraFields()
		cacheControl := extraFields["cache_control"].(map[string]string)
		require.Equal(t, "ephemeral", cacheControl["type"])

		// Second part should have persistent (message-level fallback)
		secondBlock := msg.Content.OfArrayOfContentParts[1].OfText
		require.NotNil(t, secondBlock)
		extraFields = secondBlock.ExtraFields()
		cacheControl = extraFields["cache_control"].(map[string]string)
		require.Equal(t, "persistent", cacheControl["type"])
	})

	t.Run("should add cache_control to multi-part assistant message with tool calls", func(t *testing.T) {
		t.Parallel()

		prompt := yuzu.Prompt{
			{
				Role: yuzu.MessageRoleAssistant,
				Content: []yuzu.MessagePart{
					yuzu.TextPart{
						Text: "Let me check that.",
						ProviderOptions: yuzu.ProviderOptions{
							ExtraFields: map[string]any{"cache_control": map[string]string{"type": "ephemeral"}},
						},
					},
					yuzu.ToolCallPart{
						ToolCallID: "call_1",
						ToolName:   "lookup",
						Input:      "{}",
					},
				},
			},
		}

		messages, warnings := DefaultToPrompt(prompt, "", "")

		require.Empty(t, warnings)
		require.Len(t, messages, 1)

		assistantMsg := messages[0].OfAssistant
		require.NotNil(t, assistantMsg)
		// Tool calls force the multi-part path; cache_control must still survive.
		require.Equal(t, "", assistantMsg.Content.OfString.Value)
		require.Len(t, assistantMsg.Content.OfArrayOfContentParts, 1)
		require.Len(t, assistantMsg.ToolCalls, 1)

		textBlock := assistantMsg.Content.OfArrayOfContentParts[0].OfText
		require.NotNil(t, textBlock)
		require.Equal(t, "Let me check that.", textBlock.Text)
		cacheControl := textBlock.ExtraFields()["cache_control"].(map[string]string)
		require.Equal(t, "ephemeral", cacheControl["type"])
	})
}

// TestDefaultToPrompt_MultiSegmentConcatenation covers the CHARM-2020 F2 case:
// an assistant turn with interleaved reasoning segments (a model that
// reasons, speaks, then reasons again) must not lose segments — text and
// reasoning each concatenate in order instead of last-write-wins.
func TestDefaultToPrompt_MultiSegmentConcatenation(t *testing.T) {
	t.Parallel()

	prompt := yuzu.Prompt{
		{
			Role: yuzu.MessageRoleUser,
			Content: []yuzu.MessagePart{
				yuzu.TextPart{Text: "hi"},
			},
		},
		{
			Role: yuzu.MessageRoleAssistant,
			Content: []yuzu.MessagePart{
				yuzu.ReasoningPart{Text: "a"},
				yuzu.TextPart{Text: "x"},
				yuzu.ToolCallPart{ToolCallID: "c1", ToolName: "f", Input: "{}"},
				yuzu.ReasoningPart{Text: "b"},
				yuzu.TextPart{Text: "y"},
			},
		},
	}

	messages, warnings := DefaultToPrompt(prompt, "", "")
	require.Empty(t, warnings)
	require.Len(t, messages, 2)

	msg := messages[1].OfAssistant
	require.NotNil(t, msg)
	require.Equal(t, "x\ny", msg.Content.OfString.Value, "text parts must concatenate, not overwrite")
	require.Len(t, msg.ToolCalls, 1)
	require.Equal(t, "c1", msg.ToolCalls[0].OfFunction.ID)

	extraFields := msg.ExtraFields()
	reasoningContent, hasReasoning := extraFields["reasoning_content"]
	require.True(t, hasReasoning)
	require.Equal(t, "a\nb", reasoningContent, "reasoning parts must concatenate, not overwrite")
}

// Mixed plain and extra-field text segments must keep their original
// order: A (plain), B (cached), C (plain) must serialize as A, B, C.
func TestDefaultToPrompt_MixedTextSegmentOrder(t *testing.T) {
	t.Parallel()

	prompt := yuzu.Prompt{
		{
			Role: yuzu.MessageRoleAssistant,
			Content: []yuzu.MessagePart{
				yuzu.TextPart{Text: "A"},
				yuzu.TextPart{
					Text: "B",
					ProviderOptions: yuzu.ProviderOptions{
						ExtraFields: map[string]any{
							"cache_control": map[string]string{"type": "ephemeral"},
						},
					},
				},
				yuzu.TextPart{Text: "C"},
			},
		},
	}

	messages, warnings := DefaultToPrompt(prompt, "", "")
	require.Empty(t, warnings)
	require.Len(t, messages, 1)

	msg := messages[0].OfAssistant
	require.NotNil(t, msg)
	blocks := msg.Content.OfArrayOfContentParts
	require.Len(t, blocks, 3, "expected array form with one block per segment, got %+v", msg.Content)
	require.Equal(t, "A", blocks[0].OfText.Text)
	require.Equal(t, "B", blocks[1].OfText.Text)
	require.Equal(t, "C", blocks[2].OfText.Text)
	require.Equal(t, map[string]any{"cache_control": map[string]string{"type": "ephemeral"}}, blocks[1].OfText.ExtraFields())
}

// A reasoning-only assistant turn must survive: reasoning_content is
// visible content. Dropping it breaks DeepSeek's replay contract on
// truncated thinking turns.
func TestDefaultToPrompt_ReasoningOnlyTurnKept(t *testing.T) {
	t.Parallel()

	prompt := yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "hi"}}},
		{Role: yuzu.MessageRoleAssistant, Content: []yuzu.MessagePart{
			yuzu.ReasoningPart{Text: "thinking…"},
		}},
	}

	messages, warnings := DefaultToPrompt(prompt, "", "")
	require.Empty(t, warnings)
	require.Len(t, messages, 2, "reasoning-only assistant turn must not be dropped")
	msg := messages[1].OfAssistant
	require.NotNil(t, msg)
	require.Equal(t, "thinking…", msg.ExtraFields()["reasoning_content"])
}

func TestGenerate_ExtraBodyOption(t *testing.T) {
	t.Parallel()

	server := newMockServer()
	defer server.close()
	server.prepareJSONResponse(map[string]any{"content": "hi"})

	provider, err := New(
		WithAPIKey("test-api-key"),
		WithBaseURL(server.server.URL),
	)
	require.NoError(t, err)

	model, err := provider.LanguageModel(t.Context(), "gpt-4o")
	require.NoError(t, err)

	_, err = model.Generate(t.Context(), yuzu.Call{
		Prompt: testPrompt,
		ProviderOptions: yuzu.ProviderOptions{
			ExtraBody: map[string]any{"thinking": map[string]any{"type": "enabled"}},
		},
	})
	require.NoError(t, err)
	require.Len(t, server.calls, 1)
	require.Equal(t, map[string]any{"type": "enabled"}, server.calls[0].body["thinking"])
}
