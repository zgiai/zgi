package modelmeta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	appconfig "github.com/zgiai/zgi/api/config"
)

func TestNewServiceDefaultsToZGICuratedModelSource(t *testing.T) {
	originalConfig := appconfig.GlobalConfig
	t.Cleanup(func() {
		appconfig.GlobalConfig = originalConfig
	})
	appconfig.GlobalConfig = &appconfig.Config{}

	svc := NewService(nil)

	require.Equal(t, "https://models.zgi.ai/v1", svc.apiBaseURL)
}

func TestSyncAllProvidersUsesRemoteProviderDiscovery(t *testing.T) {
	var requestedModelsPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/providers":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"provider":"qwen"}]}`))
		case "/providers/qwen/models":
			requestedModelsPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[],"page":1,"page_size":100,"total":0,"total_pages":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	svc := &Service{
		apiBaseURL: server.URL,
		httpClient: server.Client(),
	}

	results, err := svc.SyncAllProviders(context.Background())

	require.NoError(t, err)
	require.Contains(t, results, "qwen")
	require.Len(t, results, 1)
	require.Equal(t, "/providers/qwen/models", requestedModelsPath)
}
