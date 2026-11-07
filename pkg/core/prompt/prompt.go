// Package prompt composes optional, replaceable instructions for a host.
package prompt

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"clara-code/pkg/core"
	"clara-code/pkg/models"
)

//go:embed *.md
var templates embed.FS

type Mode string

const (
	Execute Mode = "execute"
	Plan    Mode = "plan"
)

type Config struct {
	Mode      Mode
	Role      string
	Tools     []core.AgentTool
	Overrides map[string]string
}

func Default(name string) string {
	body, _ := templates.ReadFile(name + ".md")
	return strings.TrimSpace(string(body))
}

// System renders a stable prefix for the selected capabilities.
func System(config Config) string {
	has := map[string]bool{}
	for _, tool := range config.Tools {
		if tool != nil {
			has[tool.Info().Name] = true
		}
	}
	sections := []string{"base"}
	if has["glob"] || has["grep"] {
		sections = append(sections, "search")
	}
	if has["fs_instructions"] {
		sections = append(sections, "workspace_rules")
	}
	if has["read_skill"] {
		sections = append(sections, "skills")
	}
	if config.Mode == Plan {
		sections = append(sections, "plan")
		if has["request_user_input"] {
			sections = append(sections, "plan_questions")
		}
	} else {
		if has["read"] || has["exec_command"] {
			sections = append(sections, "coding")
		}
		if has["read"] && has["edit"] {
			sections = append(sections, "hashline")
		}
		if has["exec_command"] && has["write_stdin"] {
			sections = append(sections, "commands")
		}
		if has["update_plan"] {
			sections = append(sections, "checklist")
		}
		if has["update_goal"] {
			sections = append(sections, "goal")
		}
		if has["spawn_agent"] || has["subagent_task"] {
			sections = append(sections, "tasks")
		}
	}
	if strings.EqualFold(config.Role, "reviewer") {
		sections = append(sections, "review")
	}
	var out []string
	for _, section := range sections {
		body, ok := config.Overrides[section]
		if !ok {
			body = Default(section)
		}
		if body != "" {
			out = append(out, body)
		}
	}
	return strings.Join(out, "\n\n")
}

// Context adds current host state after the transcript.
type Context struct {
	Workspace string
	Rules     string
	Plan      string
	Goal      string
	Tasks     string
}

func (state Context) Message() models.Message {
	var out strings.Builder
	out.WriteString("# Current workspace state\n")
	for _, item := range []struct{ label, text string }{
		{"Workspace", state.Workspace}, {"Workspace instructions", state.Rules},
		{"Execution checklist", state.Plan}, {"Goal", state.Goal}, {"Child tasks", state.Tasks},
	} {
		if item.text != "" {
			fmt.Fprintf(&out, "\n## %s\n%s\n", item.label, item.text)
		}
	}
	return models.NewUserMessage(out.String())
}

// Prepare builds a hook that can be used with either Agent entry point.
func Prepare(config Config, current func(context.Context) (Context, error)) core.PrepareStepFunction {
	system := System(config)
	return func(ctx context.Context, step core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
		result := core.PrepareStepResult{System: &system}
		if current != nil {
			state, err := current(ctx)
			if err != nil {
				return ctx, result, err
			}
			result.Messages = append(step.Messages, state.Message())
		}
		return ctx, result, nil
	}
}
