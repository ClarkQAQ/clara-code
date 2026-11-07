package provider

import (
	yuzu "clara-code/pkg/model"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareParams_Store(t *testing.T) {
	t.Parallel()

	lm := testResponsesLM()
	prompt := yuzu.Prompt{testTextMessage(yuzu.MessageRoleUser, "hello")}

	tests := []struct {
		name      string
		opts      yuzu.ProviderOptions
		wantStore bool
	}{
		{
			name:      "store true",
			opts:      yuzu.ProviderOptions{Store: new(true)},
			wantStore: true,
		},
		{
			name:      "store false",
			opts:      yuzu.ProviderOptions{Store: new(false)},
			wantStore: false,
		},
		{
			name:      "store default",
			opts:      yuzu.ProviderOptions{},
			wantStore: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			params, warnings, err := lm.prepareParams(testCall(prompt, tt.opts))
			require.NoError(t, err)
			require.Empty(t, warnings)
			require.True(t, params.Store.Valid())
			require.Equal(t, tt.wantStore, params.Store.Value)
		})
	}
}

func TestPrepareParams_PreviousResponseID(t *testing.T) {
	t.Parallel()

	lm := testResponsesLM()
	prompt := yuzu.Prompt{testTextMessage(yuzu.MessageRoleUser, "hello")}

	t.Run("forwarded", func(t *testing.T) {
		t.Parallel()

		params, warnings, err := lm.prepareParams(testCall(prompt, yuzu.ProviderOptions{
			PreviousResponseID: new("resp_abc123"),
			Store:              new(true),
		}))
		require.NoError(t, err)
		require.Empty(t, warnings)
		require.True(t, params.PreviousResponseID.Valid())
		require.Equal(t, "resp_abc123", params.PreviousResponseID.Value)
	})

	t.Run("not set", func(t *testing.T) {
		t.Parallel()

		params, warnings, err := lm.prepareParams(testCall(prompt, yuzu.ProviderOptions{}))
		require.NoError(t, err)
		require.Empty(t, warnings)
		require.False(t, params.PreviousResponseID.Valid())
	})

	t.Run("empty string ignored", func(t *testing.T) {
		t.Parallel()

		params, warnings, err := lm.prepareParams(testCall(prompt, yuzu.ProviderOptions{
			PreviousResponseID: new(""),
		}))
		require.NoError(t, err)
		require.Empty(t, warnings)
		require.False(t, params.PreviousResponseID.Valid())
	})
}

func TestPrepareParams_PreviousResponseID_Validation(t *testing.T) {
	t.Parallel()

	lm := testResponsesLM()
	opts := yuzu.ProviderOptions{
		PreviousResponseID: new("resp_abc123"),
		Store:              new(true),
	}

	t.Run("rejects with assistant messages", func(t *testing.T) {
		t.Parallel()

		_, _, err := lm.prepareParams(testCall(yuzu.Prompt{
			testTextMessage(yuzu.MessageRoleUser, "hello"),
			testTextMessage(yuzu.MessageRoleAssistant, "hi there"),
		}, opts))
		require.EqualError(t, err, previousResponseIDHistoryError)
	})

	t.Run("allows user-only prompt", func(t *testing.T) {
		t.Parallel()

		_, warnings, err := lm.prepareParams(testCall(yuzu.Prompt{
			testTextMessage(yuzu.MessageRoleUser, "hello"),
			testTextMessage(yuzu.MessageRoleUser, "follow up"),
		}, opts))
		require.NoError(t, err)
		require.Empty(t, warnings)
	})

	t.Run("allows system + user prompt", func(t *testing.T) {
		t.Parallel()

		_, warnings, err := lm.prepareParams(testCall(yuzu.Prompt{
			testTextMessage(yuzu.MessageRoleSystem, "be concise"),
			testTextMessage(yuzu.MessageRoleUser, "hello"),
		}, opts))
		require.NoError(t, err)
		require.Empty(t, warnings)
	})

	t.Run("rejects tool messages", func(t *testing.T) {
		t.Parallel()

		_, _, err := lm.prepareParams(testCall(yuzu.Prompt{
			testToolResultMessage("done"),
			testTextMessage(yuzu.MessageRoleUser, "hello"),
		}, opts))
		require.EqualError(t, err, previousResponseIDHistoryError)
	})

	t.Run("rejects without store", func(t *testing.T) {
		t.Parallel()

		_, _, err := lm.prepareParams(testCall(yuzu.Prompt{
			testTextMessage(yuzu.MessageRoleUser, "hello"),
		}, yuzu.ProviderOptions{
			PreviousResponseID: new("resp_abc123"),
		}))
		require.EqualError(t, err, previousResponseIDStoreError)
	})

	t.Run("rejects with store false", func(t *testing.T) {
		t.Parallel()

		_, _, err := lm.prepareParams(testCall(yuzu.Prompt{
			testTextMessage(yuzu.MessageRoleUser, "hello"),
		}, yuzu.ProviderOptions{
			PreviousResponseID: new("resp_abc123"),
			Store:              new(false),
		}))
		require.EqualError(t, err, previousResponseIDStoreError)
	})
}

func TestValidatePreviousResponseIDPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prompt  yuzu.Prompt
		wantErr bool
	}{
		{
			name:   "empty prompt",
			prompt: nil,
		},
		{
			name: "user-only messages",
			prompt: yuzu.Prompt{
				testTextMessage(yuzu.MessageRoleUser, "hello"),
				testTextMessage(yuzu.MessageRoleUser, "follow up"),
			},
		},
		{
			name: "system + user messages",
			prompt: yuzu.Prompt{
				testTextMessage(yuzu.MessageRoleSystem, "be concise"),
				testTextMessage(yuzu.MessageRoleUser, "hello"),
			},
		},
		{
			name: "contains assistant message",
			prompt: yuzu.Prompt{
				testTextMessage(yuzu.MessageRoleAssistant, "hi there"),
			},
			wantErr: true,
		},
		{
			name: "assistant in the middle",
			prompt: yuzu.Prompt{
				testTextMessage(yuzu.MessageRoleUser, "hello"),
				testTextMessage(yuzu.MessageRoleAssistant, "hi there"),
				testTextMessage(yuzu.MessageRoleUser, "follow up"),
			},
			wantErr: true,
		},
		{
			name: "contains tool message",
			prompt: yuzu.Prompt{
				testToolResultMessage("done"),
				testTextMessage(yuzu.MessageRoleUser, "follow up"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validatePreviousResponseIDPrompt(tt.prompt)
			if tt.wantErr {
				require.EqualError(t, err, previousResponseIDHistoryError)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestResponsesProviderMetadata_Helper(t *testing.T) {
	t.Parallel()

	t.Run("non-empty id", func(t *testing.T) {
		t.Parallel()

		metadata := responsesProviderMetadata("resp_123")
		require.Equal(t, "resp_123", metadata.ResponseID)
	})

	t.Run("empty id", func(t *testing.T) {
		t.Parallel()

		metadata := responsesProviderMetadata("")
		require.True(t, metadata.IsZero())
	})
}

func TestResponsesProviderMetadata_JSON(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(yuzu.ProviderMetadata{ResponseID: "resp_123"})
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"response_id":"resp_123"`)

	var decoded yuzu.ProviderMetadata
	err = json.Unmarshal(encoded, &decoded)
	require.NoError(t, err)
	require.Equal(t, "resp_123", decoded.ResponseID)
}

func testCall(prompt yuzu.Prompt, opts yuzu.ProviderOptions) yuzu.Call {
	return yuzu.Call{Prompt: prompt, ProviderOptions: opts}
}

func testResponsesLM() responsesLanguageModel {
	return responsesLanguageModel{
		provider: Name,
		modelID:  "gpt-4o",
	}
}

func testTextMessage(role yuzu.MessageRole, text string) yuzu.Message {
	return yuzu.Message{
		Role: role,
		Content: []yuzu.MessagePart{
			yuzu.TextPart{Text: text},
		},
	}
}

func testToolResultMessage(text string) yuzu.Message {
	return yuzu.Message{
		Role: yuzu.MessageRoleTool,
		Content: []yuzu.MessagePart{
			yuzu.ToolResultPart{
				ToolCallID: "call_123",
				Output: yuzu.ToolResultOutputContentText{
					Text: text,
				},
			},
		},
	}
}
