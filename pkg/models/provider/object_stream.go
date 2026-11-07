package provider

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"clara-code/pkg/models"
	"clara-code/pkg/models/schema"
)

// objectStream buffers the provider's JSON. Previews never alter the buffer.
type objectStream struct {
	raw       strings.Builder
	previewed bool
	last      any
}

func (s *objectStream) append(delta string, call models.ObjectCall, yield func(models.ObjectStreamPart) bool) bool {
	s.raw.WriteString(delta)
	obj, err := schema.ParseAndValidate(s.raw.String(), call.Schema)
	return err != nil || s.emit(obj, yield)
}

func (s *objectStream) replace(text string) {
	s.raw.Reset()
	s.raw.WriteString(text)
}

func (s *objectStream) emit(obj any, yield func(models.ObjectStreamPart) bool) bool {
	if s.previewed && reflect.DeepEqual(s.last, obj) {
		return true
	}
	s.previewed, s.last = true, obj
	return yield(models.ObjectStreamPart{Type: models.ObjectStreamPartTypeObject, Object: obj})
}

func (s *objectStream) finish(ctx context.Context, call models.ObjectCall, terminal bool, part models.ObjectStreamPart, yield func(models.ObjectStreamPart) bool) {
	if err := ctx.Err(); err != nil {
		yield(models.ObjectStreamPart{Type: models.ObjectStreamPartTypeError, Error: err})
		return
	}
	if !terminal {
		yield(models.ObjectStreamPart{Type: models.ObjectStreamPartTypeError, Error: models.NewIncompleteStreamError()})
		return
	}

	raw := s.raw.String()
	if part.FinishReason != models.FinishReasonStop && part.FinishReason != models.FinishReasonToolCalls {
		yield(models.ObjectStreamPart{Type: models.ObjectStreamPartTypeError, Error: &models.NoObjectGeneratedError{
			RawText: raw, ParseError: fmt.Errorf("object stream ended with %q", part.FinishReason),
			Usage: part.Usage, FinishReason: part.FinishReason,
		}})
		return
	}

	obj, err := schema.ParseAndValidateWithRepair(ctx, raw, call.Schema, call.RepairText)
	if err != nil {
		failure := &models.NoObjectGeneratedError{RawText: raw, ParseError: err, Usage: part.Usage, FinishReason: part.FinishReason}
		if detail, ok := errors.AsType[*schema.ParseError](err); ok {
			failure.RawText, failure.ParseError, failure.ValidationError = detail.RawText, detail.ParseError, detail.ValidationError
		}
		yield(models.ObjectStreamPart{Type: models.ObjectStreamPartTypeError, Error: failure})
		return
	}
	if s.emit(obj, yield) {
		part.Type = models.ObjectStreamPartTypeFinish
		yield(part)
	}
}
