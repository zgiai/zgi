package digitalhuman

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestExaDirectSkillSearchKeepsCredentialAndContentOutOfTrace(t *testing.T) {
	const token = "exa-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("x-api-key") != token {
			t.Fatalf("request = %s, token = %q", request.Method, request.Header.Get("x-api-key"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["query"] != "latest ZGI release" || body["numResults"] != float64(3) {
			t.Fatalf("body = %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"requestId":"request-1","results":[{"title":"Release","url":"https://example.com/release","text":"current release evidence"}]}`))
	}))
	defer server.Close()

	provider := &ExaDirectSkillProvider{httpClient: server.Client(), endpoint: server.URL}
	tool := provider.searchTool(map[string]string{"api_token": token})
	result := tool.Handler(t.Context(), adapter.ToolCall{ID: "call-1", Function: adapter.FunctionCall{
		Name: exaWebSearchToolName, Arguments: `{"query":"latest ZGI release","num_results":3}`,
	}})
	if result.Error != nil || result.Status != "success" {
		t.Fatalf("result = %#v, error = %v", result, result.Error)
	}
	persisted, _ := json.Marshal(map[string]any{"arguments": result.Arguments, "result": result.Result})
	for _, secret := range []string{token, "latest ZGI release", "current release evidence"} {
		if strings.Contains(string(persisted), secret) {
			t.Fatalf("persisted trace leaks %q: %s", secret, persisted)
		}
	}
	modelResult, _ := json.Marshal(result.ModelResult)
	if !strings.Contains(string(modelResult), "current release evidence") || !strings.Contains(string(modelResult), "https://example.com/release") {
		t.Fatalf("model result = %s", modelResult)
	}
}

func TestExaDirectSkillMapsCredentialFailureWithoutProviderBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":"upstream secret diagnostic"}`))
	}))
	defer server.Close()

	provider := &ExaDirectSkillProvider{httpClient: server.Client(), endpoint: server.URL}
	result := provider.searchTool(map[string]string{"api_token": "bad-token"}).Handler(
		t.Context(),
		adapter.ToolCall{Function: adapter.FunctionCall{Name: exaWebSearchToolName, Arguments: `{"query":"test"}`}},
	)
	encoded, _ := json.Marshal(result)
	if result.Error == nil || result.Recoverable || !strings.Contains(string(encoded), "invalid_token") {
		t.Fatalf("result = %#v", result)
	}
	if strings.Contains(string(encoded), "upstream secret diagnostic") || strings.Contains(string(encoded), "bad-token") {
		t.Fatalf("failure leaks upstream details: %s", encoded)
	}
}

func TestExaDirectSkillCandidateIsDigitalHumanOwned(t *testing.T) {
	provider := &ExaDirectSkillProvider{}
	items, err := provider.Candidates()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ResourceID != DirectSkillExaWebSearch || items[0].Metadata["source"] != "digital_human_direct" {
		t.Fatalf("candidates = %#v", items)
	}
	display, ok := items[0].Metadata["display"].(map[string]any)
	if !ok {
		t.Fatalf("display = %#v", items[0].Metadata["display"])
	}
	labels, ok := display["label"].(map[string]string)
	if !ok || labels["zh_Hans"] != "Exa 网页搜索" || labels["en_US"] != "Exa Web Search" {
		t.Fatalf("labels = %#v", display["label"])
	}
}
