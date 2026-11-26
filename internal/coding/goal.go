package coding

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"clara-code/internal/session"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	"clara-code/pkg/tool"
	"clara-code/pkg/util/errs"
)

var errSaveGoal = errors.New("saving goal failed")

func (c *Conversation) saveGoal(state tool.GoalState) error {
	cell, err := c.Store.SaveGoal(state)
	if err != nil {
		return fmt.Errorf("%w: %w", errSaveGoal, err)
	}
	if cell.Kind != "" {
		c.sessionMu.RLock()
		defer c.sessionMu.RUnlock()
		// The saved transcript is authoritative if the live event is missed.
		_ = c.Session.Emit(context.Background(), session.GoalUpdatedKind, cell)
	}
	return nil
}

func (c *Conversation) StartGoal(ctx context.Context, objective string, files []model.FilePart) (*core.Run, error) {
	c.startMu.Lock()
	defer c.startMu.Unlock()
	files, err := c.prepareGoalRun(ctx, files)
	if err != nil {
		return nil, err
	}
	objective = strings.TrimSpace(objective)
	if err := c.Goal.Start(objective, 0, 0); err != nil {
		return nil, err
	}
	return c.startGoalRun(ctx, model.NewUserMessage(objective, files...))
}

func (c *Conversation) ContinueGoal(ctx context.Context, files []model.FilePart) (*core.Run, error) {
	c.startMu.Lock()
	defer c.startMu.Unlock()
	files, err := c.prepareGoalRun(ctx, files)
	if err != nil {
		return nil, err
	}
	state := c.Goal.State()
	if state.Status != tool.GoalActive {
		if err := c.Goal.Resume(); err != nil {
			return nil, err
		}
	}
	return c.startGoalRun(ctx, goalContinuation("Continue the active goal: "+state.Objective, files))
}

func (c *Conversation) PauseGoal() error {
	return c.Goal.Pause()
}

func (c *Conversation) prepareGoalRun(ctx context.Context, files []model.FilePart) ([]model.FilePart, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Session.Active() != nil {
		return nil, errs.ErrBusy
	}
	return c.prepareFiles(ctx, files)
}

func (c *Conversation) startGoalRun(ctx context.Context, input model.Message) (*core.Run, error) {
	if err := c.compactRunHistory(ctx, prompt.Execute); err != nil {
		if !errors.Is(err, context.Canceled) {
			failure := session.DescribeFailure(err)
			err = errors.Join(err, c.Goal.Block(c.Host.Config.Redact(failure.Message)))
		}
		return nil, err
	}
	call := c.Call(prompt.Execute, false)
	call.Messages = []model.Message{input}
	return c.Session.Start(ctx, call)
}

func goalContinuation(text string, files []model.FilePart) model.Message {
	message := model.NewUserMessage(text, files...)
	message.Metadata = &model.MessageMetadata{Kind: model.MessageKindInternalTurn}
	return message
}
