package tool

import (
	"context"
	"fmt"

	clarafs "clara-code/pkg/tool/fs"
	claraimage "clara-code/pkg/tool/fs/image"
)

// Images provides image tools over one filesystem.
type Images struct {
	FS      clarafs.FS
	Options claraimage.Options
}

// New creates an image tool set.
func NewImages(storage clarafs.FS, options claraimage.Options) *Images {
	return &Images{FS: storage, Options: options}
}

// All returns the image
func (s *Images) All() []Tool {
	return []Tool{s.View()}
}

func (s *Images) View() Tool {
	type input struct {
		Path string `json:"path" description:"path of one static image"`
	}
	return Func(Definition{
		Name: "image_view",
		Description: "Inspect one static image and return it as model-visible content. " +
			"Use this for photos, screenshots, and diagrams. Animated images are rejected.",
		Effects: []Effect{EffectRead}, ParallelSafe: true,
	}, func(ctx context.Context, in input) (Result, error) {
		target, err := clarafs.ResolvePath(s.FS, in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		view, err := claraimage.Load(ctx, s.FS, target, s.Options)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		caption := fmt.Sprintf("%s (%s, %dx%d, %d bytes)", target, view.MIME, view.Width, view.Height, len(view.Data))
		return MediaResult(caption, view.Data, view.MIME), nil
	})
}
