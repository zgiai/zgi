package modelmeta

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// The curated ZGI Models service sends Unix timestamps for date fields while the
// documented schema uses strings. Both must decode, otherwise one numeric field
// fails the whole page and the provider cannot sync.
func TestModelMetaDateAcceptsTimestampsAndStrings(t *testing.T) {
	payload := `{"object":"list","data":[
		{"provider":"openai","model":"gpt-5.6","knowledge_cutoff":1771200000,"release_date":1783555200},
		{"provider":"openai","model":"legacy","knowledge_cutoff":"2024-07","release_date":"2024-07-18"},
		{"provider":"openai","model":"milliseconds","knowledge_cutoff":1771200000000},
		{"provider":"openai","model":"empty","knowledge_cutoff":null,"release_date":""}
	]}`

	var page ModelMetaResponse
	require.NoError(t, json.Unmarshal([]byte(payload), &page))
	require.Len(t, page.Data, 4)

	require.Equal(t, "2026-02-16", page.Data[0].KnowledgeCutoff.String())
	require.Equal(t, "2026-07-09", page.Data[0].ReleaseDate.String())
	require.Equal(t, "2024-07", page.Data[1].KnowledgeCutoff.String())
	require.Equal(t, "2024-07-18", page.Data[1].ReleaseDate.String())
	require.Equal(t, "2026-02-16", page.Data[2].KnowledgeCutoff.String())
	require.Equal(t, "", page.Data[3].KnowledgeCutoff.String())
	require.Equal(t, "", page.Data[3].ReleaseDate.String())
}

func TestNormalizeModelMetaDateKeepsUnknownValues(t *testing.T) {
	require.Equal(t, "2024-07", normalizeModelMetaDate("2024-07"))
	require.Equal(t, "20241001", normalizeModelMetaDate("20241001"))
	require.Equal(t, "", normalizeModelMetaDate("   "))
	require.Equal(t, "2026-09-03", normalizeModelMetaDate("1788393600"))
}

// Reproduces the failure that surfaced when the source moved to models.zgi.ai:
// the page carries numeric knowledge_cutoff/release_date values.
func TestFetchProviderModelsAcceptsNumericDates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		require.Equal(t, "openai", r.URL.Query().Get("provider"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"provider":"openai","model":"gpt-5.6","model_name":"GPT-5.6","status":"active","knowledge_cutoff":1771200000,"release_date":1783555200}],"page":1,"page_size":100,"total":1,"total_pages":1}`))
	}))
	t.Cleanup(server.Close)

	svc := &Service{
		apiBaseURL: server.URL,
		httpClient: server.Client(),
	}

	models, err := svc.fetchProviderModels("openai")

	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "2026-02-16", models[0].KnowledgeCutoff.String())
	require.Equal(t, "2026-07-09", models[0].ReleaseDate.String())
}
