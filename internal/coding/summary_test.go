package coding

import (
	"testing"

	"clara-code/internal/setting"
	"clara-code/pkg/model"
)

func TestSummaryDisabledInModelDefaultsAndExplicitOverride(t *testing.T) {
	for _, tc := range []struct {
		defaultValue string
		override     *string
	}{
		{defaultValue: "none"},
		{defaultValue: "auto", override: new("none")},
	} {
		m := capabilityModel{profile: setting.Model{DefaultReasoningSummary: tc.defaultValue}}
		call := m.options(model.Call{ProviderOptions: model.ProviderOptions{ReasoningSummary: tc.override, ReasoningEffort: new(model.ReasoningEffortHigh)}})
		if call.ProviderOptions.ReasoningSummary != nil || *call.ProviderOptions.ReasoningEffort != "high" {
			t.Fatal("disabled summary not normalized independently of effort")
		}
	}
}
