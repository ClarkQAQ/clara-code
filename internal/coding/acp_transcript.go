package coding

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"clara-code/internal/session"
	wire "clara-code/pkg/appwire"
)

func attachmentView(a session.Attachment) wire.Attachment {
	return wire.Attachment{Hash: a.Hash, Filename: a.Filename, MediaType: a.MediaType, Width: a.Width, Height: a.Height, Size: a.Size}
}

func attachmentsView(items []session.Attachment) []wire.Attachment {
	var out []wire.Attachment
	for _, item := range items {
		out = append(out, attachmentView(item))
	}
	return out
}

func activityView(a session.ToolActivity) *wire.Activity {
	return &wire.Activity{
		ID: a.Key(), Name: a.Name, Title: a.Title, Detail: a.Detail, Args: a.Arguments,
		Output: a.Output, Error: a.Error, Status: a.Status, ExitCode: a.ExitCode, Omitted: a.Omitted,
	}
}

func turnView(t session.Turn, redact func(string) string) wire.Turn {
	status := t.Status
	if status == "" {
		status = "completed"
	}
	out := wire.Turn{ID: t.ID, Prompt: t.Prompt, Planning: t.Planning, Status: status, Cells: []wire.Cell{}, Attachments: attachmentsView(t.Attachments)}
	if !t.StartedAt.IsZero() && !t.FinishedAt.IsZero() {
		out.DurationMS = new(max(0, t.FinishedAt.Sub(t.StartedAt).Milliseconds()))
	}
	for _, c := range t.Cells {
		if c.Kind == session.ContextCompactedKind {
			continue
		}
		cell := wire.Cell{Kind: c.Kind, Text: c.Text, Attachments: attachmentsView(c.Attachments)}
		if c.Tool != nil {
			cell.Tool = activityView(*c.Tool)
		}
		if c.Kind == "error" {
			out.Status = "failed"
			if redact != nil {
				cell.Text = redact(cell.Text)
			}
		}
		if c.Kind == "notice" && (c.Text == "Interrupted" || strings.HasPrefix(c.Text, "Previous run interrupted")) {
			out.Status = "cancelled"
		}
		out.Cells = append(out.Cells, cell)
	}
	return out
}

func sessionView(meta session.Meta) wire.Session {
	return wire.Session{
		ID: meta.ID, Root: meta.Project, Cwd: meta.Cwd, Title: meta.FirstPrompt,
		Updated:   meta.Updated.Format(time.RFC3339Nano),
		Selection: wire.Selection{Provider: meta.Provider, Model: meta.Model, Effort: meta.Effort, PlanEffort: meta.PlanEffort},
	}
}

func (h Host) acpHistory(ctx context.Context, req wire.Request) (wire.History, error) {
	result := wire.History{Turns: []wire.Turn{}}
	meta, turns, before, err := session.HistoryPage(ctx, h.Config.Home, req.SessionID, req.Before)
	if err != nil {
		return result, err
	}
	result.Session, result.Before = sessionView(meta), before
	slices.Reverse(turns)
	for _, turn := range turns {
		result.Turns = append(result.Turns, turnView(turn, h.Config.Redact))
	}
	return result, nil
}

func (h Host) acpReadImage(req wire.Request) (wire.AttachmentChunk, error) {
	if len(req.Attachments) != 1 || req.Offset < 0 {
		return wire.AttachmentChunk{}, errors.New("invalid image request")
	}
	item := req.Attachments[0]
	file, err := (session.Attachment{Hash: item.Hash}).File(h.Config.Home)
	if err != nil {
		return wire.AttachmentChunk{}, err
	}
	if req.Offset > len(file.Data) {
		return wire.AttachmentChunk{}, errors.New("invalid image offset " + strconv.Itoa(req.Offset))
	}
	end := min(req.Offset+(256<<10), len(file.Data))
	next := end
	if end == len(file.Data) {
		next = -1
	}
	return wire.AttachmentChunk{Data: base64.StdEncoding.EncodeToString(file.Data[req.Offset:end]), Next: next}, nil
}
