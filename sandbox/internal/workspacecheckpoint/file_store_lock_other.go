//go:build !linux

package workspacecheckpoint

import (
	"context"
	"sync"
)

var processLocks sync.Map

func (s *FileStore) lockKey(ctx context.Context, key string) (func(), error) {
	name := s.root.Name() + "/" + key
	value, _ := processLocks.LoadOrStore(name, make(chan struct{}, 1))
	lock := value.(chan struct{})
	select {
	case lock <- struct{}{}:
		return func() { <-lock }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
