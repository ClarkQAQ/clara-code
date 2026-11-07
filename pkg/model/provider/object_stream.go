package provider

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"clara-code/pkg/model"
	"clara-code/pkg/model/schema"
)

// objectStream buffers the provider's JSON. Previews never alter the buffer.
type objectStream struct {
	raw       strings.Builder
	previewed bool
	last      any
}

func (s *objectStream) append(delta string, call model.ObjectCall, yield func(model.ObjectStreamPart) bool) bool {
	s.raw.WriteString(delta)
	obj, err := schema.ParseAndValidate(s.raw.String(), call.Schema)
	return err != nil || s.emit(obj, yield)
}

func (s *objectStream) replace(text string) {
	s.raw.Reset()
	s.raw.WriteString(text)
}

func (s *objectStream) emit(obj any, yield func(model.ObjectStreamPart) bool) bool {
	if s.previewed && reflect.DeepEqual(s.last, obj) {
		return true
	}
	s.previewed, s.last = true, obj
	return yield(model.ObjectStreamPart{Type: model.ObjectStreamPartTypeObject, Object: obj})
}

func (s *objectStream) finish(ctx context.Context, call model.ObjectCall, terminal bool, part model.ObjectStreamPart, yield func(model.ObjectStreamPart) bool) {
	if err := ctx.Err(); err != nil {
		yield(model.ObjectStreamPart{Type: model.ObjectStreamPartTypeError, Error: err})
		return
	}
	if !terminal {
		yield(model.ObjectStreamPart{Type: model.ObjectStreamPartTypeError, Error: model.NewIncompleteStreamError()})
		return
	}

	raw := s.raw.String()
	if part.FinishReason != model.FinishReasonStop && part.FinishReason != model.FinishReasonToolCalls {
		yield(model.ObjectStreamPart{Type: model.ObjectStreamPartTypeError, Error: &model.NoObjectGeneratedError{
			RawText: raw, ParseError: fmt.Errorf("object stream ended with %q", part.FinishReason),
			Usage: part.Usage, FinishReason: part.FinishReason,
		}})
		return
	}

	obj, err := schema.ParseAndValidateWithRepair(ctx, raw, call.Schema, call.RepairText)
	if err != nil {
		failure := &model.NoObjectGeneratedError{RawText: raw, ParseError: err, Usage: part.Usage, FinishReason: part.FinishReason}
		if detail, ok := errors.AsType[*schema.ParseError](err); ok {
			failure.RawText, failure.ParseError, failure.ValidationError = detail.RawText, detail.ParseError, detail.ValidationError
		}
		yield(model.ObjectStreamPart{Type: model.ObjectStreamPartTypeError, Error: failure})
		return
	}
	if s.emit(obj, yield) {
		part.Type = model.ObjectStreamPartTypeFinish
		yield(part)
	}
}
