package openai

import (
	"bytes"
	"encoding/json"
	"reflect"
)

// paramObj carries the non-standard request fields of a parameter struct.
type paramObj struct{ extras map[string]any }

// SetExtraFields injects non-standard fields into the serialized object.
func (p *paramObj) SetExtraFields(extras map[string]any) { p.extras = extras }

// ExtraFields returns the non-standard fields of the object.
func (p paramObj) ExtraFields() map[string]any { return p.extras }

// Optional is the interface implemented by every [Opt].
type Optional interface {
	Valid() bool
	IsZero() bool
}

type optStatus int8

const (
	optOmitted optStatus = iota
	optNull
	optIncluded
)

// Opt holds an optional parameter of type T. The zero value is omitted from
// the request, so callers must use [NewOpt] to send a value and [Null] to send
// an explicit JSON null.
type Opt[T any] struct {
	Value  T
	status optStatus
}

// NewOpt returns an [Opt] holding the given value.
func NewOpt[T any](value T) Opt[T] { return Opt[T]{Value: value, status: optIncluded} }

// Null returns an [Opt] that serializes as JSON null.
func Null[T any]() Opt[T] { return Opt[T]{status: optNull} }

// Valid reports whether the value is set to something other than null.
func (o Opt[T]) Valid() bool { return o.status == optIncluded }

// Or returns the value when it is set, and the fallback otherwise.
func (o Opt[T]) Or(fallback T) T {
	if o.Valid() {
		return o.Value
	}
	return fallback
}

// IsZero reports whether the value is unset, which drives the omitzero tag.
func (o Opt[T]) IsZero() bool {
	if o.status != optOmitted {
		return false
	}
	var zero T
	return reflect.DeepEqual(o.Value, zero)
}

// MarshalJSON encodes the value, or null when it is unset.
func (o Opt[T]) MarshalJSON() ([]byte, error) {
	if !o.Valid() {
		return []byte("null"), nil
	}
	return json.Marshal(o.Value)
}

// UnmarshalJSON tracks whether the field was present, null or set.
func (o *Opt[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		o.status = optNull
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = value
	o.status = optIncluded
	return nil
}

// IsOmitted reports whether an optional parameter is unset, so the field it
// belongs to is left out of the request.
func IsOmitted(value any) bool {
	if value == nil {
		return false
	}
	if zeroer, ok := value.(interface{ IsZero() bool }); ok {
		return zeroer.IsZero()
	}
	return reflect.ValueOf(value).IsZero()
}

// String returns an optional string parameter.
func String(value string) Opt[string] { return NewOpt(value) }

// Int returns an optional integer parameter.
func Int(value int64) Opt[int64] { return NewOpt(value) }

// Bool returns an optional boolean parameter.
func Bool(value bool) Opt[bool] { return NewOpt(value) }

// Float returns an optional float parameter.
func Float(value float64) Opt[float64] { return NewOpt(value) }

// marshalObject serializes a parameter struct, filling discriminator defaults
// and merging non-standard fields.
func marshalObject(value any, extras map[string]any, defaults map[string]any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(defaults) == 0 && len(extras) == 0 {
		return data, nil
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, def := range defaults {
		if raw, ok := fields[key]; !ok || isBlankJSON(raw) {
			raw, err := json.Marshal(def)
			if err != nil {
				return nil, err
			}
			fields[key] = raw
		}
	}
	for key, extra := range extras {
		raw, err := json.Marshal(extra)
		if err != nil {
			return nil, err
		}
		fields[key] = raw
	}
	return json.Marshal(fields)
}

// marshalUnion serializes the single variant of a union that is set.
func marshalUnion(variants ...any) ([]byte, error) {
	for _, variant := range variants {
		if isOmittedValue(variant) {
			continue
		}
		return json.Marshal(variant)
	}
	return []byte("null"), nil
}

func isOmittedValue(variant any) bool {
	if variant == nil {
		return true
	}
	value := reflect.ValueOf(variant)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer:
		if value.IsNil() {
			return true
		}
	case reflect.Slice:
		if value.Len() == 0 {
			return true
		}
	}
	if zeroer, ok := variant.(interface{ IsZero() bool }); ok {
		return zeroer.IsZero()
	}
	return value.IsZero()
}

// tryUnmarshal decodes a union payload whose variants cannot be told apart by a
// discriminator field, such as a string or an array of objects.
func tryUnmarshal(data []byte, targets ...any) error {
	for _, target := range targets {
		resetValue(target)
		if err := json.Unmarshal(data, target); err == nil {
			return nil
		}
	}
	return nil
}

func resetValue(target any) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return
	}
	value.Elem().SetZero()
}

// unionDiscriminant returns the value of the field that identifies a union
// variant, which is either the object type or the message role.
func unionDiscriminant(data []byte) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return ""
	}
	for _, key := range []string{"type", "role"} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var value string
		if json.Unmarshal(raw, &value) == nil && value != "" {
			return value
		}
	}
	return ""
}

func isBlankJSON(raw json.RawMessage) bool {
	trimmed := string(bytes.TrimSpace(raw))
	return trimmed == `""` || trimmed == "null"
}
