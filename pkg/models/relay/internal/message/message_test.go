package message

import "testing"

func TestMessageCloneIsIndependent(t *testing.T) {
	original := Assistant(Text("one"), ImagePart(Image{Data: []byte("x"), MIME: "image/png"}))
	cloned := original.Clone()
	cloned.Parts[0].Text = "two"
	cloned.Parts[1].Image.Data[0] = 'y'
	if original.Text() != "one" {
		t.Fatalf("original text changed: %q", original.Text())
	}
	if string(original.Parts[1].Image.Data) != "x" {
		t.Fatalf("original image changed: %q", original.Parts[1].Image.Data)
	}
}
