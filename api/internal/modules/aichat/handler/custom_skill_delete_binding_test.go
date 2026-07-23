package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/util"
)

type customSkillDeleteBindingService struct {
	runtimeservice.Service
	action      string
	impactToken string
	err         error
	preview     *agentbindings.Impact
}

func (s *customSkillDeleteBindingService) PreviewSkillDeleteImpact(context.Context, runtimeservice.Scope, string) (*agentbindings.Impact, error) {
	return s.preview, nil
}

func (s *customSkillDeleteBindingService) DeleteSkill(_ context.Context, _ runtimeservice.Scope, _, action, impactToken string) error {
	s.action = action
	s.impactToken = impactToken
	return s.err
}

func TestPreviewSkillDeleteImpactReturnsAffectedAgentPresentation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &customSkillDeleteBindingService{preview: &agentbindings.Impact{
		Code:        agentbindings.ConflictCodeResourceBound,
		ImpactToken: "preview-token",
		Agents: []agentbindings.ImpactAgent{{
			AgentID:     uuid.NewString(),
			Name:        "Operations assistant",
			Description: "Uses the custom Skill",
		}},
	}}
	handler := NewHandler(service)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/aichat/skills/custom-skill/delete-impact", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "custom-skill"}}
	ctx.Set("account_id", uuid.NewString())
	util.SetOrganizationID(ctx, uuid.NewString())

	handler.PreviewSkillDeleteImpact(ctx)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Operations assistant") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDeleteSkillPassesBindingConfirmationAndMapsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &customSkillDeleteBindingService{err: &agentbindings.ConflictError{Impact: agentbindings.Impact{
		Code:        agentbindings.ConflictCodeResourceBound,
		Operation:   "delete_custom_skill",
		BindingType: agentbindings.BindingTypeSkill,
		ResourceID:  "custom-skill",
		ImpactToken: "fresh-token",
	}}}
	handler := NewHandler(service)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/aichat/skills/custom-skill?agent_binding_action=unbind&impact_token=stale-token", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "custom-skill"}}
	ctx.Set("account_id", uuid.NewString())
	util.SetOrganizationID(ctx, uuid.NewString())

	handler.DeleteSkill(ctx)

	if service.action != "unbind" || service.impactToken != "stale-token" {
		t.Fatalf("binding confirmation = (%q, %q), want (unbind, stale-token)", service.action, service.impactToken)
	}
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
