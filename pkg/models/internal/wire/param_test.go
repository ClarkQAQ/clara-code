package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptMarshalling(t *testing.T) {
	t.Parallel()

	t.Run("unset is omitted", func(t *testing.T) {
		t.Parallel()

		body, err := json.Marshal(ChatCompletionNewParams{Model: "gpt-4o"})
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"gpt-4o"}`, string(body))
	})

	t.Run("explicit false is kept", func(t *testing.T) {
		t.Parallel()

		body, err := json.Marshal(ChatCompletionNewParams{Store: NewOpt(false)})
		require.NoError(t, err)
		require.JSONEq(t, `{"store":false}`, string(body))
	})

	t.Run("null is kept", func(t *testing.T) {
		t.Parallel()

		body, err := json.Marshal(ChatCompletionNewParams{Metadata: Metadata{}})
		require.NoError(t, err)
		require.JSONEq(t, `{"metadata":{}}`, string(body))
	})

	t.Run("zero value reports omitted", func(t *testing.T) {
		t.Parallel()

		require.True(t, IsOmitted(Opt[string]{}))
		require.False(t, IsOmitted(NewOpt("")))
		require.False(t, IsOmitted(String("value")))
	})

	t.Run("value round trip", func(t *testing.T) {
		t.Parallel()

		var opt Opt[int64]
		require.NoError(t, json.Unmarshal([]byte("42"), &opt))
		require.True(t, opt.Valid())
		require.Equal(t, int64(42), opt.Value)
		require.False(t, IsOmitted(opt))
	})
}

func TestMarshalObjectAppliesDefaults(t *testing.T) {
	t.Parallel()

	t.Run("content part type", func(t *testing.T) {
		t.Parallel()

		body, err := json.Marshal(ChatCompletionContentPartTextParam{Text: "hello"})
		require.NoError(t, err)
		require.JSONEq(t, `{"text":"hello","type":"text"}`, string(body))
	})

	t.Run("message role", func(t *testing.T) {
		t.Parallel()

		message := SystemMessage("hello")
		body, err := json.Marshal(message)
		require.NoError(t, err)
		require.JSONEq(t, `{"role":"system","content":"hello"}`, string(body))
	})

	t.Run("explicit discriminator wins", func(t *testing.T) {
		t.Parallel()

		body, err := json.Marshal(ChatCompletionContentPartTextParam{Text: "hi", Type: "custom"})
		require.NoError(t, err)
		require.JSONEq(t, `{"text":"hi","type":"custom"}`, string(body))
	})

	t.Run("extra fields are merged", func(t *testing.T) {
		t.Parallel()

		params := ChatCompletionNewParams{Model: "gpt-4o"}
		params.SetExtraFields(map[string]any{"reasoning": map[string]any{"effort": "low"}})
		body, err := json.Marshal(params)
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"gpt-4o","reasoning":{"effort":"low"}}`, string(body))
	})
}

func TestMarshalUnion(t *testing.T) {
	t.Parallel()

	t.Run("single variant", func(t *testing.T) {
		t.Parallel()

		message := UserMessage("hi")
		body, err := json.Marshal(message)
		require.NoError(t, err)
		require.JSONEq(t, `{"role":"user","content":"hi"}`, string(body))
	})

	t.Run("content parts", func(t *testing.T) {
		t.Parallel()

		message := ToolMessage([]ChatCompletionContentPartTextParam{{Text: "done"}}, "call-1")
		body, err := json.Marshal(message)
		require.NoError(t, err)
		require.JSONEq(t, `{"role":"tool","tool_call_id":"call-1","content":[{"text":"done","type":"text"}]}`, string(body))
	})

	t.Run("discriminated round trip", func(t *testing.T) {
		t.Parallel()

		var message ChatCompletionMessageParamUnion
		require.NoError(t, json.Unmarshal([]byte(`{"role":"assistant","content":"hi"}`), &message))
		require.NotNil(t, message.OfAssistant)
		require.Equal(t, "hi", message.OfAssistant.Content.OfString.Value)
	})

	t.Run("kind discriminated round trip", func(t *testing.T) {
		t.Parallel()

		var stop ChatCompletionNewParamsStopUnion
		require.NoError(t, json.Unmarshal([]byte(`["a","b"]`), &stop))
		require.Equal(t, []string{"a", "b"}, stop.OfStringArray)
	})
}
