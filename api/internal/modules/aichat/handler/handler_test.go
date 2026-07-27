package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestRegisterRoutesDoesNotConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api")

	NewHandler(nil, nil).RegisterRoutes(group)
}

func TestSkillManagementRoutesAllowOrganizationMembers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := &capturingSkillManagementService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", uuid.NewString())
		util.SetOrganizationID(c, uuid.NewString())
		c.Next()
	})
	NewHandler(capture, nil).RegisterRoutes(router.Group("/console/api"))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/console/api/aichat/skills/custom-skill/delete-impact",
		nil,
	)
	router.ServeHTTP(recorder, request)

	if !capture.called {
		t.Fatal("PreviewSkillDeleteImpact() was not called for an organization member")
	}
}

type capturingSkillManagementService struct {
	runtimeservice.Service
	called bool
}

func (s *capturingSkillManagementService) PreviewSkillDeleteImpact(
	_ context.Context,
	_ runtimeservice.Scope,
	_ string,
) (*agentbindings.Impact, error) {
	s.called = true
	return nil, runtimeservice.ErrInvalidInput
}

func TestChatEndpointsFixRuntimeSurface(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name        string
		path        string
		bodySurface string
		wantSurface string
	}{
		{name: "legacy chat is work chat", path: "/console/api/aichat/chat", bodySurface: runtimedto.RuntimeSurfaceContextualSidebar, wantSurface: runtimedto.RuntimeSurfaceWorkChat},
		{name: "work chat", path: "/console/api/aichat/work-chat/chat", bodySurface: runtimedto.RuntimeSurfaceContextualSidebar, wantSurface: runtimedto.RuntimeSurfaceWorkChat},
		{name: "contextual sidebar", path: "/console/api/aichat/contextual/chat", bodySurface: runtimedto.RuntimeSurfaceWorkChat, wantSurface: runtimedto.RuntimeSurfaceContextualSidebar},
	} {
		t.Run(tt.name, func(t *testing.T) {
			capture := &capturingChatSurfaceService{}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("account_id", uuid.NewString())
				util.SetOrganizationID(c, uuid.NewString())
				c.Next()
			})
			NewHandler(capture, nil).RegisterRoutes(router.Group("/console/api"))

			body := []byte(`{"query":"test","model":"test-model","surface":"` + tt.bodySurface + `"}`)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			if !capture.called {
				t.Fatal("PrepareChat() was not called")
			}
			if capture.request.Surface != tt.wantSurface {
				t.Fatalf("surface = %q, want %q", capture.request.Surface, tt.wantSurface)
			}
		})
	}
}

type capturingChatSurfaceService struct {
	runtimeservice.Service
	called  bool
	request runtimedto.ChatRequest
}

func (s *capturingChatSurfaceService) PrepareChat(_ context.Context, _ runtimeservice.Scope, req runtimedto.ChatRequest) (*runtimeservice.PreparedChat, error) {
	s.called = true
	s.request = req
	return nil, runtimeservice.ErrInvalidInput
}

func TestSkillResponsePreservesDisplayTaxonomy(t *testing.T) {
	metadata := skills.SkillDiscoveryMetadata{
		ID:             "taxonomy-test",
		DependencyType: skills.SkillDependencyIntegration,
		IntegrationRequirements: []skills.SkillIntegrationRequirement{{
			IntegrationID: "github",
			ActionIDs:     []string{"github.repository.list"},
			Required:      true,
		}},
		Display: skills.SkillDisplayMetadata{
			Category:  "document_processing",
			Scenarios: []string{"document_handling", "legal_compliance"},
		},
	}

	response := skillResponse(metadata)
	if response.Display.Category != metadata.Display.Category {
		t.Fatalf("category = %q, want %q", response.Display.Category, metadata.Display.Category)
	}
	if !reflect.DeepEqual(response.Display.Scenarios, metadata.Display.Scenarios) {
		t.Fatalf("scenarios = %#v, want %#v", response.Display.Scenarios, metadata.Display.Scenarios)
	}
	if response.DependencyType != skills.SkillDependencyIntegration {
		t.Fatalf("dependency type = %q, want %q", response.DependencyType, skills.SkillDependencyIntegration)
	}
	if len(response.IntegrationRequirements) != 1 || response.IntegrationRequirements[0].IntegrationID != "github" || !response.IntegrationRequirements[0].Required {
		t.Fatalf("integration requirements = %#v", response.IntegrationRequirements)
	}
	if !reflect.DeepEqual(response.IntegrationRequirements[0].ActionIDs, []string{"github.repository.list"}) {
		t.Fatalf("integration action ids = %#v", response.IntegrationRequirements[0].ActionIDs)
	}
}

