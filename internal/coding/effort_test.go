package coding

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"clara-code/internal/setting"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

func TestCustomDeploymentEffortReachesBothProtocols(t *testing.T) {
	for _, protocol := range []string{"responses", "completions"} {
		t.Run(protocol, func(t *testing.T) {
			var expected atomic.Value
			expected.Store("high")
			c, _ := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Model     string `json:"model"`
					Effort    string `json:"reasoning_effort"`
					Reasoning struct {
						Effort  string  `json:"effort"`
						Summary *string `json:"summary"`
					} `json:"reasoning"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if protocol == "responses" {
					request.Effort = request.Reasoning.Effort
					if request.Reasoning.Summary != nil {
						t.Error("disabled reasoning summary was sent upstream")
						w.WriteHeader(http.StatusUnprocessableEntity)
						return
					}
				}
				if request.Model != "custom-deployment" || request.Effort != expected.Load().(string) {
					t.Errorf("model=%s effort=%q, want %q", request.Model, request.Effort, expected.Load())
				}
				if protocol == "responses" {
					sendResponseText(w)
				} else {
					sendTextSSE(w, "Done.", true)
				}
			})
			c.Host.Config.Protocol = protocol
			c.Host.Config.Models["fixture"] = setting.Model{ID: "custom-deployment", SupportedReasoningLevels: []model.ReasoningEffort{"low", "high"}, DefaultReasoningSummary: "none"}
			factory, err := c.Host.Config.Factory()
			if err != nil {
				t.Fatal(err)
			}
			c.Host.Models = factory
			if err := c.ChangeModel(t.Context(), "fixture", "high"); err != nil {
				t.Fatal(err)
			}
			finish(t, c, prompt.Execute, "Agent effort")
			if err := c.ChangeSelection(t.Context(), "", "fixture", "low", prompt.Plan); err != nil {
				t.Fatal(err)
			}
			expected.Store("low")
			finish(t, c, prompt.Plan, "Plan effort")
			expected.Store("high")
			finish(t, c, prompt.Execute, "Agent effort still high")
		})
	}
}
