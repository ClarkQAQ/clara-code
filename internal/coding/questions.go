package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"clara-code/pkg/model"
	"clara-code/pkg/tool"
)

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

func (o *QuestionOption) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &o.Label)
	}
	type option QuestionOption
	return json.Unmarshal(data, (*option)(o))
}

type Question struct {
	ID       string           `json:"id"`
	Header   string           `json:"header,omitempty"`
	Question string           `json:"question,omitempty"`
	Text     string           `json:"text,omitempty"`
	Options  []QuestionOption `json:"options,omitempty"`
}

func (q Question) Prompt() string {
	if q.Question != "" {
		return q.Question
	}
	return q.Text
}

type QuestionAnswer struct {
	Answers []string `json:"answers"`
	Notes   string   `json:"notes,omitempty"`
}

type Questions struct {
	Requests chan QuestionRequest
}

type QuestionRequest struct {
	Items  []Question
	Answer chan map[string]string
	Done   <-chan struct{}
}

func NewQuestions() *Questions {
	return &Questions{Requests: make(chan QuestionRequest)}
}

func (q *Questions) Tool() model.AgentTool {
	schema := json.RawMessage(`{
		"type":"object","additionalProperties":false,"required":["questions"],
		"properties":{"questions":{"type":"array","minItems":1,"maxItems":3,"items":{
			"type":"object","additionalProperties":false,"required":["id"],
			"properties":{
				"id":{"type":"string","minLength":1,"maxLength":64},
				"header":{"type":"string","maxLength":80},
				"question":{"type":"string","maxLength":2048},
				"text":{"type":"string","maxLength":2048},
				"options":{"type":"array","maxItems":4,"items":{"anyOf":[
					{"type":"string","minLength":1,"maxLength":256},
					{"type":"object","additionalProperties":false,"required":["label"],
					 "properties":{"label":{"type":"string","minLength":1,"maxLength":256},
					 "description":{"type":"string","maxLength":2048}}}
				]}}
			}
		}}}
	}`)
	type input struct {
		Questions []Question `json:"questions"`
	}
	return tool.Func(tool.Definition{Name: "request_user_input",
		Description: "Ask one to three questions. Provide a short header, question, and options with labels and descriptions. Put the recommended option first. The user can add notes or choose None of the above.",
		InputSchema: schema, Effects: []tool.Effect{tool.EffectInteraction}},
		func(ctx context.Context, in input) (tool.Result, error) {
			seen := map[string]bool{}
			for _, item := range in.Questions {
				if seen[item.ID] || strings.TrimSpace(item.Prompt()) == "" {
					return tool.ErrorResult("each question needs a unique id and non-empty question"), nil
				}
				seen[item.ID] = true
			}
			request := QuestionRequest{in.Questions, make(chan map[string]string, 1), ctx.Done()}
			select {
			case <-ctx.Done():
				return tool.Result{}, ctx.Err()
			case q.Requests <- request:
			}
			select {
			case <-ctx.Done():
				return tool.Result{}, ctx.Err()
			case answer := <-request.Answer:
				result := make(map[string]QuestionAnswer, len(answer))
				for id, value := range answer {
					if !seen[id] || len(value) > 16<<10 {
						return tool.ErrorResult("invalid question answer"), nil
					}
					var structured QuestionAnswer
					if json.Unmarshal([]byte(value), &structured) != nil || structured.Answers == nil {
						structured.Answers = []string{value}
					}
					if len(structured.Notes) > 4096 {
						return tool.ErrorResult("answer notes exceed 4096 bytes"), nil
					}
					result[id] = structured
				}
				if len(result) != len(in.Questions) {
					return tool.ErrorResult(fmt.Sprintf("expected %d answers", len(in.Questions))), nil
				}
				prompts := make(map[string]string, len(in.Questions))
				for _, item := range in.Questions {
					prompts[item.ID] = item.Prompt()
				}
				return tool.TextResult(jsonText(map[string]any{"answers": result, "questions": prompts})), nil
			}
		})
}
