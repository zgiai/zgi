package external

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/middleware"
	"gopkg.in/yaml.v3"
)

func TestPublicDocumentationExposesDigitalHumanExternalContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterPublicRoutes(router.Group("/api"))

	docs := httptest.NewRecorder()
	router.ServeHTTP(docs, httptest.NewRequest(http.MethodGet, "/api/docs", nil))
	if docs.Code != http.StatusOK || !strings.Contains(docs.Body.String(), "/api/v1/digital-humans") ||
		!strings.Contains(docs.Body.String(), "/api/digital-human-openapi.yaml") {
		t.Fatalf("docs response = %d %s", docs.Code, docs.Body.String())
	}

	spec := httptest.NewRecorder()
	router.ServeHTTP(spec, httptest.NewRequest(http.MethodGet, "/api/digital-human-openapi.yaml", nil))
	if spec.Code != http.StatusOK || !strings.Contains(spec.Header().Get("Content-Type"), "application/yaml") {
		t.Fatalf("OpenAPI response = %d %q", spec.Code, spec.Header().Get("Content-Type"))
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec.Body.Bytes(), &document); err != nil {
		t.Fatalf("OpenAPI is invalid YAML: %v", err)
	}
	for _, required := range []string{
		"/digital-humans/chat:", "/digital-humans/runs/{run_id}/events:",
		"/digital-humans/conversations/{conversation_id}/messages:",
		"/digital-humans/files/upload:", "Last-Event-ID", "X-ZGI-Run-ID",
		"x-zgi-sse-event-data-schemas:", "run_accepted:", "run_end:",
	} {
		if !strings.Contains(spec.Body.String(), required) {
			t.Fatalf("OpenAPI document is missing %q", required)
		}
	}
	if strings.Contains(spec.Body.String(), "X-External-User-ID") || strings.Contains(spec.Body.String(), "end_user_id") {
		t.Fatal("DigitalHuman contract must use the user field instead of an identity header or renamed field")
	}
}

