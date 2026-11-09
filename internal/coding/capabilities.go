package coding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"clara-code/internal/session"
	"clara-code/internal/setting"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	"clara-code/pkg/tool"
)

const omittedImage = "[Image content omitted because this model does not support image input.]"

type capabilityModel struct {
	model.LanguageModel
	profile setting.Model
}

func (m capabilityModel) Capabilities() setting.Model { return m.profile }
func (m capabilityModel) options(call model.Call) model.Call {
	if !m.profile.Images() {
		call.Prompt = withoutImages(call.Prompt)
	}
	if call.MaxOutputTokens == nil && m.profile.MaxOutputTokens > 0 {
		call.MaxOutputTokens = new(m.profile.MaxOutputTokens)
	}
	if call.ProviderOptions.ParallelToolCalls == nil && m.profile.SupportsParallelToolCalls != nil {
		call.ProviderOptions.ParallelToolCalls = m.profile.SupportsParallelToolCalls
	}
	if call.ProviderOptions.ReasoningSummary == nil && m.profile.DefaultReasoningSummary != "" {
		call.ProviderOptions.ReasoningSummary = new(m.profile.DefaultReasoningSummary)
	}
	if call.ProviderOptions.ReasoningSummary != nil && *call.ProviderOptions.ReasoningSummary == "none" {
		call.ProviderOptions.ReasoningSummary = nil
	}
	if call.ProviderOptions.TextVerbosity == nil && m.profile.DefaultVerbosity != "" {
		call.ProviderOptions.TextVerbosity = new(m.profile.DefaultVerbosity)
	}
	return call
}
func (m capabilityModel) Generate(ctx context.Context, call model.Call) (*model.Response, error) {
	return m.LanguageModel.Generate(ctx, m.options(call))
}
func (m capabilityModel) Stream(ctx context.Context, call model.Call) (model.StreamResponse, error) {
	return m.LanguageModel.Stream(ctx, m.options(call))
}
func profileOf(m model.LanguageModel) setting.Model {
	if m, ok := m.(interface{ Capabilities() setting.Model }); ok {
		return m.Capabilities()
	}
	return setting.Model{}
}
func withoutImages(messages []model.Message) []model.Message {
	out := session.SummaryMessages(messages)
	for i, message := range messages {
		for j, part := range message.Content {
			if file, ok := model.AsMessagePart[model.FilePart](part); ok && strings.HasPrefix(file.MediaType, "image/") {
				out[i].Content[j] = model.TextPart{Text: omittedImage + " [Image: " + file.Filename + "]"}
			}
			if result, ok := model.AsMessagePart[model.ToolResultPart](part); ok {
				if media, ok := model.AsToolResultOutputType[model.ToolResultOutputContentMedia](result.Output); ok && strings.HasPrefix(media.MediaType, "image/") {
					result.Output = model.ToolResultOutputContentText{Text: omittedImage + " " + media.Text}
					out[i].Content[j] = result
				}
			}
		}
	}
	return out
}
func (c *Conversation) SupportsImages() bool {
	return c.CurrentConfig().Models[c.Store.Meta().Model].Images()
}
func (c *Conversation) imageTool(profile setting.Model) model.AgentTool {
	inner := tool.NewImages(c.Workspace.toolFS(), imageOptions(c.Host.Config.Images.Defaults())).View()
	return imageCapability{AgentTool: inner, enabled: profile.Images()}
}

type imageCapability struct {
	model.AgentTool
	enabled bool
}

func (t imageCapability) Run(ctx context.Context, call model.ToolCall) (model.ToolResponse, error) {
	if !t.enabled {
		return model.NewTextErrorResponse("image_view is not available because this model does not support images"), nil
	}
	return t.AgentTool.Run(ctx, call)
}

type configuredTool struct {
	model.AgentTool
	timeout  time.Duration
	parallel bool
	exec     setting.Exec
}

