package openai

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
)

var jsonKeysCache sync.Map // reflect.Type -> map[string]bool

// decodeExtras returns the fields of a JSON object that are not part of the
// given target struct, so endpoints that send non-standard fields do not lose
// them.
func decodeExtras(data []byte, known any) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	keys := knownJSONKeys(reflect.TypeOf(known))
	extras := make(map[string]json.RawMessage)
	for key, raw := range fields {
		if keys[key] {
			continue
		}
		extras[key] = raw
	}
	if len(extras) == 0 {
		return nil, nil
	}
	return extras, nil
}

func knownJSONKeys(t reflect.Type) map[string]bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if cached, ok := jsonKeysCache.Load(t); ok {
		return cached.(map[string]bool)
	}
	keys := map[string]bool{}
	if t.Kind() == reflect.Struct {
		for i := range t.NumField() {
			field := t.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			switch {
			case name == "-":
				continue
			case name != "":
				keys[name] = true
			case field.Anonymous:
				for key := range knownJSONKeys(field.Type) {
					keys[key] = true
				}
			default:
				keys[field.Name] = true
			}
		}
	}
	jsonKeysCache.Store(t, keys)
	return keys
}

// jsonField returns the raw value of a top level JSON field, or nil.
func jsonField(data []byte, key string) json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil
	}
	return fields[key]
}
