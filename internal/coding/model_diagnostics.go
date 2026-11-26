package coding

import (
	"context"
	"errors"
	"strings"

	"clara-code/pkg/model"
)

type diagnosticModel struct {
	model.LanguageModel
	redact func(string) string
}

type diagnosticError struct {
	cause   error
	message string
}

func (e diagnosticError) Error() string {
	return e.message
}

func (e diagnosticError) Unwrap() error {
	return e.cause
}

func (m diagnosticModel) safeError(err error) error {
	if err == nil {
		return nil
	}
	var provider *model.ProviderError
	if errors.As(err, &provider) {
		copy := *provider
		copy.Message = m.redact(copy.Message)
		copy.Title = m.redact(copy.Title)
		copy.URL = m.redact(copy.URL)
		copy.RequestBody = nil
		copy.ResponseBody = nil
		copy.ResponseHeaders = make(map[string]string, len(provider.ResponseHeaders))
		for name, value := range provider.ResponseHeaders {
			copy.ResponseHeaders[strings.ToLower(name)] = m.redact(value)
		}
		return &copy
	}
	return diagnosticError{cause: err, message: m.redact(err.Error())}
}

func (m diagnosticModel) Generate(ctx context.Context, call model.Call) (*model.Response, error) {
	response, err := m.LanguageModel.Generate(ctx, call)
	return response, m.safeError(err)
}

func (m diagnosticModel) Stream(ctx context.Context, call model.Call) (model.StreamResponse, error) {
	stream, err := m.LanguageModel.Stream(ctx, call)
	if err != nil {
		return nil, m.safeError(err)
	}
	if stream == nil {
		return nil, nil
	}
	return func(yield func(model.StreamPart) bool) {
		for part := range stream {
			part.Error = m.safeError(part.Error)
			if !yield(part) {
				return
			}
		}
	}, nil
}
