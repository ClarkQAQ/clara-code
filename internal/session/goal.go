package session

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"clara-code/pkg/model"
	"clara-code/pkg/tool"
	"clara-code/pkg/util/errs"
)

const (
	GoalUpdatedKind      = "goal.updated"
	GoalContinuationKind = model.MessageKindInternalTurn
	maxGoalMarkers       = 128
)

// GoalMarker is a recorded goal transition between visible turns.
type GoalMarker struct {
	Sequence  uint64
	AfterTurn int
	AfterFork bool `json:",omitempty"`
	Cell      Cell
}

func (s *Store) GoalState() tool.GoalState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Goal
}

func (s *Store) GoalMarkers() []GoalMarker {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.data.GoalMarkers)
}

// SaveGoal checkpoints the goal and its visible status transition together.
func (s *Store) SaveGoal(state tool.GoalState) (_ Cell, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	first := s.db == nil
	if first && (!s.draft || next.Meta.ID == "") {
		return Cell{}, errs.ErrConflict
	}
	if state == next.Goal {
		return Cell{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if first {
		if err := s.openWriter(ctx, next.Meta.ID); err != nil {
			return Cell{}, err
		}
		defer func() {
			if err != nil {
				err = errors.Join(err, s.closeLocked())
			}
		}()
	}
	cell := goalTransition(next.Goal, state)
	next.Goal = state
	if next.Meta.FirstPrompt == "" {
		next.Meta.FirstPrompt = excerpt(strings.Join(strings.Fields(state.Objective), " "), 240)
	}
	if cell.Kind != "" {
		cell = next.recordGoalTransition(cell)
	}
	if err := s.saveCheckpoint(ctx, next, first, turnChanges{}); err != nil {
		return Cell{}, err
	}
	return cell, nil
}

func (d *sessionData) recordGoalTransition(cell Cell) Cell {
	after := len(d.Turns)
	if d.Active != nil {
		turn := *d.Active
		turn.Cells = slices.Clone(turn.Cells)
		if saved := turn.append(cell.Kind, cell.Text); saved != nil {
			d.Active = &turn
			return *saved
		}
		after++
	}
	markers := d.GoalMarkers
	sequence := uint64(1)
	if len(markers) > 0 {
		sequence = markers[len(markers)-1].Sequence + 1
	}
	markers = markers[max(0, len(markers)-maxGoalMarkers+1):]
	d.GoalMarkers = append(slices.Clone(markers), GoalMarker{
		Sequence: sequence, AfterTurn: after, Cell: cell,
		AfterFork: d.Meta.ForkPoint != nil && d.Meta.ForkPoint.AfterTurn == after,
	})
	if d.Active != nil {
		// The terminal replays a boundary marker after the turn finishes.
		return Cell{}
	}
	return cell
}

func goalTransition(previous, state tool.GoalState) Cell {
	if state.Status == previous.Status && state.Objective == previous.Objective {
		return Cell{}
	}
	var words, detail string
	switch state.Status {
	case tool.GoalActive:
		if previous.Objective == state.Objective && (previous.Status == tool.GoalPaused || previous.Status == tool.GoalBlocked) {
			words = "Goal resumed"
		} else {
			words = "Goal started"
		}
	case tool.GoalPaused:
		words = "Goal paused"
	case tool.GoalCompleted:
		words, detail = "Goal completed", state.Summary
	case tool.GoalBlocked:
		words, detail = "Goal blocked", state.Blocker
	case tool.GoalExhausted:
		words = "Goal budget exhausted"
	default:
		return Cell{}
	}
	if detail != "" {
		words += " · " + detail
	}
	return Cell{Kind: GoalUpdatedKind, Text: words}
}