func TestPublicDocumentationRoutesExposeCurrentAgentContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterPublicRoutes(router.Group("/api"))

	docs := httptest.NewRecorder()
	router.ServeHTTP(docs, httptest.NewRequest(http.MethodGet, "/api/docs", nil))
	if docs.Code != http.StatusOK {
		t.Fatalf("docs status = %d, want %d", docs.Code, http.StatusOK)
	}
	for _, expected := range []string{"/api/openapi.yaml", "/swagger/index.html", "/api/v1/agents/config", "user-input", "sse_contract"} {
		if !strings.Contains(docs.Body.String(), expected) {
			t.Fatalf("docs body does not contain %q: %s", expected, docs.Body.String())
		}
	}

	spec := httptest.NewRecorder()
	router.ServeHTTP(spec, httptest.NewRequest(http.MethodGet, "/api/openapi.yaml", nil))
	if spec.Code != http.StatusOK {
		t.Fatalf("OpenAPI status = %d, want %d", spec.Code, http.StatusOK)
	}
	if contentType := spec.Header().Get("Content-Type"); !strings.Contains(contentType, "application/yaml") {
		t.Fatalf("OpenAPI Content-Type = %q, want application/yaml", contentType)
	}
	var openAPI map[string]interface{}
	if err := yaml.Unmarshal(spec.Body.Bytes(), &openAPI); err != nil {
		t.Fatalf("OpenAPI document is invalid YAML: %v", err)
	}
	if openAPI["openapi"] != "3.0.3" {
		t.Fatalf("OpenAPI version = %#v, want 3.0.3", openAPI["openapi"])
	}
	for _, expected := range []string{
		"/agents/config:",
		"/files/upload:",
		"/agents/chat:",
		"/agents/conversations/{conversation_id}/messages:",
		"/agents/messages/{message_id}/regenerate:",
		"/agents/conversations/{conversation_id}/messages/{message_id}/user-input/{request_id}/continue:",
		"x-zgi-sse-event-data-schemas:",
		"/agents/memory/{key}:",
		"X-External-User-ID",
		"ConversationListEnvelope",
	} {
		if !strings.Contains(spec.Body.String(), expected) {
			t.Fatalf("OpenAPI document does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{
		"client_action_required",
		"client_action_result",
		"/tool-governance/",
		"/client-actions/",
		"waiting_client_action",
	} {
		if strings.Contains(spec.Body.String(), forbidden) || strings.Contains(docs.Body.String(), forbidden) {
			t.Fatalf("Agent public documentation exposes sidebar-only or unreachable contract %q", forbidden)
		}
	}
	paths, ok := openAPI["paths"].(map[string]interface{})
	if !ok {
		t.Fatalf("OpenAPI paths has unexpected type %T", openAPI["paths"])
	}
	assertOpenAPIRefsResolve(t, openAPI)
	assertOpenAPIOperationsDocumented(t, paths)
	for _, operation := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/files/upload"},
		{http.MethodPost, "/agents/chat"},
		{http.MethodGet, "/agents/conversations"},
		{http.MethodGet, "/agents/conversations/search"},
		{http.MethodGet, "/agents/conversations/{conversation_id}"},
		{http.MethodPatch, "/agents/conversations/{conversation_id}"},
		{http.MethodDelete, "/agents/conversations/{conversation_id}"},
		{http.MethodGet, "/agents/conversations/{conversation_id}/messages"},
		{http.MethodGet, "/agents/conversations/{conversation_id}/events"},
		{http.MethodPost, "/agents/conversations/{conversation_id}/stop"},
		{http.MethodPost, "/agents/messages/{message_id}/regenerate"},
		{http.MethodPost, "/agents/conversations/{conversation_id}/messages/{message_id}/workflow-continuation"},
		{http.MethodPost, "/agents/conversations/{conversation_id}/messages/{message_id}/user-input/{request_id}/continue"},
		{http.MethodGet, "/agents/memory"},
		{http.MethodDelete, "/agents/memory"},
		{http.MethodGet, "/agents/memory/export"},
		{http.MethodPut, "/agents/memory/{key}"},
		{http.MethodDelete, "/agents/memory/{key}"},
		{http.MethodPost, "/agents/memory/operations/{operation_id}/undo"},
	} {
		pathItem, ok := paths[operation.path].(map[string]interface{})
		if !ok {
			t.Fatalf("OpenAPI path %s is missing", operation.path)
		}
		method := strings.ToLower(operation.method)
		methodItem, ok := pathItem[method].(map[string]interface{})
		if !ok {
			t.Fatalf("OpenAPI operation %s %s is missing", operation.method, operation.path)
		}
		if !hasOpenAPIParameterRef(pathItem["parameters"], "#/components/parameters/ExternalUserID") &&
			!hasOpenAPIParameterRef(methodItem["parameters"], "#/components/parameters/ExternalUserID") {
			t.Fatalf("OpenAPI operation %s %s does not require X-External-User-ID", operation.method, operation.path)
		}
	}
}

func assertOpenAPIOperationsDocumented(t *testing.T, paths map[string]interface{}) {
	t.Helper()
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			t.Fatalf("OpenAPI path %s has unexpected type %T", path, rawPathItem)
		}
		for method, rawOperation := range pathItem {
			switch strings.ToLower(method) {
			case "get", "post", "put", "patch", "delete":
			default:
				continue
			}
			operation, ok := rawOperation.(map[string]interface{})
			if !ok {
				t.Fatalf("OpenAPI operation %s %s has unexpected type %T", method, path, rawOperation)
			}
			for _, field := range []string{"summary", "description", "operationId"} {
				value, _ := operation[field].(string)
				if strings.TrimSpace(value) == "" {
					t.Errorf("OpenAPI operation %s %s is missing %s", strings.ToUpper(method), path, field)
				}
			}
			responses, ok := operation["responses"].(map[string]interface{})
			if !ok {
				t.Errorf("OpenAPI operation %s %s has no responses object", strings.ToUpper(method), path)
				continue
			}
			if _, ok := responses["default"]; !ok {
				t.Errorf("OpenAPI operation %s %s has no default error response", strings.ToUpper(method), path)
			}
			success, ok := responses["200"]
			if !ok {
				success, ok = responses["201"]
			}
			if !ok {
				t.Errorf("OpenAPI operation %s %s has no 200 or 201 response", strings.ToUpper(method), path)
				continue
			}
			successObject, ok := success.(map[string]interface{})
			if !ok {
				t.Errorf("OpenAPI success response %s %s has unexpected type %T", strings.ToUpper(method), path, success)
				continue
			}
			if _, hasRef := successObject["$ref"]; !hasRef {
				content, hasContent := successObject["content"].(map[string]interface{})
				if !hasContent || len(content) == 0 {
					t.Errorf("OpenAPI success response %s %s has neither $ref nor content", strings.ToUpper(method), path)
				}
			}
		}
	}
}

