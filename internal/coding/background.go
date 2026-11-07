package coding

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"clara-code/internal/session"
	"clara-code/pkg/model"
	toolapi "clara-code/pkg/tool"
)

type trackedCommand struct {
	model.AgentTool
	conversation *Conversation
	owner        string
}

func (c *Conversation) command(tool model.AgentTool, owner string) model.AgentTool {
	return trackedCommand{AgentTool: observeCommand(tool), conversation: c, owner: owner}
}

func (t trackedCommand) Run(ctx context.Context, call model.ToolCall) (model.ToolResponse, error) {
	c := t.conversation
	prefix := "task:"
	if active := c.Session.Active(); t.owner == "main" && active != nil {
		prefix = active.ID + ":"
	}
	key := prefix + strconv.FormatUint(c.commandSequence.Add(1), 10) + ":" + call.ID
	content := model.ToolCallContent{ToolCallID: call.ID, ToolName: t.Info().Name, Input: call.Input}
	if content.ToolCallID == "" {
		content.ToolCallID = key
	}
	if err := c.activities.startKey(content, key, t.owner); err != nil {
		return model.ToolResponse{}, err
	}
	response, err := t.AgentTool.Run(ctx, call)
	if err != nil {
		c.activities.failKey(key, err)
		return response, err
	}
	result := model.ToolResultContent{ToolCallID: content.ToolCallID, ToolName: content.ToolName, Result: model.ToolResultOutputContentText{Text: response.Content}, ClientMetadata: response.Metadata}
	if response.IsError {
		result.Result = model.ToolResultOutputContentError{Error: errors.New(response.Content)}
	}
	if err := c.activities.completeKey(result, key); err != nil {
		return response, err
	}
	for _, terminal := range c.commands.Terminals() {
		c.onTerminal(terminal)
	}
	return response, nil
}

func (c *Conversation) publishActivity(activity session.ToolActivity) error {
	var err error
	if activity.Owner == "" || activity.Owner == "main" {
		c.sessionMu.RLock()
		err = c.Session.Emit(context.Background(), "tool.activity", activity)
		c.sessionMu.RUnlock()
	}
	if activity.Name == "exec_command" || activity.Name == "write_stdin" {
		err = errors.Join(err, c.Store.UpdateCommand(activity))
		c.notifyBackground()
	}
	return err
}

func (c *Conversation) notifyBackground() {
	if c.backgroundUpdates == nil {
		return
	}
	select {
	case c.backgroundUpdates <- struct{}{}:
	default:
	}
}

func (c *Conversation) onTerminal(info toolapi.TerminalInfo) {
	if activity, changed := c.activities.terminal(info); changed {
		if err := c.Store.UpdateCommand(activity); err != nil {
			c.setBackgroundError(err)
		}
	}
	c.notifyBackground()
}

func (c *Conversation) setBackgroundError(err error) {
	c.backgroundMu.Lock()
	c.backgroundErr = err
	c.backgroundMu.Unlock()
	c.notifyBackground()
}

func (c *Conversation) BackgroundError() error {
	c.backgroundMu.Lock()
	defer c.backgroundMu.Unlock()
	err := c.backgroundErr
	c.backgroundErr = nil
	return err
}

func (c *Conversation) watchTerminals(ctx context.Context) {
	defer close(c.monitorDone)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.commands.Refresh(ctx); err != nil && ctx.Err() == nil {
				c.setBackgroundError(err)
			}
		}
	}
}

func (c *Conversation) BackgroundUpdates() <-chan struct{} {
	return c.backgroundUpdates
}

func (c *Conversation) Terminals() []toolapi.TerminalInfo {
	if c.commands == nil {
		return nil
	}
	infos := c.commands.Terminals()
	for i := range infos {
		if c.Workspace.Native != nil {
			if cwd, err := c.Workspace.Native.ResolvePath(infos[i].Workdir); err == nil {
				infos[i].Workdir = cwd.String()
			}
		}
	}
	return infos
}

func (c *Conversation) StopTerminals(ctx context.Context, id string) error {
	if c.commands == nil {
		return fmt.Errorf("no background terminals")
	}
	return c.commands.Stop(ctx, id)
}

func (c *Conversation) terminalSummaries() []map[string]any {
	var out []map[string]any
	for _, terminal := range c.Terminals() {
		command, _ := session.PreviewOutput(terminal.Command, 800)
		out = append(out, map[string]any{"session_id": terminal.ID, "command": command, "status": terminal.Status, "exit_code": terminal.Poll.ExitCode, "automatic_updates": terminal.Observable})
	}
	return out
}
