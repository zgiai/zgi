package service

import (
	contentparsecap "github.com/zgiai/zgi/api/internal/capabilities/contentparse"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

func RuntimeEnvOverridesForCandidate(catalog *contracts.ParseProviderCatalog, candidate routing.RouteCandidate) map[string]string {
	return contentparsecap.RuntimeEnvOverridesForCandidate(catalog, candidate)
}
