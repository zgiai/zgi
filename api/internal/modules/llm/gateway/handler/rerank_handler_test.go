package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/types"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type rerankStubService struct {
	types.GatewayService

	caps    types.ModelCapabilities
	calls   int
	lastReq *adapter.RerankRequest
}

func (s *rerankStubService) ModelCapabilities(context.Context, string) (types.ModelCapabilities, error) {
	return s.caps, nil
}

func (s *rerankStubService) Rerank(_ context.Context, _ *apikeymodel.TenantAPIKey, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	s.calls++
	s.lastReq = req
	return &adapter.RerankResponse{
		Object:  "rerank",
		Model:   req.Model,
		Results: []adapter.RerankResult{{Index: 1, RelevanceScore: 0.9}, {Index: 0, RelevanceScore: 0.1}},
	}, nil
}

// newRerankTestRouter mirrors the public /v1 group: body limit, API key auth,
// then the handler. withKey replaces the key lookup with a resolved key so the
// handler itself can be exercised without a repository.
func newRerankTestRouter(t *testing.T, service types.GatewayService, withKey bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	projector := testApplicationErrorProjector(t)
	router := gin.New()
	v1 := router.Group("/v1")
	v1.Use(GatewayBodyLimit(0, projector))
	if withKey {
		v1.Use(func(c *gin.Context) {
			c.Set("llm_api_key", &apikeymodel.TenantAPIKey{ID: "key-1", OrganizationID: "00000000-0000-0000-0000-000000000001"})
		})
	} else {
		v1.Use(LLMAPIKeyAuthMiddleware(nil, projector))
	}
	v1.POST("/rerank", NewLLMHandler(service, projector).Rerank)
	return router
}

func postRerank(router *gin.Engine, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/v1/rerank", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

const validRerankBody = `{"model":"qwen3-rerank","query":"capital of France","documents":["Berlin","Paris"],"top_n":2}`

func TestRerankWithoutAPIKeyIsUnauthorized(t *testing.T) {
	service := &rerankStubService{}
	recorder := postRerank(newRerankTestRouter(t, service, false), validRerankBody)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", recorder.Code, recorder.Body.String())
	}
	if service.calls != 0 {
		t.Fatalf("Rerank called %d times without a key", service.calls)
	}
}

func TestRerankRejectsInvalidBody(t *testing.T) {
	for _, tc := range []struct {
		name, body string
	}{
		{"malformed json", `{"model":`},
		{"missing model", `{"query":"q","documents":["a"]}`},
		{"missing query", `{"model":"m","documents":["a"]}`},
		{"empty documents", `{"model":"m","query":"q","documents":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &rerankStubService{}
			recorder := postRerank(newRerankTestRouter(t, service, true), tc.body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", recorder.Code, recorder.Body.String())
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode %s: %v", recorder.Body.String(), err)
			}
			if payload["error_code"] != "llm.request.invalid" {
				t.Fatalf("error_code = %v, want llm.request.invalid", payload["error_code"])
			}
			if service.calls != 0 {
				t.Fatalf("Rerank called %d times for an invalid body", service.calls)
			}
		})
	}
}

func TestValidatePublicRerankRequest(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, body, message string
	}{
		{"valid", validRerankBody, ""},
		{"object documents", `{"model":"m","query":"q","documents":[{"text":"a"}]}`, ""},
		{"missing model", `{"query":"q","documents":["a"]}`, "model is required"},
		{"blank query", `{"model":"m","query":" ","documents":["a"]}`, "query is required"},
		{"missing documents", `{"model":"m","query":"q"}`, "documents must contain at least one item"},
		{"empty documents", `{"model":"m","query":"q","documents":[]}`, "documents must contain at least one item"},
		{"documents not a list", `{"model":"m","query":"q","documents":"a"}`, "documents must contain at least one item"},
		{"non-positive top_n", `{"model":"m","query":"q","documents":["a"],"top_n":0}`, "top_n must be a positive integer"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var req adapter.RerankRequest
			if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
				t.Fatal(err)
			}
			err := validatePublicRerankRequest(&req)
			if tc.message == "" {
				if err != nil {
					t.Fatalf("validatePublicRerankRequest() = %v, want nil", err)
				}
				return
			}
			if got := publicInputMessage(err); got != tc.message {
				t.Fatalf("validatePublicRerankRequest() message = %q, want %q", got, tc.message)
			}
		})
	}
}

func TestRerankForwardsToGatewayService(t *testing.T) {
	t.Setenv(gateway.CapabilityGateEnv, "")
	service := &rerankStubService{caps: types.ModelCapabilities{Known: true, UseCasesDeclared: true, Rerank: true}}
	recorder := postRerank(newRerankTestRouter(t, service, true), validRerankBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", recorder.Code, recorder.Body.String())
	}
	if service.calls != 1 {
		t.Fatalf("Rerank called %d times, want 1", service.calls)
	}
	if service.lastReq.Model != "qwen3-rerank" || service.lastReq.Query != "capital of France" ||
		service.lastReq.TopN == nil || *service.lastReq.TopN != 2 {
		t.Fatalf("forwarded request = %+v", service.lastReq)
	}
	var payload adapter.RerankResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
	if len(payload.Results) != 2 || payload.Results[0].Index != 1 {
		t.Fatalf("results = %+v", payload.Results)
	}
}

func TestRerankRejectsModelWithoutRerankCapability(t *testing.T) {
	t.Setenv(gateway.CapabilityGateEnv, "")
	service := &rerankStubService{caps: types.ModelCapabilities{Known: true, UseCasesDeclared: true}}
	recorder := postRerank(newRerankTestRouter(t, service, true), validRerankBody)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "unsupported_capability") {
		t.Fatalf("body = %s, want unsupported_capability", recorder.Body.String())
	}
	if service.calls != 0 {
		t.Fatalf("Rerank called %d times for a model without rerank", service.calls)
	}
}

func TestRerankCapabilityGateFailsOpen(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  string
		caps types.ModelCapabilities
	}{
		{"unknown model", "", types.ModelCapabilities{}},
		{"legacy row without use cases", "", types.ModelCapabilities{Known: true}},
		{"gate disabled", "off", types.ModelCapabilities{Known: true, UseCasesDeclared: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(gateway.CapabilityGateEnv, tc.env)
			service := &rerankStubService{caps: tc.caps}
			recorder := postRerank(newRerankTestRouter(t, service, true), validRerankBody)
			if recorder.Code != http.StatusOK || service.calls != 1 {
				t.Fatalf("status = %d calls = %d, want 200 and 1 call; body = %s", recorder.Code, service.calls, recorder.Body.String())
			}
		})
	}
}
