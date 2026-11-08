package provider

import (
	"encoding/json"
	"testing"

	"clara-code/pkg/model"
)

func TestResponsesSummaryNoneIsOmittedWithoutDroppingEffort(t *testing.T) {
	for _, summary := range []string{"none", "", "auto", "concise", "detailed"} {
		t.Run(summary, func(t *testing.T) {
			lm := testResponsesLM()
			lm.reasoningModel = true
			call := testCall(model.Prompt{model.NewUserMessage("task")}, model.ProviderOptions{ReasoningSummary: new(summary), ReasoningEffort: new(model.ReasoningEffortHigh)})
			params, _, err := lm.prepareParams(call)
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			var request struct {
				Reasoning struct {
					Effort  string
					Summary *string
				}
			}
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatal(err)
			}
			if request.Reasoning.Effort != "high" {
				t.Fatal("summary normalization dropped effort")
			}
			if summary == "none" || summary == "" {
				if request.Reasoning.Summary != nil {
					t.Fatal("disabled summary sent upstream", string(body))
				}
			} else if request.Reasoning.Summary == nil || *request.Reasoning.Summary != summary {
				t.Fatal("valid summary lost", string(body))
			}
		})
	}
	lm := testResponsesLM()
	if _, _, err := lm.prepareParams(testCall(nil, model.ProviderOptions{ReasoningSummary: new("unsupported")})); err == nil {
		t.Fatal("invalid summary passed provider validation")
	}
}
