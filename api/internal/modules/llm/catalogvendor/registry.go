package catalogvendor

import (
	"strings"
	"sync"

	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
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
	values     map[string]string
	normalized map[string]string
	metadata   map[string]Metadata
}{
	values:     make(map[string]string),
	normalized: make(map[string]string),
	metadata:   make(map[string]Metadata),
}

func key(provider, model string) string {
	return strings.TrimSpace(provider) + "\x00" + strings.TrimSpace(model)
}

// normalizedKey folds supplier/vendor spelling differences (for example
// "agicto" versus "agicto supplier") so a query can match the catalog row even
// when the two sides spell the same commercial supplier differently.
func normalizedKey(provider, model string) string {
	return channelprovider.NormalizeIdentityKey(provider) + "\x00" + strings.ToLower(strings.TrimSpace(model))
}

func storeEntry(values, normalized map[string]string, entry Entry) {
	vendor := strings.ToLower(strings.TrimSpace(entry.Vendor))
	if vendor == "" {
		return
	}
	values[key(entry.Provider, entry.Model)] = vendor
	if folded := normalizedKey(entry.Provider, entry.Model); folded != "\x00" {
		normalized[folded] = vendor
	}
}

// Replace atomically publishes the vendor projection from the latest
// successfully applied Console catalog. Vendor data is intentionally kept out
// of llm_models because it is display metadata owned by Console.
func Replace(entries []Entry) {
	values := make(map[string]string, len(entries))
	normalized := make(map[string]string, len(entries))
	for _, entry := range entries {
		storeEntry(values, normalized, entry)
	}

	registry.Lock()
	registry.values = values
	registry.normalized = normalized
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
	foldedProvider := channelprovider.NormalizeIdentityKey(provider)
	registry.Lock()
	for existingKey := range registry.values {
		if strings.HasPrefix(existingKey, provider+"\x00") {
			delete(registry.values, existingKey)
		}
	}
	for existingKey := range registry.normalized {
		if foldedProvider != "" && strings.HasPrefix(existingKey, foldedProvider+"\x00") {
			delete(registry.normalized, existingKey)
		}
	}
	for _, entry := range entries {
		if strings.TrimSpace(entry.Provider) != provider {
			continue
		}
		storeEntry(registry.values, registry.normalized, entry)
	}
	registry.Unlock()
}

func Lookup(provider, model string) string {
	registry.RLock()
	vendor := registry.values[key(provider, model)]
	if vendor == "" {
		vendor = registry.normalized[normalizedKey(provider, model)]
	}
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
