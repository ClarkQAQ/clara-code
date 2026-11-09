package coding

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"clara-code/internal/session"
	"clara-code/internal/setting"
	wire "clara-code/pkg/appwire"
	"clara-code/pkg/core"
	"clara-code/pkg/model"
	acp "github.com/coder/acp-go-sdk"
)

func hasClaraExtension(c acp.ClientCapabilities) bool {
	switch v := c.Meta[wire.Capability].(type) {
	case float64:
		return v == wire.Version
	case int:
		return v == wire.Version
	}
	return false
}

func (h Host) acpExtension(ctx context.Context, server *core.ACPServer, live *sync.Map, method string, data json.RawMessage) (any, error) {
	if !hasClaraExtension(server.ClientCapabilities()) || !strings.HasPrefix(method, wire.Prefix) {
		return nil, acp.NewMethodNotFound(method)
	}
	var req wire.Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, acp.NewInvalidParams("invalid extension request")
	}
	switch strings.TrimPrefix(method, wire.Prefix) {
	case "session/history":
		return h.acpHistory(ctx, req)
	case "session/steer":
		value, ok := live.Load(req.SessionID)
		if !ok {
			return nil, acpSessionNotFound()
		}
		chat := value.(*acpConversation).chat
		run := chat.Session.Active()
		if run == nil || (req.RunID != "" && req.RunID != run.ID) {
			return nil, acp.NewInvalidParams("turn is no longer active")
		}
		if strings.TrimSpace(req.Text) == "" && len(req.Attachments) == 0 || len(req.Text) > 256<<10 {
			return nil, acp.NewInvalidParams("invalid steering input")
		}
		config := chat.CurrentConfig()
		files, err := AttachmentFiles(config.Home, attachmentRefs(req.Attachments), config.Images)
		if err != nil {
			return nil, err
		}
		return chat.QueueFiles(ctx, run, req.Text, false, files)
	case "attachment/read":
		return h.acpReadImage(req)
	case "project/resolve", "config/get", "config/setup", "attachment/import", "git/diff":
	default:
		return nil, acp.NewMethodNotFound(method)
	}
	if !filepath.IsAbs(req.Cwd) {
		return nil, acp.NewInvalidParams("workspace path must be absolute")
	}
	workspace, err := ResolveWorkspace(req.Cwd)
	if err != nil {
		return nil, err
	}
	switch method {
	case wire.Prefix + "project/resolve":
		return wire.Workspace{Root: workspace.Root, Cwd: workspace.Cwd, Branch: workspace.Branch(ctx), Git: workspace.Git}, nil
	case wire.Prefix + "git/diff":
		text, more, err := workspace.DiffPage(ctx, req.Kind, req.Offset)
		next := -1
		if more {
			next = req.Offset + len(text)
		}
		return wire.DiffPage{Text: text, Next: next}, err
	}
	config, err := h.Config.Reload(ctx, workspace.Cwd, workspace.Root)
	if err != nil {
		return nil, err
	}
	switch method {
	case wire.Prefix + "config/get":
		var selection wire.Selection
		if req.SessionID != "" {
			meta, _, _, err := session.HistoryPage(ctx, config.Home, req.SessionID, 0)
			if err == nil {
				selection = sessionView(meta).Selection
			}
		}
		return optionsView(config, selection, h.Models == nil && config.NeedsSetup()), nil
	case wire.Prefix + "config/setup":
		next, err := config.SaveInitial(ctx, setting.InitialConfig(req.Setup))
		if err != nil {
			return nil, errors.New(config.Redact(err.Error()))
		}
		return optionsView(next, wire.Selection{}, false), nil
	case wire.Prefix + "attachment/import":
		a, err := LoadAttachment(ctx, config.Home, workspace.Cwd, req.Path, config.Images)
		return attachmentView(a), err
	}
	return nil, acp.NewMethodNotFound(method)
}

