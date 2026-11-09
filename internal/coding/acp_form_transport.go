package coding

import (
	"bytes"
	"encoding/json"
	"io"

	wire "clara-code/pkg/appwire"
)

// ACP SDK v0.13.5 does not serialize session scope on form requests. Supply the
// standard field on the wire, keeping the metadata copy for SDK-based clients
// whose decoder also lacks the field.
type scopedACPOutput struct{ io.Writer }

func (w scopedACPOutput) Write(data []byte) (int, error) {
	if !bytes.Contains(data, []byte(`"elicitation/create"`)) {
		return w.Writer.Write(data)
	}
	var envelope struct {
		JSONRPC string                     `json:"jsonrpc"`
		ID      json.RawMessage            `json:"id"`
		Method  string                     `json:"method"`
		Params  map[string]json.RawMessage `json:"params"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Method != "elicitation/create" || envelope.Params["sessionId"] != nil {
		return w.Writer.Write(data)
	}
	var meta map[string]json.RawMessage
	if json.Unmarshal(envelope.Params["_meta"], &meta) != nil || meta[wire.Namespace+"sessionId"] == nil {
		return w.Writer.Write(data)
	}
	envelope.Params["sessionId"] = meta[wire.Namespace+"sessionId"]
	out, err := json.Marshal(envelope)
	if err != nil {
		return 0, err
	}
	if _, err = w.Writer.Write(append(out, '\n')); err != nil {
		return 0, err
	}
	return len(data), nil
}
