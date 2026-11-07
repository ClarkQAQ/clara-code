package prompt

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"clara-code/pkg/core"
	"clara-code/pkg/models"
)

// Compactor replaces old, complete turns with a model-generated handoff.
// BudgetBytes is a conservative serialized context budget, including tool schemas.
type Compactor struct {
	Model        models.LanguageModel
	BudgetBytes  int
	Instructions string
}

func (c Compactor) Fit(ctx context.Context, model models.LanguageModel, system string, history []models.Message, items []core.AgentTool) ([]models.Message, error) {
	if c.BudgetBytes <= 0 {
		return nil, nil
	}
	definitions := make([]models.ToolInfo, 0, len(items))
	for _, tool := range items {
		definitions = append(definitions, tool.Info())
	}
	encoded, err := json.Marshal([]any{system, history, definitions})
	if err != nil {
		return nil, err
	}
	if len(encoded) <= c.BudgetBytes {
		return nil, nil
	}
	start, cut := 0, 0
	for start < len(history) && history[start].Role == models.MessageRoleSystem {
		start++
	}
	for i := start + 1; i < len(history); i++ {
		if history[i].Role == models.MessageRoleUser {
			cut = i
		}
	}
	if cut <= start {
		return nil, errors.New("prompt exceeds budget without a complete old turn to compact")
	}
	transcript, err := json.Marshal(history[start:cut])
	if err != nil {
		return nil, err
	}
	if c.Model != nil {
		model = c.Model
	}
	if model == nil {
		return nil, errors.New("compaction model is required")
	}
	instructions := c.Instructions
	if instructions == "" {
		instructions = Default("compact")
	}
	limit := int64(2048)
	response, err := model.Generate(ctx, models.Call{Prompt: []models.Message{
		models.NewSystemMessage(instructions), models.NewUserMessage(string(transcript)),
	}, MaxOutputTokens: &limit})
	if err != nil {
		return nil, err
	}
	if response == nil || response.Content.Text() == "" {
		return nil, errors.New("compaction returned no summary")
	}
	out := append(slices.Clone(history[:start]), models.NewUserMessage("Prior conversation handoff:\n"+response.Content.Text()))
	out = append(out, history[cut:]...)
	encoded, err = json.Marshal([]any{system, out, definitions})
	if err != nil {
		return nil, err
	}
	if len(encoded) > c.BudgetBytes {
		return nil, errors.New("prompt exceeds budget after compaction")
	}
	return out, nil
}
