package provider

import (
	yuzu "clara-code/pkg/model"
	"clara-code/pkg/model/schema"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// generateObjectWithTool is a helper for providers without native JSON mode.
// It converts the schema to a tool definition, forces the model to call it,
// and extracts the tool's input as the structured output.
func generateObjectWithTool(
	ctx context.Context,
	model yuzu.LanguageModel,
	call yuzu.ObjectCall,
) (*yuzu.ObjectResponse, error) {
	toolName := call.SchemaName
	if toolName == "" {
		toolName = "generate_object"
	}

	toolDescription := call.SchemaDescription
	if toolDescription == "" {
		toolDescription = "Generate a structured object matching the schema"
	}

	tool := yuzu.FunctionTool{
		Name:        toolName,
		Description: toolDescription,
		InputSchema: schema.ToMap(call.Schema),
	}

	toolChoice := yuzu.SpecificToolChoice(tool.Name)
	resp, err := model.Generate(ctx, yuzu.Call{
		Prompt:           call.Prompt,
		Tools:            []yuzu.Tool{tool},
		ToolChoice:       &toolChoice,
		MaxOutputTokens:  call.MaxOutputTokens,
		Temperature:      call.Temperature,
		TopP:             call.TopP,
		TopK:             call.TopK,
		PresencePenalty:  call.PresencePenalty,
		FrequencyPenalty: call.FrequencyPenalty,
		UserAgent:        call.UserAgent,
		Headers:          call.Headers,
		ProviderOptions:  call.ProviderOptions,
	})
	if err != nil {
		return nil, fmt.Errorf("tool-based generation failed: %w", err)
	}

	toolCalls := resp.Content.ToolCalls()
	if len(toolCalls) == 0 {
		return nil, &yuzu.NoObjectGeneratedError{
			RawText:      resp.Content.Text(),
			ParseError:   fmt.Errorf("no tool call generated"),
			Usage:        resp.Usage,
			FinishReason: resp.FinishReason,
		}
	}

	toolCall := toolCalls[0]

	var obj any
	if call.RepairText != nil {
		obj, err = schema.ParseAndValidateWithRepair(ctx, toolCall.Input, call.Schema, call.RepairText)
	} else {
		obj, err = schema.ParseAndValidate(toolCall.Input, call.Schema)
	}

	if err != nil {
		if nogErr, ok := errors.AsType[*yuzu.NoObjectGeneratedError](err); ok {
			nogErr.Usage = resp.Usage
			nogErr.FinishReason = resp.FinishReason
		}
		return nil, err
	}

	return &yuzu.ObjectResponse{
		Object:           obj,
		RawText:          toolCall.Input,
		Usage:            resp.Usage,
		FinishReason:     resp.FinishReason,
		Warnings:         resp.Warnings,
		ProviderMetadata: resp.ProviderMetadata,
	}, nil
}

// generateObjectWithText is a helper for providers without tool or JSON mode support.
// It adds the schema to the system prompt and parses the text response as JSON.
// This is a fallback for older models or simple providers.
func generateObjectWithText(
	ctx context.Context,
	model yuzu.LanguageModel,
	call yuzu.ObjectCall,
) (*yuzu.ObjectResponse, error) {
	jsonSchemaBytes, err := json.Marshal(call.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	schemaInstruction := fmt.Sprintf(
		"You must respond with valid JSON that matches this schema: %s\n"+
			"Respond ONLY with the JSON object, no additional text or explanation.",
		string(jsonSchemaBytes),
	)

	enhancedPrompt := make(yuzu.Prompt, 0, len(call.Prompt)+1)

	hasSystem := false
	for _, msg := range call.Prompt {
		if msg.Role == yuzu.MessageRoleSystem {
			hasSystem = true
			existingText := ""
			if len(msg.Content) > 0 {
				if textPart, ok := msg.Content[0].(yuzu.TextPart); ok {
					existingText = textPart.Text
				}
			}
			enhancedPrompt = append(enhancedPrompt, yuzu.NewSystemMessage(existingText+"\n\n"+schemaInstruction))
		} else {
			enhancedPrompt = append(enhancedPrompt, msg)
		}
	}

	if !hasSystem {
		enhancedPrompt = append(yuzu.Prompt{yuzu.NewSystemMessage(schemaInstruction)}, call.Prompt...)
	}

	resp, err := model.Generate(ctx, yuzu.Call{
		Prompt:           enhancedPrompt,
		MaxOutputTokens:  call.MaxOutputTokens,
		Temperature:      call.Temperature,
		TopP:             call.TopP,
		TopK:             call.TopK,
		PresencePenalty:  call.PresencePenalty,
		FrequencyPenalty: call.FrequencyPenalty,
		UserAgent:        call.UserAgent,
		Headers:          call.Headers,
		ProviderOptions:  call.ProviderOptions,
	})
	if err != nil {
		return nil, fmt.Errorf("text-based generation failed: %w", err)
	}

	textContent := resp.Content.Text()
	if textContent == "" {
		return nil, &yuzu.NoObjectGeneratedError{
			RawText:      "",
			ParseError:   fmt.Errorf("no text content in response"),
			Usage:        resp.Usage,
			FinishReason: resp.FinishReason,
		}
	}

	var obj any
	if call.RepairText != nil {
		obj, err = schema.ParseAndValidateWithRepair(ctx, textContent, call.Schema, call.RepairText)
	} else {
		obj, err = schema.ParseAndValidate(textContent, call.Schema)
	}

	if err != nil {
		if nogErr, ok := errors.AsType[*schema.ParseError](err); ok {
			return nil, &yuzu.NoObjectGeneratedError{
				RawText:         nogErr.RawText,
				ParseError:      nogErr.ParseError,
				ValidationError: nogErr.ValidationError,
				Usage:           resp.Usage,
				FinishReason:    resp.FinishReason,
			}
		}
		return nil, err
	}

	return &yuzu.ObjectResponse{
		Object:           obj,
		RawText:          textContent,
		Usage:            resp.Usage,
		FinishReason:     resp.FinishReason,
		Warnings:         resp.Warnings,
		ProviderMetadata: resp.ProviderMetadata,
	}, nil
}

// streamObjectWithTool is a helper for providers without native JSON streaming.
// It uses streaming tool calls to extract and parse the structured output progressively.
func streamObjectWithTool(
	ctx context.Context,
	model yuzu.LanguageModel,
	call yuzu.ObjectCall,
) (yuzu.ObjectStreamResponse, error) {
	// Create a tool from the schema
	toolName := call.SchemaName
	if toolName == "" {
		toolName = "generate_object"
	}

	toolDescription := call.SchemaDescription
	if toolDescription == "" {
		toolDescription = "Generate a structured object matching the schema"
	}

	tool := yuzu.FunctionTool{
		Name:        toolName,
		Description: toolDescription,
		InputSchema: schema.ToMap(call.Schema),
	}

	// Make a streaming Generate call with forced tool choice
	toolChoice := yuzu.SpecificToolChoice(tool.Name)
	stream, err := model.Stream(ctx, yuzu.Call{
		Prompt:           call.Prompt,
		Tools:            []yuzu.Tool{tool},
		ToolChoice:       &toolChoice,
		MaxOutputTokens:  call.MaxOutputTokens,
		Temperature:      call.Temperature,
		TopP:             call.TopP,
		TopK:             call.TopK,
		PresencePenalty:  call.PresencePenalty,
		FrequencyPenalty: call.FrequencyPenalty,
		UserAgent:        call.UserAgent,
		Headers:          call.Headers,
		ProviderOptions:  call.ProviderOptions,
	})
	if err != nil {
		return nil, fmt.Errorf("tool-based streaming failed: %w", err)
	}

	return readObjectStream(ctx, stream, call, tool.Name), nil
}

// streamObjectWithText is a helper for providers without tool or JSON streaming support.
// It adds the schema to the system prompt and parses the streamed text as JSON progressively.
func streamObjectWithText(
	ctx context.Context,
	model yuzu.LanguageModel,
	call yuzu.ObjectCall,
) (yuzu.ObjectStreamResponse, error) {
	jsonSchemaMap := schema.ToMap(call.Schema)
	jsonSchemaBytes, err := json.Marshal(jsonSchemaMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	schemaInstruction := fmt.Sprintf(
		"You must respond with valid JSON that matches this schema: %s\n"+
			"Respond ONLY with the JSON object, no additional text or explanation.",
		string(jsonSchemaBytes),
	)

	enhancedPrompt := make(yuzu.Prompt, 0, len(call.Prompt)+1)

	hasSystem := false
	for _, msg := range call.Prompt {
		if msg.Role == yuzu.MessageRoleSystem {
			hasSystem = true
			existingText := ""
			if len(msg.Content) > 0 {
				if textPart, ok := msg.Content[0].(yuzu.TextPart); ok {
					existingText = textPart.Text
				}
			}
			enhancedPrompt = append(enhancedPrompt, yuzu.NewSystemMessage(existingText+"\n\n"+schemaInstruction))
		} else {
			enhancedPrompt = append(enhancedPrompt, msg)
		}
	}

	if !hasSystem {
		enhancedPrompt = append(yuzu.Prompt{yuzu.NewSystemMessage(schemaInstruction)}, call.Prompt...)
	}

	stream, err := model.Stream(ctx, yuzu.Call{
		Prompt:           enhancedPrompt,
		MaxOutputTokens:  call.MaxOutputTokens,
		Temperature:      call.Temperature,
		TopP:             call.TopP,
		TopK:             call.TopK,
		PresencePenalty:  call.PresencePenalty,
		FrequencyPenalty: call.FrequencyPenalty,
		UserAgent:        call.UserAgent,
		Headers:          call.Headers,
		ProviderOptions:  call.ProviderOptions,
	})
	if err != nil {
		return nil, fmt.Errorf("text-based streaming failed: %w", err)
	}

	return readObjectStream(ctx, stream, call, ""), nil
}

// readObjectStream adapts text and forced-tool responses to the same JSON stream.
func readObjectStream(ctx context.Context, stream yuzu.StreamResponse, call yuzu.ObjectCall, toolName string) yuzu.ObjectStreamResponse {
	return func(yield func(yuzu.ObjectStreamPart) bool) {
		var object objectStream
		var done yuzu.ObjectStreamPart
		var terminal, sawToolCall bool
		var toolID string

		for part := range stream {
			switch part.Type {
			case yuzu.StreamPartTypeTextDelta:
				if toolID == "" && !sawToolCall && !object.append(part.Delta, call, yield) {
					return
				}
			case yuzu.StreamPartTypeToolInputStart:
				if toolName != "" && part.ToolCallName == toolName && toolID == "" {
					toolID = part.ID
					object.replace("")
				}
			case yuzu.StreamPartTypeToolInputDelta:
				if toolID != "" && part.ID == toolID && !object.append(part.Delta, call, yield) {
					return
				}
			case yuzu.StreamPartTypeToolCall:
				if toolName != "" && part.ToolCallName == toolName && !sawToolCall {
					object.replace(part.ToolCallInput)
					sawToolCall = true
				}
			case yuzu.StreamPartTypeError:
				yield(yuzu.ObjectStreamPart{Type: yuzu.ObjectStreamPartTypeError, Error: part.Error})
				return
			case yuzu.StreamPartTypeFinish:
				terminal, done.Usage, done.FinishReason = true, part.Usage, part.FinishReason
			case yuzu.StreamPartTypeWarnings:
				done.Warnings = part.Warnings
			}
			if !part.ProviderMetadata.IsZero() {
				done.ProviderMetadata = part.ProviderMetadata
			}
		}

		if terminal && toolID != "" && !sawToolCall && (done.FinishReason == yuzu.FinishReasonStop || done.FinishReason == yuzu.FinishReasonToolCalls) {
			yield(yuzu.ObjectStreamPart{Type: yuzu.ObjectStreamPartTypeError, Error: &yuzu.NoObjectGeneratedError{
				RawText: object.raw.String(), ParseError: fmt.Errorf("no tool call generated"),
				Usage: done.Usage, FinishReason: done.FinishReason,
			}})
			return
		}
		object.finish(ctx, call, terminal, done, yield)
	}
}
