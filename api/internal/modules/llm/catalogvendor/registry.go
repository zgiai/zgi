package catalogvendor

import (
	"strings"
	"sync"
)

type Entry struct {
	Provider string
	Model    string
	Vendor   string
}

var registry = struct {
	sync.RWMutex
	values map[string]string
}{values: make(map[string]string)}

func key(provider, model string) string {
	return strings.TrimSpace(provider) + "\x00" + strings.TrimSpace(model)
}

// Replace atomically publishes the vendor projection from the latest
// successfully applied Console catalog. Vendor data is intentionally kept out
// of llm_models because it is display metadata owned by Console.
func Replace(entries []Entry) {
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		vendor := strings.TrimSpace(entry.Vendor)
		if vendor == "" {
			continue
		}
		values[key(entry.Provider, entry.Model)] = vendor
	}

	registry.Lock()
	registry.values = values
	registry.Unlock()
}

func Lookup(provider, model string) string {
	registry.RLock()
	vendor := registry.values[key(provider, model)]
	registry.RUnlock()
	return vendor
}

func Enrich(provider, model string, target *string) {
	if target == nil {
		return
	}
	if vendor := Lookup(provider, model); vendor != "" {
		*target = vendor
	}
}
