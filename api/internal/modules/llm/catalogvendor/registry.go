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

type Metadata struct {
	Vendor      string
	VendorName  string
	CNName      string
	ENName      string
	Description string
	Website     string
	CountryCode string
}

var registry = struct {
	sync.RWMutex
	values   map[string]string
	metadata map[string]Metadata
}{values: make(map[string]string), metadata: make(map[string]Metadata)}

func key(provider, model string) string {
	return strings.TrimSpace(provider) + "\x00" + strings.TrimSpace(model)
}

// Replace atomically publishes the vendor projection from the latest
// successfully applied Console catalog. Vendor data is intentionally kept out
// of llm_models because it is display metadata owned by Console.
func Replace(entries []Entry) {
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		vendor := strings.ToLower(strings.TrimSpace(entry.Vendor))
		if vendor == "" {
			continue
		}
		values[key(entry.Provider, entry.Model)] = vendor
	}

	registry.Lock()
	registry.values = values
	registry.Unlock()
}

// ReplaceMetadata atomically publishes display metadata owned by Console.
func ReplaceMetadata(entries []Metadata) {
	metadata := make(map[string]Metadata, len(entries))
	for _, entry := range entries {
		vendor := strings.TrimSpace(entry.Vendor)
		if vendor == "" {
			continue
		}
		entry.Vendor = vendor
		metadata[vendor] = entry
	}

	registry.Lock()
	registry.metadata = metadata
	registry.Unlock()
}

// ReplaceProvider refreshes one provider from a ModelMeta source without
// discarding vendor data already loaded for other providers.
func ReplaceProvider(provider string, entries []Entry) {
	provider = strings.TrimSpace(provider)
	registry.Lock()
	for existingKey := range registry.values {
		if strings.HasPrefix(existingKey, provider+"\x00") {
			delete(registry.values, existingKey)
		}
	}
	for _, entry := range entries {
		vendor := strings.TrimSpace(entry.Vendor)
		if vendor == "" || strings.TrimSpace(entry.Provider) != provider {
			continue
		}
		registry.values[key(entry.Provider, entry.Model)] = vendor
	}
	registry.Unlock()
}

func Lookup(provider, model string) string {
	registry.RLock()
	vendor := registry.values[key(provider, model)]
	registry.RUnlock()
	return vendor
}

func LookupMetadata(vendor string) (Metadata, bool) {
	registry.RLock()
	metadata, ok := registry.metadata[strings.ToLower(strings.TrimSpace(vendor))]
	registry.RUnlock()
	return metadata, ok
}

func Enrich(provider, model string, target *string) {
	if target == nil {
		return
	}
	if vendor := Lookup(provider, model); vendor != "" {
		*target = vendor
	}
}
