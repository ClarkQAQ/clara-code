package coding

import (
	"context"
	"encoding/base64"
	"encoding/json"
	acp "github.com/coder/acp-go-sdk"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"clara-code/internal/setting"
	wire "clara-code/pkg/appwire"
)

func optionsView(config setting.Config, selection wire.Selection, setup bool) wire.Options {
	out := wire.Options{
		Models: []wire.Model{}, Selection: selection,
		ConfigPath: filepath.Join(config.Home, "config.toml"), NeedsSetup: setup,
	}
	providers := make([]string, 0, len(config.Providers))
	for provider := range config.Providers {
		providers = append(providers, provider)
	}
	if len(providers) == 0 {
		providers = append(providers, config.ModelProvider)
	}
	slices.Sort(providers)
	for _, provider := range providers {
		current := config
		if provider != config.ModelProvider {
			var err error
			current, err = config.SelectProvider(provider)
			if err != nil {
				continue
			}
		}
		models := make([]string, 0, len(current.Models))
		for name, m := range current.Models {
			if m.Visibility != "hidden" || (name == selection.Model && provider == selection.Provider) {
				models = append(models, name)
			}
		}
		slices.Sort(models)
		for _, name := range models {
			m := current.Models[name]
			label := m.DisplayName
			if label == "" {
				label = name
			}
			providerLabel := current.Connection().Name
			if providerLabel == "" {
				providerLabel = provider
			}
			levels := []string{""}
			for _, effort := range m.Levels() {
				if effort != "" {
					levels = append(levels, string(effort))
				}
			}
			plan := current.PlanEffort
			if plan == "" || !m.Allows(plan) {
				plan = m.Effort
			}
			out.Models = append(out.Models, wire.Model{
				Provider: provider, ProviderLabel: providerLabel, ID: name, Label: label, Description: m.Description,
				Efforts: levels, Default: string(m.Effort), DefaultPlan: string(plan), Images: m.Images(),
			})
			if out.Selection.Model == "" && provider == config.ModelProvider && name == config.DefaultModel {
				out.Selection = wire.Selection{Provider: provider, Model: name, Effort: string(m.Effort), PlanEffort: string(plan)}
			}
		}
	}
	return out
}

type acpConversation struct {
	mu   sync.Mutex
	chat *Conversation
	mode string
}

func configSelect(id, name, category, current string, options []acp.SessionConfigSelectOption) acp.SessionConfigOption {
	cat := acp.SessionConfigOptionCategory(category)
	return acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
		Id: acp.SessionConfigId(id), Name: name, Type: "select", Category: &cat,
		CurrentValue: acp.SessionConfigValueId(current),
		Options:      acp.SessionConfigSelectOptions{Ungrouped: new(acp.SessionConfigSelectOptionsUngrouped(options))},
	}}
}

func (e *acpConversation) options() []acp.SessionConfigOption {
	selection := sessionView(e.chat.Store.Meta()).Selection
	data := optionsView(e.chat.CurrentConfig(), selection, false)
	modes := []acp.SessionConfigSelectOption{{Value: "agent", Name: "Agent"}, {Value: "plan", Name: "Plan"}}
	if e.chat.Workspace.Git {
		modes = append(modes, acp.SessionConfigSelectOption{Value: "review", Name: "Read-only review"})
	}
	out := []acp.SessionConfigOption{configSelect("mode", "Mode", "mode", e.currentMode(), modes)}
	var models, agent, plan []acp.SessionConfigSelectOption
	for _, m := range data.Models {
		name := m.Label
		if m.ProviderLabel != "" {
			name += " · " + m.ProviderLabel
		}
		models = append(models, acp.SessionConfigSelectOption{
			Value: acp.SessionConfigValueId(modelValue(m.Provider, m.ID)), Name: name, Description: new(m.Description),
			Meta: map[string]any{wire.Namespace + "images": m.Images},
		})
		if m.Provider != selection.Provider || m.ID != selection.Model {
			continue
		}
		for _, effort := range m.Efforts {
			label := effort
			if label == "" {
				label = "Provider default"
			}
			a := acp.SessionConfigSelectOption{Value: acp.SessionConfigValueId(effortValue(effort)), Name: label}
			p := a
			a.Meta = map[string]any{wire.Namespace + "default": effort == m.Default}
			p.Meta = map[string]any{wire.Namespace + "default": effort == m.DefaultPlan}
			agent, plan = append(agent, a), append(plan, p)
		}
	}
	if len(models) > 0 {
		out = append(out, configSelect("model", "Model", "model", modelValue(selection.Provider, selection.Model), models))
	}
	if len(agent) > 0 {
		out = append(out, configSelect("agent_effort", "Agent effort", "thought_level", effortValue(selection.Effort), agent))
		out = append(out, configSelect("plan_effort", "Plan effort", "thought_level", effortValue(selection.PlanEffort), plan))
	}
	return out
}

func modelValue(provider, name string) string {
	value, _ := json.Marshal([2]string{provider, name})
	return base64.RawURLEncoding.EncodeToString(value)
}

func effortValue(effort string) string {
	if effort == "" {
		return "default"
	}
	return "effort:" + effort
}

func (e *acpConversation) setOption(ctx context.Context, req acp.SetSessionConfigOptionRequest) error {
	item := req.ValueId
	valid := false
	for _, option := range e.options() {
		if option.Select.Id != item.ConfigId {
			continue
		}
		valid = slices.ContainsFunc(*option.Select.Options.Ungrouped, func(o acp.SessionConfigSelectOption) bool { return o.Value == item.Value })
	}
	if !valid {
		return acp.NewInvalidParams("unknown configuration option or value")
	}
	value := string(item.Value)
	if value == "default" {
		value = ""
	}
	meta := e.chat.Store.Meta()
	switch item.ConfigId {
	case "mode":
		e.mu.Lock()
		e.mode = value
		e.mu.Unlock()
		return nil
	case "model":
		data, _ := base64.RawURLEncoding.DecodeString(value)
		var target [2]string
		if json.Unmarshal(data, &target) != nil {
			return acp.NewInvalidParams("invalid model")
		}
		for _, m := range optionsView(e.chat.CurrentConfig(), sessionView(meta).Selection, false).Models {
			if m.Provider == target[0] && m.ID == target[1] {
				return e.chat.ChangeReasoningSelection(ctx, m.Provider, m.ID, m.Default, m.DefaultPlan)
			}
		}
		return acp.NewInvalidParams("model is unavailable")
	case "agent_effort":
		meta.Effort = strings.TrimPrefix(value, "effort:")
	case "plan_effort":
		meta.PlanEffort = strings.TrimPrefix(value, "effort:")
	}
	return e.chat.ChangeReasoningSelection(ctx, meta.Provider, meta.Model, meta.Effort, meta.PlanEffort)
}

func (e *acpConversation) currentMode() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.mode
}
