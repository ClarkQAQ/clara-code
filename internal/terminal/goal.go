package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"clara-code/internal/session"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	"clara-code/pkg/tool"
)

func (a *app) startGoal(objective string) {
	if a.mode == prompt.Plan {
		a.notice(errors.New("switch to Agent mode before starting a goal"))
		return
	}
	if objective == "" {
		a.notice(errors.New("usage: /goal OBJECTIVE"))
		return
	}
	a.startTurn("/goal "+objective, objective, a.attachments, func(ctx context.Context, files []model.FilePart) (*core.Run, error) {
		run, err := a.conv.StartGoal(ctx, objective, files)
		if err != nil {
			return nil, fmt.Errorf("cannot start goal: %w", err)
		}
		return run, nil
	})
}

func (a *app) continueGoal() {
	if a.mode == prompt.Plan {
		a.notice(errors.New("switch to Agent mode before continuing a goal"))
		return
	}
	a.startTurn("/continue", "", a.attachments, a.conv.ContinueGoal)
}

func (a *app) goalState() tool.GoalState {
	if a.conv == nil {
		return tool.GoalState{}
	}
	return a.conv.Goal.State()
}

func (a *app) resumeSavedGoal() {
	if a.resumeGoalOnAgent && a.mode != prompt.Plan && a.run == nil {
		a.resumeGoalOnAgent = false
		if a.goalState().Status == tool.GoalActive {
			a.startTurn("", "", nil, a.conv.ContinueGoal)
		}
	}
}

func (a *app) cancelTurn() {
	a.question = nil
	a.run.Cancel()
}

func (a *app) persistGoalPause() {
	if err := a.conv.PauseGoal(); err != nil {
		a.notice(fmt.Errorf("cannot pause goal: %w", err))
	}
	a.syncGoalMarkers()
}

func (a *app) pauseGoal() {
	if a.goalState().Status != tool.GoalActive {
		a.notice(errors.New("no active goal"))
		return
	}
	a.persistGoalPause()
	if a.run != nil && a.mode != prompt.Plan {
		a.cancelTurn()
	}
}

func (a *app) syncGoalMarkers() {
	for _, marker := range a.conv.Store.GoalMarkers() {
		if marker.AfterTurn > a.loadedTurns {
			break
		}
		if marker.Sequence > a.goalMarkerSequence {
			a.view.cell(marker.Cell)
			a.goalMarkerSequence = marker.Sequence
		}
	}
}

func (a *app) showRunGoalMarker(cell session.Cell) {
	if slices.Contains(a.runGoalMarkers, cell.Text) {
		return
	}
	a.flushStream()
	a.view.cell(cell)
	a.runGoalMarkers = append(a.runGoalMarkers, cell.Text)
}
