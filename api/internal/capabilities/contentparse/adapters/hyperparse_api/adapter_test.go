package hyperparseapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/zgiai/ginext/internal/contracts"
)

func TestAdapterParsesRemoteResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"ok": true,
			"result": {
				"doc_id": "remote-1",
				"file_name": "sample.pdf",
				"page_count": 1,
				"markdown": "hello remote",
				"source": "local",
				"extract_output": {
					"metadata": {"document_id":"doc-1"},
					"elements": [{"id":"c1","type":"text","page":0,"content":"hello remote","ordinal":1}]
				}
			}
		}`))
	}))
	defer server.Close()

	t.Setenv("CONTENT_PARSE_HYPERPARSE_API_BASE_URL", server.URL)
	t.Setenv("CONTENT_PARSE_HYPERPARSE_API_KEY", "test-key")

	adapter := NewAdapter()
	artifact, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "sample.pdf",
		Data:       []byte("%PDF-1.4"),
		Intent:     contracts.ParseIntentPreview,
		Profile:    contracts.ParseProfileFastPreview,
		EngineHint: contracts.ParseEngineLocal,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if artifact.ArtifactID != "remote-1" {
		t.Fatalf("ArtifactID = %q", artifact.ArtifactID)
	}
	if len(artifact.Elements) != 1 {
		t.Fatalf("elements=%d", len(artifact.Elements))
	}
}

func TestAdapterParsesChunksOnlyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"ok": true,
			"result": {
				"doc_id": "remote-2",
				"file_name": "scan.pdf",
				"page_count": 1,
				"markdown": "hello chunk only",
				"chunks": [{
					"id":"c1",
					"type":"text",
					"page":0,
					"text":"hello chunk only",
					"ordinal":1,
					"confidence":0.82
				}]
			}
		}`))
	}))
	defer server.Close()

	t.Setenv("CONTENT_PARSE_HYPERPARSE_API_BASE_URL", server.URL)
	adapter := NewAdapter()
	artifact, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "scan.pdf",
		Data:       []byte("%PDF-1.4"),
		Intent:     contracts.ParseIntentPreview,
		Profile:    contracts.ParseProfileFastPreview,
		EngineHint: contracts.ParseEngineLocal,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(artifact.Elements) != 1 {
		t.Fatalf("elements=%d", len(artifact.Elements))
	}
	if artifact.Elements[0].Confidence == nil || *artifact.Elements[0].Confidence != 0.82 {
		t.Fatalf("confidence=%v", artifact.Elements[0].Confidence)
	}
}

func TestAdapterHealth(t *testing.T) {
	t.Setenv("CONTENT_PARSE_HYPERPARSE_API_BASE_URL", "https://parse.example.com")
	adapter := NewAdapter()
	health, err := adapter.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.Name != "hyperparse_api" || !health.Available {
		t.Fatalf("health=%+v", health)
	}
}

func TestAdapterRequiresBytesForNow(t *testing.T) {
	t.Setenv("CONTENT_PARSE_HYPERPARSE_API_BASE_URL", "https://parse.example.com")
	adapter := NewAdapter()
	_, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeURL,
		SourceRef:  "https://example.com/doc.pdf",
		FileName:   "doc.pdf",
		Intent:     contracts.ParseIntentPreview,
		Profile:    contracts.ParseProfileFastPreview,
	})
	if err == nil {
		t.Fatal("expected error for non-byte source")
	}
}

func TestFirstNonEmptyEnv(t *testing.T) {
	t.Setenv("CONTENT_PARSE_HYPERPARSE_API_KEY", "")
	t.Setenv("HYPERPARSE_API_KEY", "fallback")
	if got := firstNonEmptyEnv("CONTENT_PARSE_HYPERPARSE_API_KEY", "HYPERPARSE_API_KEY"); got != "fallback" {
		t.Fatalf("got=%q", got)
	}
}

func TestTimeoutFromEnv(t *testing.T) {
	t.Setenv("CONTENT_PARSE_HYPERPARSE_API_TIMEOUT_SECONDS", "9")
	if got := timeoutFromEnv("CONTENT_PARSE_HYPERPARSE_API_TIMEOUT_SECONDS", 0); got.Seconds() != 9 {
		t.Fatalf("timeout=%v", got)
	}
}

func TestToRemoteEngine(t *testing.T) {
	if got := toRemoteEngine(contracts.ParseEngineVLM); got != "vlm" {
		t.Fatalf("got=%q", got)
	}
}

func TestAdapterSendsModeFromProfile(t *testing.T) {
	var gotMode string
	var gotProfile string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("MultipartReader() error = %v", err)
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("NextPart() error = %v", err)
			}
			body, _ := io.ReadAll(part)
			switch part.FormName() {
			case "mode":
				gotMode = strings.TrimSpace(string(body))
			case "profile":
				gotProfile = strings.TrimSpace(string(body))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"doc_id":"remote-mode","file_name":"scan.pdf","page_count":1,"markdown":"ok","chunks":[]}}`))
	}))
	defer server.Close()

	t.Setenv("CONTENT_PARSE_HYPERPARSE_API_BASE_URL", server.URL)
	adapter := NewAdapter()
	_, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "scan.pdf",
		Data:       []byte("%PDF-1.4"),
		Intent:     contracts.ParseIntentPreview,
		Profile:    contracts.ParseProfileHighQuality,
		EngineHint: contracts.ParseEngineLocal,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if gotMode != "strict" || gotProfile != string(contracts.ParseProfileHighQuality) {
		t.Fatalf("mode=%q profile=%q", gotMode, gotProfile)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}
