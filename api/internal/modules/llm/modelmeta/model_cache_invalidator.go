package modelmeta

import (
	"context"
	"sync"
)

type ModelCacheInvalidator interface {
	InvalidateModelCache(ctx context.Context)
}

var modelCacheInvalidatorRegistry struct {
	sync.RWMutex
	invalidator ModelCacheInvalidator
}

func SetModelCacheInvalidator(invalidator ModelCacheInvalidator) {
	modelCacheInvalidatorRegistry.Lock()
	defer modelCacheInvalidatorRegistry.Unlock()
	modelCacheInvalidatorRegistry.invalidator = invalidator
}

func currentModelCacheInvalidator() ModelCacheInvalidator {
	modelCacheInvalidatorRegistry.RLock()
	defer modelCacheInvalidatorRegistry.RUnlock()
	return modelCacheInvalidatorRegistry.invalidator
}