func attachmentRefs(items []wire.Attachment) []session.Attachment {
	out := make([]session.Attachment, 0, len(items))
	for _, a := range items {
		out = append(out, session.Attachment{Hash: a.Hash, Filename: a.Filename, MediaType: a.MediaType})
	}
	return out
}

func (e *acpConversation) preparePrompt(ctx context.Context, req acp.PromptRequest, messages []model.Message, extended bool) ([]model.Message, error) {
	if extended {
		for i, block := range req.Prompt {
			switch {
			case block.Image != nil:
				if name, ok := block.Image.Meta[wire.Namespace+"filename"].(string); ok && len(name) <= 255 {
					part, ok := model.AsMessagePart[model.FilePart](messages[0].Content[i])
					if ok {
						part.Filename = filepath.Base(name)
						messages[0].Content[i] = part
					}
				}
			case block.ResourceLink != nil:
				link := block.ResourceLink
				hash, ok := strings.CutPrefix(link.Uri, "clara-attachment:")
				if !ok || link.Meta[wire.Namespace+"attachment"] != true || link.MimeType == nil {
					continue
				}
				file, err := (session.Attachment{Hash: hash, Filename: filepath.Base(link.Name), MediaType: *link.MimeType}).File(e.chat.Host.Config.Home)
				if err != nil {
					return nil, err
				}
				messages[0].Content[i] = file
			}
		}
	}
	return e.chat.PrepareImages(ctx, messages)
}

func serveACPQuestions(ctx context.Context, server *core.ACPServer, chat *Conversation, sid string) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-chat.Questions.Requests:
			respondACPQuestion(ctx, server, sid, req)
		}
	}
}

func respondACPQuestion(ctx context.Context, server *core.ACPServer, sid string, req QuestionRequest) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-req.Done:
			cancel()
		case <-ctx.Done():
		}
	}()
	schema := acp.UnstableElicitationSchema{Properties: map[string]any{}}
	items := make([]Question, len(req.Items))
	for i, item := range req.Items {
		key := "q" + strconv.Itoa(i)
		items[i] = item
		items[i].ID = key
		field := map[string]any{"type": "string", "title": item.Prompt(), "maxLength": 4096}
		if len(item.Options) != 0 {
			values := []string{}
			for _, option := range item.Options {
				if option.Label != "Other" {
					values = append(values, option.Label)
				}
			}
			field["enum"] = append(values, "Other")
			schema.Properties[key+".other"] = map[string]any{"type": "string", "title": "Custom answer", "maxLength": 4096}
		}
		schema.Properties[key] = field
		schema.Properties[key+".notes"] = map[string]any{"type": "string", "title": "Notes", "maxLength": 4096}
		schema.Required = append(schema.Required, key)
	}
	form := acp.NewUnstableCreateElicitationRequestForm(schema)
	form.Form.Message = "Clara Code needs your input to continue."
	form.Form.Meta = map[string]any{wire.Namespace + "sessionId": sid, wire.Namespace + "questions": items}
	result, err := server.ElicitForm(ctx, form)
	var answers map[string]string
	if err == nil && result.Accept != nil {
		answers = make(map[string]string)
		for i, item := range req.Items {
			key := items[i].ID
			value, _ := result.Accept.Content[key].(string)
			if len(item.Options) > 0 {
				if value == "Other" {
					value, _ = result.Accept.Content[key+".other"].(string)
				} else if !slices.ContainsFunc(item.Options, func(o QuestionOption) bool { return o.Label == value }) {
					continue
				}
			}
			if strings.TrimSpace(value) == "" || len(value) > 4096 {
				continue
			}
			notes, _ := result.Accept.Content[key+".notes"].(string)
			body, _ := json.Marshal(QuestionAnswer{Answers: []string{value}, Notes: notes})
			answers[item.ID] = string(body)
		}
	}
	select {
	case req.Answer <- answers:
	case <-ctx.Done():
	}
}