func assertOpenAPIRefsResolve(t *testing.T, document map[string]interface{}) {
	t.Helper()
	var walk func(interface{})
	walk = func(value interface{}) {
		switch typed := value.(type) {
		case map[string]interface{}:
			if ref, ok := typed["$ref"].(string); ok && strings.HasPrefix(ref, "#/") {
				var current interface{} = document
				for _, segment := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
					object, ok := current.(map[string]interface{})
					if !ok {
						t.Fatalf("OpenAPI reference %q does not resolve", ref)
					}
					current, ok = object[segment]
					if !ok {
						t.Fatalf("OpenAPI reference %q does not resolve", ref)
					}
				}
			}
			for _, child := range typed {
				walk(child)
			}
		case []interface{}:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(document)
}

func hasOpenAPIParameterRef(value interface{}, ref string) bool {
	parameters, _ := value.([]interface{})
	for _, parameter := range parameters {
		item, _ := parameter.(map[string]interface{})
		if item["$ref"] == ref {
			return true
		}
	}
	return false
}

func TestExternalAppMetadataUsesPublishedAgentConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agentID := uuid.New()
	webAppID := uuid.New()
	repository := &externalAgentRepositoryStub{agent: &agents.Agent{
		ID:          agentID,
		WebAppID:    webAppID,
		Name:        "Published Research Agent",
		Description: "Evidence-backed answers",
		AgentsType:  "AGENT",
	}}
	runtime := &externalAgentRuntimeStub{config: &dto.AgentWebAppRuntimeConfigResponse{
		AgentID:     agentID.String(),
		WebAppID:    webAppID.String(),
		AgentType:   "AGENT",
		Version:     "v12",
		VersionUUID: uuid.NewString(),
		Config: dto.AgentConfigResponse{
			HomeTitle:          "Start your research",
			OpeningStatement:   "What should I investigate?",
			InputPlaceholder:   "Ask a question",
			SuggestedQuestions: []string{"Compare the evidence"},
			FileUpload:         true,
			AgentMemoryEnabled: true,
			AgentMemorySlots:   []dto.AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
		},
	}}
	handler := &ExternalWorkflowHandler{agentRepository: repository, agentRuntime: runtime}

	info := httptest.NewRecorder()
	infoContext, _ := gin.CreateTestContext(info)
	infoContext.Request = httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	setExternalAPIKeyInfo(infoContext, agentID)
	handler.GetAppInfo(infoContext)
	if info.Code != http.StatusOK || !strings.Contains(info.Body.String(), "Published Research Agent") || !strings.Contains(info.Body.String(), "v12") {
		t.Fatalf("info response = %d %s", info.Code, info.Body.String())
	}

	parameters := httptest.NewRecorder()
	parameterContext, _ := gin.CreateTestContext(parameters)
	parameterContext.Request = httptest.NewRequest(http.MethodGet, "/api/v1/parameters", nil)
	setExternalAPIKeyInfo(parameterContext, agentID)
	handler.GetAppParameters(parameterContext)
	if parameters.Code != http.StatusOK {
		t.Fatalf("parameters status = %d, body=%s", parameters.Code, parameters.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(parameters.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode parameters: %v", err)
	}
	features, _ := body["features"].(map[string]interface{})
	memory, _ := body["agent_memory"].(map[string]interface{})
	fileUpload, _ := body["file_upload"].(map[string]interface{})
	if features["home_title"] != "Start your research" || memory["requires_user"] != true || fileUpload["enabled"] != true {
		t.Fatalf("published parameters = %#v", body)
	}
}

type externalAgentRepositoryStub struct {
	agent *agents.Agent
}

func (s *externalAgentRepositoryStub) GetByID(context.Context, string) (*agents.Agent, error) {
	return s.agent, nil
}

type externalAgentRuntimeStub struct {
	config *dto.AgentWebAppRuntimeConfigResponse
}

func (s *externalAgentRuntimeStub) GetPublishedAgentRuntimeConfig(context.Context, string) (*dto.AgentWebAppRuntimeConfigResponse, error) {
	return s.config, nil
}

func setExternalAPIKeyInfo(c *gin.Context, agentID uuid.UUID) {
	c.Set("api_key_info", &middleware.APIKeyInfo{
		ID:       uuid.New(),
		AgentID:  agentID,
		TenantID: uuid.New(),
	})
}
