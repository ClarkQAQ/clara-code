package core

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"clara-code/pkg/model"
)

type longRunModel struct {
	model.LanguageModel
	calls int
	stop  int
}

func (m *longRunModel) Generate(ctx context.Context, _ model.Call) (*model.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.calls++
	response := &model.Response{FinishReason: model.FinishReasonStop, Usage: model.Usage{TotalTokens: 1}}
	if m.calls < m.stop {
		response.Content = model.ResponseContent{model.ToolCallContent{
			ToolCallID: strconv.Itoa(m.calls),
			ToolName:   "tick",
			Input:      "{}",
		}}
		response.FinishReason = model.FinishReasonToolCalls
	} else {
		response.Content = model.ResponseContent{model.TextContent{Text: "done"}}
	}
	return response, nil
}

func (m *longRunModel) Stream(ctx context.Context, call model.Call) (model.StreamResponse, error) {
	response, err := m.Generate(ctx, call)
	if err != nil {
		return nil, err
	}
	return responseStream(response), nil
}

func TestAgentStepBudget(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options []AgentOption
		calls   int
	}{
		{name: "default generation", calls: 105},
		{name: "default streaming", calls: 105},
		{name: "explicit budget", options: []AgentOption{WithMaxSteps(3)}, calls: 3},
		{name: "reset budget", options: []AgentOption{WithMaxSteps(3), WithMaxSteps(0)}, calls: 105},
	} {
		t.Run(tc.name, func(t *testing.T) {
			language := &longRunModel{stop: 105}
			engine := NewAgent(language, tc.options...)
			var result *AgentResult
			var err error
			if tc.name == "default generation" {
				result, err = engine.Generate(t.Context(), AgentCall{Prompt: "Run to completion"})
			} else {
				result, err = engine.Stream(t.Context(), AgentStreamCall{Prompt: "Run to completion"})
			}
			if tc.calls < language.stop {
				if err == nil || !strings.Contains(err.Error(), "agent step limit reached") {
					t.Fatalf("expected budget exhaustion, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if result == nil || language.calls != tc.calls || len(result.Steps) != tc.calls ||
				result.TotalUsage.TotalTokens != int64(tc.calls) {
				t.Fatalf("after %d calls, unexpected result and usage", language.calls)
			}
		})
	}
}

func TestAgentCancelsAfterFormerDefaultBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	language := &longRunModel{stop: 110}
	_, err := NewAgent(language).Stream(ctx, AgentStreamCall{
		Prompt: "Run until cancelled",
		OnStepFinish: func(StepResult) error {
			if language.calls == 103 {
				cancel()
			}
			return nil
		},
	})
	if !errors.Is(err, context.Canceled) || language.calls != 103 {
		t.Fatalf("cancelled after %d calls with error %v", language.calls, err)
	}
}
