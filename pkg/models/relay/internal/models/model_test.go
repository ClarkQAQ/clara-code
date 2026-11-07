package models_test

import (
	"context"
	"io"
	"testing"

	"clara-code/pkg/models/relay/internal/message"
	"clara-code/pkg/models/relay/internal/models"
)

type fakeModel struct{}

func (fakeModel) Info() models.Info { return models.Info{ID: "fake"} }

func (fakeModel) Stream(context.Context, models.Request) (models.Stream, error) {
	return &fakeStream{events: []models.StreamEvent{
		{Type: models.StreamTextDelta, Delta: "hello"},
		{Type: models.StreamUsage, Usage: &models.Usage{InputTokens: 2, OutputTokens: 1, TotalTokens: 3}},
		{Type: models.StreamCompleted, Response: &models.Response{
			Message:      message.Assistant(message.Text("hello")),
			FinishReason: models.FinishStop,
		}},
	}}, nil
}

type fakeStream struct {
	events []models.StreamEvent
	index  int
}

func (s *fakeStream) Next(context.Context) (models.StreamEvent, error) {
	if s.index >= len(s.events) {
		return models.StreamEvent{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *fakeStream) Close() error { return nil }

func TestGenerateCollectsUsageAndResponse(t *testing.T) {
	response, err := models.Generate(context.Background(), fakeModel{}, models.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got := response.Message.Text(); got != "hello" {
		t.Fatalf("text = %q", got)
	}
	if response.Usage.TotalTokens != 3 {
		t.Fatalf("usage = %+v", response.Usage)
	}
}
