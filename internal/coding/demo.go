package coding

import (
	"context"
	"errors"
	"strings"
	"time"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"

	"clara-code/internal/session"
)

type DemoModel struct{}

func (DemoModel) Provider() string { return "demo" }
func (DemoModel) Model() string    { return "demo" }

func (DemoModel) Generate(ctx context.Context, call model.Call) (*model.Response, error) {
	var last string
	for _, message := range call.Prompt {
		text := session.Text(message)
		if message.Role == model.MessageRoleSystem && text == prompt.Default("compact") {
			return demoText("Earlier conversation summarized."), nil
		}
		if message.Role == model.MessageRoleUser && (message.Metadata == nil || message.Metadata.Kind != "context") && !strings.HasPrefix(text, "# Current workspace state") {
			last = text
		}
	}
	if strings.Contains(last, "CANCEL_SLOW_TURN") {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	switch strings.TrimSpace(last) {
	case "你好":
		return demoText("你好！有什么可以帮你的吗？喵~"), nil
	case "":
		return demoText("Ready to help."), nil
	default:
		return demoText("Demo response: " + last), nil
	}
}

func demoText(text string) *model.Response {
	return &model.Response{Content: model.ResponseContent{model.TextContent{Text: text}},
		FinishReason: model.FinishReasonStop, Usage: model.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}}
}

func (d DemoModel) Stream(ctx context.Context, call model.Call) (model.StreamResponse, error) {
	response, err := d.Generate(ctx, call)
	if err != nil {
		return nil, err
	}
	return func(yield func(model.StreamPart) bool) {
		const id = "demo"
		if !yield(model.StreamPart{Type: model.StreamPartTypeTextStart, ID: id}) {
			return
		}
		for _, r := range response.Content.Text() {
			if ctx.Err() != nil || !yield(model.StreamPart{Type: model.StreamPartTypeTextDelta, ID: id, Delta: string(r)}) {
				return
			}
		}
		if !yield(model.StreamPart{Type: model.StreamPartTypeTextEnd, ID: id}) {
			return
		}
		yield(model.StreamPart{Type: model.StreamPartTypeFinish, FinishReason: response.FinishReason, Usage: response.Usage})
	}, nil
}

func (DemoModel) GenerateObject(context.Context, model.ObjectCall) (*model.ObjectResponse, error) {
	return nil, errors.New("demo model supports text only")
}

func (DemoModel) StreamObject(context.Context, model.ObjectCall) (model.ObjectStreamResponse, error) {
	return nil, errors.New("demo model supports text only")
}
