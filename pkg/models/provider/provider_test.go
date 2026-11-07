package provider

import (
	yuzu "clara-code/pkg/models"
	openai "clara-code/pkg/models/internal/wire"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewProvider_Defaults(t *testing.T) {
	t.Parallel()

	_, err := New()
	require.ErrorContains(t, err, "base URL is required")
	const endpoint = "https://models.example/v1"
	provider, err := New(WithBaseURL(endpoint))
	require.NoError(t, err)
	require.Equal(t, APIChatCompletions, provider.options.api)
	require.Equal(t, endpoint, provider.options.baseURL)
}

func TestNewProvider_InvalidAPI(t *testing.T) {
	t.Parallel()

	_, err := New(WithAPI(API("bogus")))
	require.ErrorContains(t, err, "unsupported API")
}

func TestNewProvider_OrganizationAndProjectHeaders(t *testing.T) {
	t.Parallel()

	provider, err := New(
		WithBaseURL("https://models.example/v1"),
		WithOrganization("acme"),
		WithProject("proj"),
	)
	require.NoError(t, err)
	require.Equal(t, "acme", provider.options.headers["OpenAi-Organization"])
	require.Equal(t, "proj", provider.options.headers["OpenAi-Project"])
}

func TestDefaultPrepareCallFunc_UnsupportedResponsesSetting(t *testing.T) {
	t.Parallel()

	model := languageModel{provider: Name, modelID: "gpt-4o"}
	warnings, err := DefaultPrepareCallFunc(model, &openai.ChatCompletionNewParams{}, yuzu.Call{
		ProviderOptions: yuzu.ProviderOptions{
			Include: []yuzu.IncludeType{yuzu.IncludeReasoningEncryptedContent},
		},
	})
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	require.Equal(t, yuzu.CallWarningTypeUnsupportedSetting, warnings[0].Type)
	require.Equal(t, "Include", warnings[0].Setting)
}

func TestPrepareParams_UnsupportedChatCompletionsSetting(t *testing.T) {
	t.Parallel()

	lm := testResponsesLM()
	prompt := yuzu.Prompt{testTextMessage(yuzu.MessageRoleUser, "hello")}

	_, warnings, err := lm.prepareParams(testCall(prompt, yuzu.ProviderOptions{
		LogitBias: map[string]int64{"50256": -100},
	}))
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	require.Equal(t, yuzu.CallWarningTypeUnsupportedSetting, warnings[0].Type)
	require.Equal(t, "LogitBias", warnings[0].Setting)
}
