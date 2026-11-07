package responses

import (
	"testing"

	"clara-code/pkg/model/relay/internal/message"
)

func TestToolOutputKeepsMedia(t *testing.T) {
	output := toolOutput(&message.ToolResult{
		Output: "caption",
		Content: []message.Part{message.ImagePart(message.Image{
			Data: []byte("image"), MIME: "image/png",
		})},
	})
	items, ok := output.([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("output = %#v", output)
	}
}