func (t configuredTool) Info() model.ToolInfo {
	info := t.AgentTool.Info()
	if t.timeout > 0 {
		info.Timeout = t.timeout
	}
	if !t.parallel {
		info.Parallel = false
	}
	if info.Name == "exec_command" {
		info.Description = "Run a shell command. A command still running after yield_seconds returns a session_id. Use write_stdin to read more output, send input, or wait. timeout_seconds " + commandTimeout(t.exec) + "."
		info.Parameters = maps.Clone(info.Parameters)
		defaults := map[string]string{"timeout_seconds": "total runtime limit in seconds; omitted: unlimited; maximum 600"}
		if t.exec.TimeoutSeconds != nil {
			defaults["timeout_seconds"] = fmt.Sprintf("total runtime limit in seconds; default %d, maximum 600", *t.exec.TimeoutSeconds)
		}
		if t.exec.YieldSeconds > 0 {
			defaults["yield_seconds"] = fmt.Sprintf("wait in seconds before returning a session_id; default %d, maximum 30", t.exec.YieldSeconds)
		}
		for key, description := range defaults {
			if property, ok := info.Parameters[key].(map[string]any); ok {
				property = maps.Clone(property)
				property["description"] = description
				info.Parameters[key] = property
			}
		}
		if info.InputSchema != nil {
			info.InputSchema = maps.Clone(info.InputSchema)
			info.InputSchema["properties"] = info.Parameters
		}
	}
	return info
}
func (t configuredTool) Run(ctx context.Context, call model.ToolCall) (model.ToolResponse, error) {
	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}
	if t.Info().Name == "exec_command" && (t.exec.YieldSeconds > 0 || t.exec.TimeoutSeconds != nil) {
		var args map[string]json.RawMessage
		if json.Unmarshal([]byte(call.Input), &args) == nil && args != nil {
			if _, ok := args["yield_seconds"]; !ok && t.exec.YieldSeconds > 0 {
				args["yield_seconds"], _ = json.Marshal(t.exec.YieldSeconds)
			}
			if _, ok := args["timeout_seconds"]; !ok && t.exec.TimeoutSeconds != nil {
				args["timeout_seconds"], _ = json.Marshal(*t.exec.TimeoutSeconds)
			}
			data, err := json.Marshal(args)
			if err != nil {
				return model.ToolResponse{}, err
			}
			call.Input = string(data)
		}
	}
	return t.AgentTool.Run(ctx, call)
}
func (c *Conversation) configureTools(items []model.AgentTool, profile setting.Model) []model.AgentTool {
	out := make([]model.AgentTool, 0, len(items))
	for _, item := range items {
		info := item.Info()
		if info.Name == "image_view" && !profile.Images() {
			continue
		}
		n := c.Host.Config.Tools.DefaultTimeoutSeconds
		if value, ok := c.Host.Config.Tools.Timeouts[info.Name]; ok {
			n = value
		}
		for name, m := range c.Host.Config.MCP {
			if strings.HasPrefix(info.Name, "mcp__"+name+"__") && m.ToolTimeoutSec > 0 {
				n = m.ToolTimeoutSec
			}
		}
		parallel := profile.SupportsParallelToolCalls == nil || *profile.SupportsParallelToolCalls
		out = append(out, configuredTool{AgentTool: item, timeout: time.Duration(n) * time.Second, parallel: parallel, exec: c.Host.Config.Tools.Exec})
	}
	return out
}
func (c *Conversation) ModeEffort(mode prompt.Mode) string {
	meta := c.Store.Meta()
	if mode == prompt.Plan && meta.PlanEffort != "" {
		return meta.PlanEffort
	}
	return meta.Effort
}
func (c *Conversation) ChangeSelection(ctx context.Context, provider, name, effort string, mode prompt.Mode) error {
	if c.Session.Active() != nil {
		return errors.New("finish or cancel the current turn before switching models")
	}
	config := c.CurrentConfig()
	if provider != "" && len(config.Providers) > 0 {
		var err error
		config, err = config.SelectProvider(provider)
		if err != nil {
			return err
		}
	}
	profile, ok := config.Models[name]
	if !ok {
		return errors.New("model is not configured")
	}
	if !profile.Allows(model.ReasoningEffort(effort)) {
		return errors.New("reasoning effort is not supported by this model")
	}
	meta := c.Store.Meta()
	agent, plan := meta.Effort, meta.PlanEffort
	if !profile.Allows(model.ReasoningEffort(agent)) {
		agent = string(profile.Effort)
	}
	if !profile.Allows(model.ReasoningEffort(plan)) {
		plan = string(config.PlanEffort)
		if !profile.Allows(model.ReasoningEffort(plan)) {
			plan = string(profile.Effort)
		}
	}
	if mode == prompt.Plan {
		plan = effort
	} else {
		agent = effort
	}
	return c.setReasoningSelection(ctx, config, name, agent, plan)
}

// ChangeReasoningSelection commits Agent and Plan settings together.
func (c *Conversation) ChangeReasoningSelection(ctx context.Context, provider, name, agent, plan string) error {
	if c.Session.Active() != nil {
		return errors.New("finish or cancel the current turn before switching models")
	}
	config := c.CurrentConfig()
	if provider != "" && len(config.Providers) > 0 {
		var err error
		config, err = config.SelectProvider(provider)
		if err != nil {
			return err
		}
	}
	return c.setReasoningSelection(ctx, config, name, agent, plan)
}

func (c *Conversation) setReasoningSelection(ctx context.Context, config setting.Config, name, agent, plan string) error {
	profile, ok := config.Models[name]
	if !ok {
		return errors.New("model is not configured")
	}
	if !profile.Allows(model.ReasoningEffort(agent)) || !profile.Allows(model.ReasoningEffort(plan)) {
		return errors.New("agent or plan reasoning effort is not supported by this model")
	}
	h := c.Host
	h.Config = config
	next, err := h.model(ctx, name, agent)
	if err != nil {
		return err
	}
	if err := c.Store.SetSelection(config.ModelProvider, name, agent, plan); err != nil {
		return err
	}
	c.model = next
	return nil
}

func (c *Conversation) CurrentConfig() setting.Config {
	config := c.Host.Config
	if provider := c.Store.Meta().Provider; provider != "" && provider != config.ModelProvider {
		if selected, err := config.SelectProvider(provider); err == nil {
			return selected
		}
	}
	return config
}
func (c *Conversation) currentModel(ctx context.Context, name, effort string) (model.LanguageModel, error) {
	h := c.Host
	h.Config = c.CurrentConfig()
	return h.model(ctx, name, effort)
}