func TestMessageResponseRedactsModelInvocationMetadata(t *testing.T) {
	message := &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Query:          "q",
		Answer:         "a",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "vision-model",
		Metadata: map[string]interface{}{
			"model_invocations": []interface{}{
				map[string]interface{}{
					"request": map[string]interface{}{
						"messages": []interface{}{
							"data:image/jpeg;base64,raw-body",
						},
					},
				},
			},
			"generated_files": []interface{}{
				map[string]interface{}{"file_id": "file-1"},
			},
		},
	}

	resp := messageResponse(message)
	if _, ok := resp.Metadata["model_invocations"]; ok {
		t.Fatalf("metadata = %#v, should not expose model_invocations", resp.Metadata)
	}
	if resp.Metadata["model_invocations_redacted"] != true {
		t.Fatalf("metadata = %#v, want model_invocations_redacted marker", resp.Metadata)
	}
	if resp.Metadata["model_invocation_count"] != 1 {
		t.Fatalf("metadata = %#v, want model_invocation_count=1", resp.Metadata)
	}
	if _, ok := resp.Metadata["generated_files"]; !ok {
		t.Fatalf("metadata = %#v, should preserve lightweight message metadata", resp.Metadata)
	}
}

func TestMessageResponseFiltersFinalAnswerInvocationMetadata(t *testing.T) {
	message := &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Query:          "q",
		Answer:         "a",
		Status:         runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{"kind": "final_answer", "tool_name": "submit_final_answer"},
				map[string]interface{}{"kind": "tool_call", "skill_id": "file-reader", "tool_name": "read_file"},
			},
		},
	}

	resp := messageResponse(message)
	invocations, _ := resp.Metadata["skill_invocations"].([]interface{})
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want only one user-visible invocation", resp.Metadata["skill_invocations"])
	}
	invocation, _ := invocations[0].(map[string]interface{})
	if invocation["kind"] != "tool_call" {
		t.Fatalf("skill_invocations = %#v, want final_answer filtered", invocations)
	}
}

func TestMessageResponseRedactsExternalActionArgumentsFromHistory(t *testing.T) {
	message := &runtimemodel.Message{
		ID: uuid.New(), ConversationID: uuid.New(), Status: runtimemodel.MessageStatusWaitingApproval,
		Metadata: map[string]interface{}{"skill_invocations": []interface{}{map[string]interface{}{
			"kind": "tool_governance", "skill_id": "external-apps", "tool_name": "execute_action",
			"governance": map[string]interface{}{"frozen_invocation": map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "provider_id": "external-integrations",
				"arguments": map[string]interface{}{
					"integration_id": "github", "action_id": "github.issue.create", "connection_id": "connection-a",
					"arguments": map[string]interface{}{"body": "xoxb-12345678901234567890", "title": "safe title"},
				},
			}},
		}}},
	}

	response := messageResponse(message)
	encoded, err := json.Marshal(response.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "xoxb-") || !strings.Contains(string(encoded), "__zgi_redacted__") || !strings.Contains(string(encoded), "safe title") {
		t.Fatalf("public history metadata = %s", encoded)
	}
	original, _ := json.Marshal(message.Metadata)
	if !strings.Contains(string(original), "xoxb-") {
		t.Fatal("message response projection mutated stored metadata")
	}
}
