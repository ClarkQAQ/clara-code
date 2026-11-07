package coding

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"clara-core/models"
	"clara-core/tools"

	"clara-code/internal/session"
)

type commandObservation struct {
	poll  tools.Poll
	valid bool
}

type observationKey struct{}

type observedRunner struct{ tools.Runner }
type observedProcess struct{ tools.Process }

func (r observedRunner) Start(ctx context.Context, spec tools.Spec) (tools.Process, error) {
	process, err := r.Runner.Start(ctx, spec)
	if err != nil {
		return nil, err
	}
	return observedProcess{process}, nil
}

func (p observedProcess) Poll(ctx context.Context, wait time.Duration) (tools.Poll, error) {
	poll, err := p.Process.Poll(ctx, wait)
	if observation, ok := ctx.Value(observationKey{}).(*commandObservation); ok && err == nil {
		observation.poll = poll
		observation.valid = true
	}
	return poll, err
}

type observedCommand struct{ models.AgentTool }

func observeCommand(tool models.AgentTool) models.AgentTool {
	return observedCommand{tool}
}

func (t observedCommand) Run(ctx context.Context, call models.ToolCall) (models.ToolResponse, error) {
	observation := &commandObservation{}
	result, err := t.AgentTool.Run(context.WithValue(ctx, observationKey{}, observation), call)
	if err != nil {
		return result, err
	}
	var input struct {
		SessionID string `json:"session_id"`
		Stop      bool   `json:"stop"`
	}
	_ = json.Unmarshal([]byte(call.Input), &input)
	if !observation.valid && (!input.Stop || result.IsError) {
		return result, nil
	}
	poll := observation.poll
	outcome := session.CommandOutcome{Running: poll.Running, DurationMS: poll.Duration.Milliseconds(), TimedOut: poll.TimedOut, SessionID: input.SessionID, Stopped: input.Stop}
	outcome.Output, outcome.Omitted = session.PreviewOutput(poll.Output, 16<<10)
	if observation.valid && !poll.Running {
		exitCode := poll.ExitCode
		outcome.ExitCode = &exitCode
	}
	if poll.Running && t.Info().Name == "exec_command" {
		line, _, _ := strings.Cut(result.Content, "\n")
		if id, ok := strings.CutPrefix(line, "session_id: "); ok && strings.HasPrefix(id, "exec_") {
			outcome.SessionID = id
		}
	}
	metadata := map[string]any{"clara_command": outcome}
	if result.Metadata != "" {
		metadata["upstream"] = result.Metadata
	}
	body, _ := json.Marshal(metadata)
	result.Metadata = string(body)
	return result, nil
}
