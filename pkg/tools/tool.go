// Package tools provides optional, host-assembled Clara Core tools.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/models"
	"github.com/google/jsonschema-go/jsonschema"
)

type Tool = core.AgentTool
type Result = core.ToolResponse
type Effect = models.Effect

const (
	EffectRead               = models.EffectRead
	EffectWrite              = models.EffectWrite
	EffectDelete             = models.EffectDelete
	EffectExecute            = models.EffectExecute
	EffectNetwork            = models.EffectNetwork
	EffectInteraction        = models.EffectInteraction
	EffectState              = models.EffectState
	EffectOther       Effect = "other"
	EffectAudio       Effect = "audio"
)

type Definition struct {
	Name, Description string
	InputSchema       json.RawMessage
	Effects           []Effect
	ParallelSafe      bool
	Timeout           time.Duration
	OutputLimit       int
}

type optionalTool struct {
	base      Tool
	info      models.ToolInfo
	run       func(context.Context, models.ToolCall) (Result, error)
	validator *jsonschema.Resolved
	schemaErr error
	limit     int
}

// Func adapts a typed handler to a Yuzu-style AgentTool.
func Func[Input any](def Definition, handler func(context.Context, Input) (Result, error)) Tool {
	run := func(ctx context.Context, call models.ToolCall) (Result, error) {
		var input Input
		if err := json.Unmarshal([]byte(call.Input), &input); err != nil {
			return ErrorResult(err.Error()), nil
		}
		return handler(ctx, input)
	}
	base := models.NewAgentTool(def.Name, def.Description, func(ctx context.Context, in Input, _ models.ToolCall) (Result, error) { return handler(ctx, in) })
	info := base.Info()
	info.Parallel, info.Timeout = def.ParallelSafe, def.Timeout
	for _, effect := range def.Effects {
		if info.Effect == "" || info.Effect == EffectNetwork {
			info.Effect = effect
		}
		if effect == EffectWrite || effect == EffectDelete || effect == EffectExecute {
			info.Effect = effect
			break
		}
	}
	if len(def.InputSchema) == 0 {
		def.InputSchema, _ = json.Marshal(map[string]any{"type": "object", "properties": info.Parameters, "required": info.Required, "additionalProperties": false})
	}
	var schema jsonschema.Schema
	var validator *jsonschema.Resolved
	err := json.Unmarshal(def.InputSchema, &schema)
	if err == nil {
		validator, err = schema.Resolve(nil)
		if err == nil {
			var fields struct {
				Properties map[string]any
				Required   []string
			}
			err = json.Unmarshal(def.InputSchema, &fields)
			info.Parameters, info.Required = fields.Properties, fields.Required
			if err == nil {
				err = json.Unmarshal(def.InputSchema, &info.InputSchema)
			}
		}
	}
	return &optionalTool{base: base, info: info, run: run, validator: validator, schemaErr: err, limit: def.OutputLimit}
}

func (tool *optionalTool) Info() models.ToolInfo { return tool.info }
func (tool *optionalTool) ProviderOptions() models.ProviderOptions {
	return tool.base.ProviderOptions()
}
func (tool *optionalTool) SetProviderOptions(options models.ProviderOptions) {
	tool.base.SetProviderOptions(options)
}
func (tool *optionalTool) SetParallel(value bool) { tool.info.Parallel = value }

func (tool *optionalTool) Run(ctx context.Context, call models.ToolCall) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if tool.schemaErr != nil {
		return Result{}, tool.schemaErr
	}
	if len(call.Input) > 8<<20 {
		return Result{}, errors.New("tool input limit reached")
	}
	if call.Input == "" {
		call.Input = "{}"
	}
	var data any
	if err := json.Unmarshal([]byte(call.Input), &data); err != nil {
		return tool.invalidInput(err), nil
	}
	if err := tool.validator.Validate(data); err != nil {
		return tool.invalidInput(err), nil
	}
	result, err := tool.run(ctx, call)
	limit := tool.limit
	if limit <= 0 || limit > 16<<20 {
		limit = 16 << 20
	}
	if len(result.Data) > limit {
		return ErrorResult("Tool media exceeds its size limit."), err
	}
	if len(result.Content) > limit {
		result.Content = strings.ToValidUTF8(result.Content[:limit], "") + "\n[output truncated]"
	}
	return result, err
}

func TextResult(text string) Result  { return models.NewTextResponse(text) }
func ErrorResult(text string) Result { return models.NewTextErrorResponse(text) }
func MediaResult(text string, data []byte, mime string) Result {
	result := models.NewMediaResponse(data, mime)
	result.Content = text
	return result
}

func (tool *optionalTool) invalidInput(err error) Result {
	names := make([]string, 0, len(tool.info.Parameters))
	for name := range tool.info.Parameters {
		names = append(names, name)
	}
	slices.Sort(names)
	return ErrorResult(fmt.Sprintf("%s: invalid arguments: %v\nAllowed fields: %s. Required: %s. Use the tool schema; no arguments were executed", tool.info.Name, err, strings.Join(names, ", "), strings.Join(tool.info.Required, ", ")))
}
