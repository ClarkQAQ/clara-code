package coding

import (
	"context"
	"errors"
	"strings"
	"time"

	"clara-core/agent/prompt"
	"clara-core/models"

	"clara-code/internal/session"
)

type DemoModel struct{}

func (DemoModel) Provider() string { return "demo" }
func (DemoModel) Model() string    { return "demo" }

func (DemoModel) Generate(ctx context.Context, call models.Call) (*models.Response, error) {
	var last string
	for _, message := range call.Prompt {
		text := session.Text(message)
		if message.Role == models.MessageRoleSystem && text == prompt.Default("compact") {
			return demoText("Earlier conversation summarized."), nil
		}
		if message.Role == models.MessageRoleUser && !strings.HasPrefix(text, "# Current workspace state") {
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

func demoText(text string) *models.Response {
	return &models.Response{Content: models.ResponseContent{models.TextContent{Text: text}},
		FinishReason: models.FinishReasonStop, Usage: models.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}}
}

func (d DemoModel) Stream(ctx context.Context, call models.Call) (models.StreamResponse, error) {
	response, err := d.Generate(ctx, call)
	if err != nil {
		return nil, err
	}
	return func(yield func(models.StreamPart) bool) {
		const id = "demo"
		if !yield(models.StreamPart{Type: models.StreamPartTypeTextStart, ID: id}) {
			return
		}
		for _, r := range response.Content.Text() {
			if ctx.Err() != nil || !yield(models.StreamPart{Type: models.StreamPartTypeTextDelta, ID: id, Delta: string(r)}) {
				return
			}
		}
		if !yield(models.StreamPart{Type: models.StreamPartTypeTextEnd, ID: id}) {
			return
		}
		yield(models.StreamPart{Type: models.StreamPartTypeFinish, FinishReason: response.FinishReason, Usage: response.Usage})
	}, nil
}

func (DemoModel) GenerateObject(context.Context, models.ObjectCall) (*models.ObjectResponse, error) {
	return nil, errors.New("demo model supports text only")
}

func (DemoModel) StreamObject(context.Context, models.ObjectCall) (models.ObjectStreamResponse, error) {
	return nil, errors.New("demo model supports text only")
}
