package completions

import (
	"testing"

	"clara-code/pkg/model/relay/internal/message"
	wire "clara-code/pkg/model/relay/internal/models/protocol/completions"
)

func TestCompletionToolContentKeepsMedia(t *testing.T) {
	output := completionToolContent(&message.ToolResult{
		Output: "caption",
		Content: []message.Part{message.ImagePart(message.Image{
			Data: []byte("image"), MIME: "image/png",
		})},
	})
	parts, ok := output.([]wire.ContentPart)
	if !ok || len(parts) != 2 {
		t.Fatalf("output = %#v", output)
	}
}
