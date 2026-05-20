package storage

import (
	"context"

	"github.com/zgiai/zgi/runner/internal/plugin"
)

// Store abstracts how plugin packages are persisted and expanded.
type Store interface {
	SavePackage(ctx context.Context, manifest plugin.Manifest, pkg []byte) (string, error)
	Remove(ctx context.Context, manifest plugin.Manifest) error
	Workspace(manifest plugin.Manifest) string
}
