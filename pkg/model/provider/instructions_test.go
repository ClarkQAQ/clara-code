package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"clara-code/pkg/model"
)

func TestResponsesSystemInstructionsMappingIsExplicit(t *testing.T) {
	lm := testResponsesLM()
	call := testCall(model.Prompt{model.NewSystemMessage("base"), model.NewDeveloperMessage("policy"), model.NewUserMessage("task")}, model.ProviderOptions{})
	params, _, err := lm.prepareParams(call)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(params)
	if strings.Contains(string(body), `"instructions":"base"`) {
		t.Fatal("mapping enabled by default")
	}
	lm.systemInstructions = true
	params, _, err = lm.prepareParams(call)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(params)
	if !strings.Contains(string(body), `"instructions":"base"`) || strings.Count(string(body), "base") != 1 || !strings.Contains(string(body), `"role":"developer"`) {
		t.Fatal("base was duplicated or policy lost", string(body))
	}
	call.ProviderOptions.Instructions = new("explicit")
	params, _, err = lm.prepareParams(call)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(params)
	if !strings.Contains(string(body), `"instructions":"explicit"`) || strings.Contains(string(body), "base") {
		t.Fatal("explicit instructions did not win")
	}
}
