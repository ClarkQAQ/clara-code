package prompt

import (
	"slices"
	"unicode/utf8"

	"clara-code/pkg/model"
)

const retainedUserTokens int64 = 20_000

// compactedHistory keeps durable policy, recent user messages, and their original
// order. The handoff closes the old window before any unsent input is appended.
func compactedHistory(history []model.Message, active, end int, historicalTokens int64, summary string) ([]model.Message, []int, int64) {
	var historicalUsed int64
	retained := make([]model.Message, end)
	remaining := retainedUserTokens
	for i := end - 1; i >= 0; i-- {
		msg := history[i]
		switch msg.Role {
		case model.MessageRoleDeveloper, model.MessageRoleSystem:
			retained[i] = msg
		case model.MessageRoleUser:
			if handoffMessage(msg) || msg.Metadata != nil && msg.Metadata.Kind == "context" {
				continue
			}
			if i < active {
				remaining = min(remaining, historicalTokens)
			}
			available := remaining
			msg.Content, remaining = retainedUserContent(msg.Content, available, i >= active)
			if i < active {
				historicalUsed += available - remaining
			}
			if len(msg.Content) > 0 {
				retained[i] = msg
			}
		}
	}

	out := make([]model.Message, 0, len(history)+1)
	anchors := make([]int, len(history)+1)
	for i, msg := range retained {
		anchors[i] = len(out)
		if msg.Role != "" {
			out = append(out, msg)
		}
	}
	handoff := model.NewUserMessage("Prior conversation handoff:\n" + summary)
	handoff.Metadata = &model.MessageMetadata{Kind: "handoff"}
	out = append(out, handoff)
	for i := end; i < len(history); i++ {
		anchors[i] = len(out)
		out = append(out, history[i])
	}
	anchors[len(history)] = len(out)
	return out, anchors, historicalUsed
}

func retainedUserContent(parts []model.MessagePart, tokens int64, active bool) ([]model.MessagePart, int64) {
	var kept []model.MessagePart
	for _, part := range slices.Backward(parts) {
		if text, ok := model.AsMessagePart[model.TextPart](part); ok {
			if tokens <= 0 || text.Text == "" {
				continue
			}
			cost := tokensForBytes(len(text.Text))
			if cost > tokens {
				text.Text = shortenUserText(text.Text, int(tokens)*4)
				cost = tokens
			}
			tokens -= cost
			if text.Text != "" {
				kept = append(kept, text)
			}
		} else if _, ok := model.AsMessagePart[model.FilePart](part); ok && active {
			kept = append(kept, part)
		}
	}
	slices.Reverse(kept)
	return kept, tokens
}

// Retain both ends of the boundary message, which often carries a final question
// after a long paste. Neither half may split a UTF-8 character.
func shortenUserText(text string, limit int) string {
	const marker = "…"
	available := limit - len(marker)
	head := (available + 1) / 2
	for head > 0 && !utf8.RuneStart(text[head]) {
		head--
	}
	tail := len(text) - available/2
	for tail < len(text) && !utf8.RuneStart(text[tail]) {
		tail++
	}
	return text[:head] + marker + text[tail:]
}
