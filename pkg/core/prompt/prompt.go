// Package prompt composes optional, replaceable instructions for a host.
package prompt

import (
	"context"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
)

//go:embed *.md
var templates embed.FS

type Mode string

const (
	Execute Mode = "execute"
	Plan    Mode = "plan"
)

type Config struct {
	Mode       Mode
	Role       string
	Tools      []core.AgentTool
	Overrides  map[string]string
	Parameters *Parameters
}

// Parameters describe the host's actual tool semantics.
type Parameters struct {
	Paths            string
	ExamplePath      string
	ExampleHeader    string
	ExecYieldSeconds int
	ExecTimeout      string
}

func (config Config) parameters() Parameters {
	p := Parameters{Paths: "logical workspace paths", ExamplePath: "/main.go", ExampleHeader: "/main.go", ExecYieldSeconds: 5, ExecTimeout: "defaults to 600 and has a maximum of 600"}
	if config.Parameters != nil {
		p = *config.Parameters
	}
	return p
}

func render(name string, config Config) string {
	if body, ok := config.Overrides[name]; ok {
		return body
	}
	var out strings.Builder
	t := template.Must(template.New(name).Parse(templateText(name)))
	if err := t.Execute(&out, config.parameters()); err != nil {
		panic(err)
	}
	return out.String()
}

func Default(name string) string {
	return render(name, Config{})
}

func templateText(name string) string {
	body, _ := templates.ReadFile(name + ".md")
	return strings.TrimSpace(string(body))
}

// System renders a stable prefix for the selected capabilities.
func System(config Config) string {
	var out []string
	for _, section := range sections(config) {
		if body := render(section, config); body != "" {
			out = append(out, body)
		}
	}
	return strings.Join(out, "\n\n")
}

func sections(config Config) []string {
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
	return sections
}

// Context adds current host state after the transcript.
type Context struct {
	Workspace string
	Rules     string
	Plan      string
	Goal      string
	Tasks     string
}

func (state Context) Message() model.Message {
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
	return model.NewUserMessage(out.String())
}

// Prepare builds a hook that can be used with either Agent entry point.
func Prepare(config Config, current func(context.Context) (Context, error)) core.PrepareStepFunction {
	system, _ := Instructions(config)
	return func(ctx context.Context, step core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
		result := core.PrepareStepResult{System: &system}
		fragments := Policy(config)
		if current != nil {
			state, err := current(ctx)
			if err != nil {
				return ctx, result, err
			}
			rules := state.Rules
			state.Rules = ""
			fragments = append(fragments, []Fragment{
				{Kind: "workspace_instructions", Source: "host", Scope: state.Workspace, Lifecycle: "step", Role: model.MessageRoleUser, Text: rules},
				{Kind: "workspace_state", Source: "host", Lifecycle: "step", Role: model.MessageRoleUser, Text: state.Message().Content[0].(model.TextPart).Text},
			}...)
		}
		window, err := LoadWindow(step.ContextState)
		if err != nil {
			return ctx, result, err
		}
		messages, next, err := window.Prepare(step.Messages, fragments)
		if err != nil {
			return ctx, result, err
		}
		result.Messages = messages
		result.ContextState, err = next.Marshal()
		if err != nil {
			return ctx, result, err
		}
		return ctx, result, nil
	}
}
