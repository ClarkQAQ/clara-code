package prompt_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

type recordingSummaryModel struct {
	model.LanguageModel
	generate func(model.Call) (*model.Response, error)
}

func (m recordingSummaryModel) Generate(_ context.Context, call model.Call) (*model.Response, error) {
	return m.generate(call)
}

func TestCompactionPendingInputAndThresholds(t *testing.T) {
	pending := submission("unsent " + strings.Repeat("A", 10_000))
	history := []model.Message{submission("original"), pending}
	compactor := prompt.Compactor{BudgetBytes: 800, PendingFrom: new(1)}
	out, err := compactor.Fit(t.Context(), nil, "", history, nil)
	if err != nil || out != nil {
		t.Fatal("unsent input affected the byte trigger", err)
	}
	compactor = prompt.Compactor{
		TriggerTokens:       90,
		ActiveContextTokens: new(int64(30)),
		PendingFrom:         new(1),
		Assemble: func([]model.Message) ([]model.Message, error) {
			return nil, errors.New("unexpected full history scan")
		},
	}
	out, err = compactor.Fit(t.Context(), nil, "", history, nil)
	if err != nil || out != nil {
		t.Fatal("re-estimated an authoritative token count", err)
	}
	compactor.Assemble = nil
	compactor.ActiveContextTokens = new(int64(90))
	var request model.Call
	m := summaryModel{input: &request, response: &model.Response{FinishReason: model.FinishReasonStop, Content: model.ResponseContent{model.TextContent{Text: "handoff"}}}}
	out, err = compactor.Fit(t.Context(), m, "", history, nil)
	if err != nil || len(out) != 3 || !reflect.DeepEqual(out[2], pending) || out[1].Metadata.Kind != "handoff" ||
		len(request.Prompt) != 2 || strings.Contains(windowText(request.Prompt), "unsent") {
		t.Fatal("pending input leaked into the summary or moved ahead of the handoff", err)
	}
}

func TestSummaryRetryPrunesPairedToolsOnlyInRequest(t *testing.T) {
	call := model.Message{Role: model.MessageRoleAssistant, Content: []model.MessagePart{
		model.ToolCallPart{ToolCallID: "a", ToolName: "read"}, model.ToolCallPart{ToolCallID: "b", ToolName: "read"},
	}}
	result := func(id string) model.Message {
		return model.Message{Role: model.MessageRoleTool, Content: []model.MessagePart{
			model.ToolResultPart{ToolCallID: id, Output: model.ToolResultOutputContentText{Text: "result " + id}},
		}}
	}
	history := []model.Message{call, result("b"), result("a"), submission("recent")}
	overflow := &model.ProviderError{ContextTooLargeErr: true, StatusCode: 503}
	transient := &model.ProviderError{StatusCode: 503, ResponseHeaders: map[string]string{"retry-after-ms": "1"}}
	attempts := 0
	m := recordingSummaryModel{generate: func(request model.Call) (*model.Response, error) {
		attempts++
		if attempts == 1 {
			if len(request.Prompt) != 6 || request.Prompt[1].Role != model.MessageRoleAssistant {
				t.Fatal("tools missing before overflow")
			}
			return nil, overflow
		}
		if len(request.Prompt) != 3 || request.Prompt[0].Role != model.MessageRoleDeveloper ||
			windowText(request.Prompt[1:2]) != "recent\n" || request.Prompt[2].Role != model.MessageRoleUser {
			t.Fatal("unpaired tools or missing instructions after overflow")
		}
		if attempts == 2 {
			return nil, transient
		}
		return &model.Response{FinishReason: model.FinishReasonStop, Content: model.ResponseContent{
			model.TextContent{Text: "first"}, model.TextContent{Text: "second"},
		}}, nil
	}}
	out, err := (prompt.Compactor{Force: true}).Fit(t.Context(), m, "policy", history, nil)
	if err != nil || attempts != 3 || len(out) != 2 || !strings.Contains(windowText(out), "first\nsecond") ||
		!reflect.DeepEqual(history[:3], []model.Message{call, result("b"), result("a")}) {
		t.Fatal("did not recover without changing history", attempts, err)
	}
}

func TestSummaryErrorsPreserveOriginalHistory(t *testing.T) {
	user := submission("recent")
	overflow := &model.ProviderError{ContextTooLargeErr: true}
	transient := &model.ProviderError{StatusCode: 503, ResponseHeaders: map[string]string{"retry-after-ms": "1"}}
	for _, tc := range []struct {
		name     string
		err      error
		response *model.Response
		attempts int
	}{
		{"last real item cannot be removed", overflow, nil, 1},
		{"auth", &model.ProviderError{StatusCode: 401}, nil, 1},
		{"bounded retry", transient, nil, 4},
		{"output limit", nil, &model.Response{FinishReason: model.FinishReasonLength, Content: model.ResponseContent{model.TextContent{Text: "partial"}}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			m := recordingSummaryModel{generate: func(model.Call) (*model.Response, error) {
				attempts++
				return tc.response, tc.err
			}}
			out, err := (prompt.Compactor{Force: true}).Fit(t.Context(), m, "policy", []model.Message{user}, nil)
			if err == nil || out != nil || attempts != tc.attempts || tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatal("incorrect retry or rollback", attempts, err)
			}
			if tc.response != nil && !strings.Contains(err.Error(), "output token limit") {
				t.Fatal("missing truncation cause", err)
			}
		})
	}
}

func TestSummaryCancellationInterruptsBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered := make(chan struct{})
	m := recordingSummaryModel{generate: func(model.Call) (*model.Response, error) {
		close(entered)
		return nil, &model.ProviderError{StatusCode: 503}
	}}
	done := make(chan error, 1)
	go func() {
		_, err := (prompt.Compactor{Force: true}).Fit(ctx, m, "", []model.Message{submission("recent")}, nil)
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("summary backoff ignored cancellation", err)
	}
}
