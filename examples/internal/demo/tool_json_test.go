package demo

import (
	"context"
	"errors"
	"testing"

	"clara-code/pkg/core"
	modelapi "clara-code/pkg/model"
)

func TestToolJSONValidationAndExplicitRepair(t *testing.T) {
	cases := []struct {
		name, input           string
		repair, deny, wantRun bool
	}{
		{name: "malformed", input: `{"value":"x"`},
		{name: "null", input: "null"},
		{name: "array", input: "[]"},
		{name: "valid", input: `{"value":"x"}`, wantRun: true},
		{name: "explicit repair", input: `{"value":"x"`, repair: true, wantRun: true},
		{name: "repair still checks permission", input: `{"value":"x"`, repair: true, deny: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			type input struct {
				Value string `json:"value"`
			}
			runs, permissions := 0, 0
			tool := modelapi.NewAgentTool("change", "Change a value.", func(_ context.Context, args input, _ modelapi.ToolCall) (modelapi.ToolResponse, error) {
				runs++
				return modelapi.NewTextResponse(args.Value), nil
			})
			call := core.AgentStreamCall{Prompt: "Apply the change.", StopWhen: []core.StopCondition{core.StepCountIs(1)}}
			if tt.repair {
				call.RepairToolCall = func(_ context.Context, options core.ToolCallRepairOptions) (*modelapi.ToolCallContent, error) {
					return &modelapi.ToolCallContent{ToolCallID: "replacement", ToolName: options.OriginalToolCall.ToolName, Input: `{"value":"x"}`}, nil
				}
			}
			call.AuthorizeTool = func(context.Context, modelapi.ToolInfo, modelapi.ToolCall) error {
				permissions++
				if tt.deny {
					return errors.New("denied")
				}
				return nil
			}
			model := Model{Reply: func(context.Context, modelapi.Call) (modelapi.Response, error) {
				return modelapi.Response{Content: modelapi.ResponseContent{modelapi.ToolCallContent{
					ToolCallID: "original", ToolName: "change", Input: tt.input,
				}}, FinishReason: modelapi.FinishReasonToolCalls}, nil
			}}
			result, err := core.NewAgent(model, core.WithTools(tool)).Stream(t.Context(), call)
			if err != nil {
				t.Fatal(err)
			}
			out := result.Steps[0].Content.ToolResults()
			if len(out) != 1 || out[0].ToolCallID != "original" {
				t.Fatalf("expected result for original call, got %+v", out)
			}
			if (runs == 1) != tt.wantRun || (out[0].Result.GetType() == modelapi.ToolResultContentTypeText) != tt.wantRun {
				t.Fatalf("runs=%d, result=%+v", runs, out[0].Result)
			}
			if (permissions == 1) != (tt.wantRun || tt.deny) {
				t.Fatalf("unexpected authorization count %d", permissions)
			}
		})
	}
}
