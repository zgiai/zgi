package contentparse

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/contracts"
)

type defaultStrategyResolver struct {
	defaultAdapter string
	catalog        *contracts.ParseProviderCatalog
}

func NewDefaultStrategyResolver(catalog *contracts.ParseProviderCatalog, defaultAdapter string) StrategyResolver {
	return &defaultStrategyResolver{defaultAdapter: defaultAdapter, catalog: catalog}
}

func (r *defaultStrategyResolver) Resolve(req contracts.ParseRequest) (string, contracts.ParseRequest, error) {
	normalized, err := normalizeParseRequest(req)
	if err != nil {
		return "", req, err
	}
	if adapter := r.resolveAdapter(normalized); adapter != "" {
		return adapter, normalized, nil
	}

	return r.defaultAdapter, normalized, nil
}

func normalizeParseRequest(req contracts.ParseRequest) (contracts.ParseRequest, error) {
	normalized := req
	if normalized.SourceType == "" {
		normalized.SourceType = contracts.ParseSourceTypeBytes
	}
	if normalized.Intent == "" {
		normalized.Intent = contracts.ParseIntentPreview
	}
	if normalized.Profile == "" {
		normalized.Profile = contracts.ParseProfileDefault
	}
	if normalized.EngineHint == "" {
		normalized.EngineHint = contracts.ParseEngineLocal
	}
	if normalized.SourceType == contracts.ParseSourceTypeBytes && len(normalized.Data) == 0 {
		return normalized, fmt.Errorf("content parse request requires data bytes for source type %q", normalized.SourceType)
	}
	return normalized, nil
}

func (r *defaultStrategyResolver) resolveAdapter(req contracts.ParseRequest) string {
	if r.catalog == nil {
		return ""
	}
	for _, provider := range r.catalog.Providers {
		if !provider.Enabled {
			continue
		}
		if provider.Engine != "" && provider.Engine != req.EngineHint {
			continue
		}
		if strings.TrimSpace(provider.Adapter) == "" {
			continue
		}
		return provider.Adapter
	}
	return ""
}
