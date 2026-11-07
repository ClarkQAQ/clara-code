package tools

import (
	"context"
	"sync"

	clarafs "clara-code/pkg/tools/fs"
	"clara-code/pkg/tools/fs/hashline"
)

const readLimit = 8 << 20

// Files owns one session's filesystem tools and edit snapshots.
type Files struct {
	// SearchSource is an optional host policy, set before tools are used.
	// It applies only to glob and grep, not direct reads or file mutations.
	SearchSource func(context.Context, clarafs.Path, bool) (clarafs.FS, error)
	// InstructionsSource optionally resolves rules in the host path namespace.
	InstructionsSource func(context.Context, clarafs.Path) (string, error)

	service *clarafs.Service
	store   *hashline.Store
	editor  *hashline.Editor
	mu      sync.Mutex
	rules   map[clarafs.Path]string
}

// New creates a filesystem tool set.
func NewFiles(reader clarafs.Reader, store *hashline.Store) *Files {
	if store == nil {
		store = hashline.NewStore()
	}
	storage, ok := reader.(clarafs.FS)
	if !ok {
		storage = clarafs.At(reader, "/")
	}
	service := clarafs.New(storage)
	return &Files{
		service: service, store: store, rules: map[clarafs.Path]string{},
		editor: &hashline.Editor{FS: storage, Store: store, Resolver: hashline.DefaultResolver{}},
	}
}

// Store returns the shared Hashline store.
func (s *Files) Store() *hashline.Store { return s.store }

// All returns every filesystem tool.
func (s *Files) All() []Tool {
	return []Tool{
		s.List(), s.Stat(), s.Instructions(), s.Read(), s.Write(), s.Edit(),
		s.Remove(), s.Move(), s.Copy(), s.Glob(), s.Grep(),
	}
}

func (s *Files) cleanPath(raw string) (clarafs.Path, error) {
	return clarafs.ResolvePath(s.service.FS, raw)
}
