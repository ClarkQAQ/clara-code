package tools

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"time"

	"clara-code/pkg/util/errs"
)

// SnapshotProcess provides non-consuming status and retained output.
// OutputOffset counts bytes discarded before Output by the bounded capture.
type SnapshotProcess interface {
	Snapshot(context.Context) (Poll, error)
}

// TerminalInfo is an immutable snapshot of one detached command.
type TerminalInfo struct {
	ID         string
	Command    string
	Workdir    string
	Started    time.Time
	Status     string
	Poll       Poll
	Observable bool
	Error      string
	Revision   uint64
}

type terminalRecord struct {
	stopping bool
	info     TerminalInfo
	process  Process
}

func commandDuration(started, finished time.Time) time.Duration {
	if !finished.IsZero() {
		return finished.Sub(started)
	}
	return time.Since(started)
}

func (c *capture) snapshot() (string, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.snapshotValid {
		c.snapshotValue = string(c.data)
		c.snapshotValid = true
	}
	return c.snapshotValue, c.dropped
}

func (p *localProcess) Snapshot(ctx context.Context) (Poll, error) {
	if err := ctx.Err(); err != nil {
		return Poll{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	output, offset := p.output.snapshot()
	return Poll{Output: output, OutputOffset: offset, Running: !p.finished, ExitCode: p.exitCode, TimedOut: p.timedOut, Duration: commandDuration(p.started, p.finishedAt)}, errors.Join(p.waitErr, p.killErr)
}

func (p *sshProcess) Snapshot(ctx context.Context) (Poll, error) {
	if err := ctx.Err(); err != nil {
		return Poll{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	output, offset := p.output.snapshot()
	err := p.waitErr
	if p.timedOut {
		err = nil
	}
	return Poll{Output: output, OutputOffset: offset, Running: !p.finished, ExitCode: p.exitCode, TimedOut: p.timedOut, Duration: commandDuration(p.started, p.finishedAt)}, err
}

func snapshotProcess(process Process) (SnapshotProcess, bool) {
	if owned, ok := process.(*ownedProcess); ok {
		process = owned.Process
	}
	observable, ok := process.(SnapshotProcess)
	return observable, ok
}

func (s *Commands) register(id, command, workdir string, process Process, poll Poll) {
	_, observable := snapshotProcess(process)
	s.records[id] = &terminalRecord{process: process, info: TerminalInfo{ID: id, Command: command, Workdir: workdir, Started: time.Now().Add(-poll.Duration), Status: "running", Poll: poll, Observable: observable}}
}

// Terminals returns running and retained completed snapshots, newest first.
func (s *Commands) Terminals() []TerminalInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TerminalInfo, 0, len(s.records))
	for _, record := range s.records {
		out = append(out, record.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
	return out
}

func (s *Commands) terminal(id string) (TerminalInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return TerminalInfo{}, false
	}
	return record.info, true
}

func (s *Commands) update(id string, poll Poll, err error, stopped bool) {
	s.mu.Lock()
	record, ok := s.records[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	previous := record.info
	next := previous
	if previous.Status != "running" {
		s.mu.Unlock()
		return
	}
	next.Poll = poll
	next.Status = "running"
	if !poll.Running {
		next.Status = "succeeded"
		if poll.ExitCode != 0 || err != nil {
			next.Status = "failed"
		}
		if poll.TimedOut {
			next.Status = "timed_out"
		}
		if stopped || record.stopping {
			next.Status = "stopped"
		}
	}
	if err != nil {
		next.Error = err.Error()
	}
	// A changing elapsed duration alone does not require a notification.
	changed := next
	changed.Poll.Duration = previous.Poll.Duration
	if reflect.DeepEqual(previous, changed) {
		s.mu.Unlock()
		return
	}
	next.Revision++
	record.info = next
	if next.Status != "running" {
		delete(s.sessions, id)
		s.completed = append(s.completed, id)
		if len(s.completed) > 32 {
			delete(s.records, s.completed[0])
			s.completed = s.completed[1:]
		}
	}
	publish := s.OnChange
	s.mu.Unlock()
	if publish != nil {
		publish(next)
	}
}

// Refresh observes detached processes without consuming write_stdin output.
func (s *Commands) Refresh(ctx context.Context) error {
	s.mu.Lock()
	processes := make(map[string]Process, len(s.sessions))
	for id, process := range s.sessions {
		processes[id] = process
	}
	s.mu.Unlock()
	var failures error
	for id, process := range processes {
		if err := ctx.Err(); err != nil {
			return err
		}
		snapshot, ok := snapshotProcess(process)
		if !ok {
			continue
		}
		poll, err := snapshot.Snapshot(ctx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		s.update(id, poll, err, false)
		if !poll.Running {
			failures = errors.Join(failures, process.Close())
		}
	}
	return failures
}

// Stop closes one detached process. Empty id stops the current running set.
func (s *Commands) Stop(ctx context.Context, id string) error {
	s.mu.Lock()
	processes := map[string]Process{}
	if id != "" {
		if process, ok := s.sessions[id]; ok {
			processes[id] = process
			s.records[id].stopping = true
		} else if _, ok := s.records[id]; !ok {
			s.mu.Unlock()
			return errs.ErrNotFound
		}
	} else {
		for key, process := range s.sessions {
			processes[key] = process
			s.records[key].stopping = true
		}
	}
	s.mu.Unlock()
	var err error
	for key, process := range processes {
		if ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		}
		err = errors.Join(err, s.stopProcess(ctx, key, process))
	}
	return err
}

func (s *Commands) stopProcess(ctx context.Context, id string, process Process) error {
	s.mu.Lock()
	if record := s.records[id]; record != nil {
		record.stopping = true
	}
	s.mu.Unlock()
	err := process.Close()
	poll := Poll{Running: false, ExitCode: -1}
	if snapshot, ok := snapshotProcess(process); ok {
		current, snapshotErr := snapshot.Snapshot(context.WithoutCancel(ctx))
		poll = current
		err = errors.Join(err, snapshotErr)
	}
	if _, observable := snapshotProcess(process); !observable {
		poll.Running = false
	}
	s.update(id, poll, err, true)
	return err
}

func (s *Commands) result(result Result, id string) Result {
	info, ok := s.terminal(id)
	if ok {
		data, _ := json.Marshal(map[string]any{"terminal": info})
		result.Metadata = string(data)
	}
	return result
}

func (s *Commands) consumeCompleted(id string) (Result, bool) {
	s.mu.Lock()
	record, ok := s.records[id]
	if !ok || record.info.Status == "running" {
		s.mu.Unlock()
		return Result{}, false
	}
	info := record.info
	process := record.process
	delete(s.records, id)
	s.completed = slices.DeleteFunc(s.completed, func(value string) bool { return value == id })
	s.mu.Unlock()
	// Completed processes retain their unread capture cursor until collected.
	poll, err := process.Poll(context.Background(), time.Nanosecond)
	if err != nil {
		return ErrorResult(err.Error()), true
	}
	result := pollResult("session completed\n", poll)
	if info.Status == "stopped" {
		result = pollResult("Command stopped\n", poll)
		result.IsError = false
	}
	result.Metadata = infoMetadata(info)
	return result, true
}

func infoMetadata(info TerminalInfo) string {
	data, _ := json.Marshal(map[string]any{"terminal": info})
	return string(data)
}
