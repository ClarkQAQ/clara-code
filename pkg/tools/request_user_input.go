package tools

import (
	"context"
	"encoding/json"
	"strings"

	"clara-code/pkg/util/errs"
)

type Question struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Options []string `json:"options,omitempty"`
}

type AskUser func(context.Context, []Question) (map[string]string, error)

func RequestUserInput(ask AskUser) Tool {
	return Func(Definition{Name: "request_user_input", Description: "Ask the user to resolve a decision. Group related questions.", Effects: []Effect{EffectInteraction}},
		func(ctx context.Context, input struct {
			Questions []Question `json:"questions"`
		}) (Result, error) {
			if len(input.Questions) == 0 || len(input.Questions) > 3 {
				return Result{}, errs.New(errs.CodeInvalidArgument, "provide one to three questions")
			}
			seen := map[string]bool{}
			for _, q := range input.Questions {
				if q.ID == "" || len(q.ID) > 64 || seen[q.ID] || strings.TrimSpace(q.Text) == "" || len(q.Text) > 2048 || len(q.Options) > 4 {
					return Result{}, errs.New(errs.CodeInvalidArgument, "invalid question")
				}
				seen[q.ID] = true
				for _, option := range q.Options {
					if option == "" || len(option) > 256 {
						return Result{}, errs.New(errs.CodeInvalidArgument, "invalid question option")
					}
				}
			}
			if ask == nil {
				return Result{}, errs.ErrUnsupported
			}
			answers, err := ask(ctx, input.Questions)
			if err != nil {
				return Result{}, err
			}
			for id, answer := range answers {
				if !seen[id] || len(answer) > 4096 {
					return Result{}, errs.New(errs.CodeInvalidArgument, "invalid user answer")
				}
			}
			raw, err := json.Marshal(answers)
			return TextResult(string(raw)), err
		})
}
