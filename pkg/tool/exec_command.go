package tool

import (
	"clara-code/pkg/util/errs"
	"clara-code/pkg/util/id"
	"context"
	"errors"
	"fmt"
	"time"
)

func (s *Commands) ExecCommand() Tool {
	type input struct {
		Command        string `json:"command" description:"shell command"`
		Workdir        string `json:"workdir,omitempty" description:"working directory"`
		TimeoutSeconds int    `json:"timeout_seconds,omitempty" description:"total runtime limit in seconds; default and maximum 600"`
		YieldSeconds   int    `json:"yield_seconds,omitempty" description:"wait in seconds before returning a session_id; default 5, maximum 30"`
		TTY            bool   `json:"tty,omitempty" description:"request a terminal"`
	}
	tool := Func(Definition{
		Name: "exec_command",
		Description: "Run a shell command. A command still running after yield_seconds returns a session_id. " +
			"Use write_stdin to read more output, send input, or wait.",
		Effects: []Effect{EffectExecute}, Timeout: 10 * time.Minute,
	}, func(ctx context.Context, in input) (Result, error) {
		if s.Runner == nil {
			return Result{}, errs.ErrUnsupported
		}
		timeout := time.Duration(in.TimeoutSeconds) * time.Second
		noTimeout := s.NoDefaultTimeout && in.TimeoutSeconds == 0
		if in.TimeoutSeconds <= 0 || in.TimeoutSeconds > 600 {
			timeout = 10 * time.Minute
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return Result{}, errs.ErrConflict
		}
		if len(s.sessions)+s.starting >= 32 {
			s.mu.Unlock()
			return Result{}, errs.New(errs.CodeLimit, "too many active commands")
		}
		s.starting++
		s.mu.Unlock()
		defer func() { s.mu.Lock(); s.starting--; s.mu.Unlock() }()
		lifetime := s.Context
		if lifetime == nil {
			lifetime = context.WithoutCancel(ctx)
		}
		processCtx, cancel := context.WithCancel(lifetime)
		stop := context.AfterFunc(ctx, cancel)
		defer stop()
		keep := false
		defer func() {
			if !keep {
				cancel()
			}
		}()
		process, err := s.Runner.Start(processCtx, Spec{
			Command: in.Command, Workdir: in.Workdir, Timeout: timeout, NoTimeout: noTimeout, TTY: in.TTY,
		})
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		yield := time.Duration(in.YieldSeconds) * time.Second
		if yield <= 0 {
			yield = 5 * time.Second
		}
		if yield > 30*time.Second {
			yield = 30 * time.Second
		}
		poll, err := process.Poll(ctx, yield)
		if err != nil || !poll.Running {
			if err := errors.Join(err, process.Close()); err != nil {
				return Result{}, err
			}
			return pollResult("", poll), nil
		}
		if !stop() || processCtx.Err() != nil {
			return Result{}, errors.Join(ctx.Err(), processCtx.Err(), process.Close())
		}
		sessionID := id.New("exec")
		s.mu.Lock()
		if s.closed || lifetime.Err() != nil {
			s.mu.Unlock()
			return Result{}, errors.Join(errs.ErrConflict, lifetime.Err(), process.Close())
		}
		owned := &ownedProcess{Process: process, cancel: cancel}
		s.sessions[sessionID] = owned
		s.register(sessionID, in.Command, in.Workdir, owned, poll)
		s.mu.Unlock()
		keep = true
		_ = s.Refresh(context.WithoutCancel(ctx))
		return s.result(TextResult(fmt.Sprintf("session_id: %s\n%s", sessionID, renderPoll("", poll))), sessionID), nil
	})
	if s.NoDefaultTimeout {
		value := tool.(*optionalTool)
		description := "optional total runtime limit in seconds, up to 600; omission means no total deadline in this host"
		value.info.Parameters["timeout_seconds"].(map[string]any)["description"] = description
		value.info.InputSchema["properties"].(map[string]any)["timeout_seconds"].(map[string]any)["description"] = description
		value.info.Description += " Omitted timeout_seconds sets no total deadline in this host."
	}
	return tool
}
