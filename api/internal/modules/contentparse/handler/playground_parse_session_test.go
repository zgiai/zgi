package handler

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/service"
)

func TestPlaygroundParseSessionCacheKeepsOnlyTrustedLightweightResult(t *testing.T) {
	cache := newPlaygroundParseSessionCache(time.Minute)
	workspaceID := uuid.New()
	exec := &playgroundExecution{
		Response: playgroundParseResponse{
			File: playgroundFileSummary{
				Name:   "sample.pdf",
				Size:   9,
				SHA256: "hash",
			},
		},
		RequestedProviderKey: "local",
		AdapterName:          "hyperparse_sdk",
		EffectiveRequest: contracts.ParseRequest{
			Profile: contracts.ParseProfileAuto,
			Data:    []byte("raw bytes must not be retained"),
		},
		SourceData: []byte("source bytes must not be retained"),
	}

	sessionID := cache.Store(service.PlaygroundRunListFilter{WorkspaceID: &workspaceID}, exec)
	if sessionID == "" {
		t.Fatal("expected parse session id")
	}

	otherWorkspaceID := uuid.New()
	if got, ok := cache.Get(sessionID, service.PlaygroundRunListFilter{WorkspaceID: &otherWorkspaceID}); ok || got != nil {
		t.Fatal("expected parse session to be scoped to the creating workspace")
	}

	got, ok := cache.Get(sessionID, service.PlaygroundRunListFilter{WorkspaceID: &workspaceID})
	if !ok || got == nil {
		t.Fatal("expected parse session for matching workspace")
	}
	if got.Response.ParseSessionID != sessionID {
		t.Fatalf("expected response parse_session_id %q, got %q", sessionID, got.Response.ParseSessionID)
	}
	if len(got.SourceData) != 0 {
		t.Fatal("expected cached execution to omit source bytes")
	}
	if len(got.EffectiveRequest.Data) != 0 {
		t.Fatal("expected cached execution to omit request bytes")
	}
}

func TestPlaygroundParseSessionCacheExpires(t *testing.T) {
	cache := newPlaygroundParseSessionCache(time.Nanosecond)
	sessionID := cache.Store(service.PlaygroundRunListFilter{}, &playgroundExecution{
		Response: playgroundParseResponse{File: playgroundFileSummary{SHA256: "hash"}},
	})
	time.Sleep(time.Millisecond)

	if got, ok := cache.Get(sessionID, service.PlaygroundRunListFilter{}); ok || got != nil {
		t.Fatal("expected expired parse session to be unavailable")
	}
}

func TestPlaygroundParseSessionCacheEvictsLeastRecentlyUsed(t *testing.T) {
	cache := newPlaygroundParseSessionCache(time.Minute)
	cache.maxEntries = 2
	firstID := cache.Store(service.PlaygroundRunListFilter{}, &playgroundExecution{
		Response: playgroundParseResponse{File: playgroundFileSummary{SHA256: "first"}},
	})
	secondID := cache.Store(service.PlaygroundRunListFilter{}, &playgroundExecution{
		Response: playgroundParseResponse{File: playgroundFileSummary{SHA256: "second"}},
	})
	if _, ok := cache.Get(firstID, service.PlaygroundRunListFilter{}); !ok {
		t.Fatal("expected first session to be accessible before eviction")
	}
	thirdID := cache.Store(service.PlaygroundRunListFilter{}, &playgroundExecution{
		Response: playgroundParseResponse{File: playgroundFileSummary{SHA256: "third"}},
	})

	if _, ok := cache.Get(secondID, service.PlaygroundRunListFilter{}); ok {
		t.Fatal("expected least recently used session to be evicted")
	}
	if _, ok := cache.Get(firstID, service.PlaygroundRunListFilter{}); !ok {
		t.Fatal("expected recently accessed session to remain")
	}
	if _, ok := cache.Get(thirdID, service.PlaygroundRunListFilter{}); !ok {
		t.Fatal("expected newest session to remain")
	}
}

func TestPlaygroundParseSessionCacheSkipsOversizedItems(t *testing.T) {
	cache := newPlaygroundParseSessionCache(time.Minute)
	cache.maxItemBytes = 64
	sessionID := cache.Store(service.PlaygroundRunListFilter{}, &playgroundExecution{
		Response: playgroundParseResponse{
			File: playgroundFileSummary{SHA256: "large"},
			Artifact: &contracts.ParseArtifact{
				Markdown: "this markdown makes the cached item larger than the configured item limit",
			},
		},
	})

	if sessionID != "" {
		t.Fatal("expected oversized parse result to skip session caching")
	}
}
