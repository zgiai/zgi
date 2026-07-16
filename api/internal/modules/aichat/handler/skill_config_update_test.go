package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/util"
)

type skillConfigUpdateHandlerService struct {
	runtimeservice.Service
	config  *runtimeservice.SkillConfig
	err     error
	request runtimedto.UpdateSkillConfigRequest
}

func (s *skillConfigUpdateHandlerService) UpdateSkillConfig(
	_ context.Context,
	_ runtimeservice.Scope,
	req runtimedto.UpdateSkillConfigRequest,
) (*runtimeservice.SkillConfig, error) {
	s.request = req
	return s.config, s.err
}

func TestUpdateSkillConfigReturnsAppliedBusinessResult(t *testing.T) {
	service := &skillConfigUpdateHandlerService{config: &runtimeservice.SkillConfig{
		EnabledSkillIDs: []string{"time"},
	}}
	recorder := runUpdateSkillConfigRequest(t, service, `{"enabled_skill_ids":["time"]}`)

	var body struct {
		Code string `json:"code"`
		Data struct {
			Status          string   `json:"status"`
			Applied         bool     `json:"applied"`
			EnabledSkillIDs []string `json:"enabled_skill_ids"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusOK || body.Code != "0" || !body.Data.Applied || body.Data.Status != skillConfigUpdateStatusApplied {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if len(body.Data.EnabledSkillIDs) != 1 || body.Data.EnabledSkillIDs[0] != "time" {
		t.Fatalf("enabled_skill_ids = %#v, want [time]", body.Data.EnabledSkillIDs)
	}
}

func TestUpdateSkillConfigReturnsConfirmationRequiredAsSuccess(t *testing.T) {
	impact := agentbindings.Impact{
		Code:        agentbindings.ConflictCodeResourceBound,
		Operation:   "suspend_organization_skills",
		BindingType: agentbindings.BindingTypeSkill,
		ResourceID:  "calculator",
		ImpactToken: "impact-token",
		Agents: []agentbindings.ImpactAgent{{
			AgentID: uuid.NewString(),
			Name:    "Calculator agent",
		}},
	}
	service := &skillConfigUpdateHandlerService{err: &agentbindings.ConflictError{Impact: impact}}
	recorder := runUpdateSkillConfigRequest(t, service, `{"enabled_skill_ids":[]}`)

	var body struct {
		Code string `json:"code"`
		Data struct {
			Status  string               `json:"status"`
			Applied bool                 `json:"applied"`
			Impact  agentbindings.Impact `json:"impact"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusOK || body.Code != "0" || body.Data.Applied || body.Data.Status != skillConfigUpdateStatusConfirmationRequired {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if body.Data.Impact.ImpactToken != impact.ImpactToken || len(body.Data.Impact.Agents) != 1 {
		t.Fatalf("impact = %#v, want token %q and one agent", body.Data.Impact, impact.ImpactToken)
	}
}

func runUpdateSkillConfigRequest(
	t *testing.T,
	service runtimeservice.Service,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler := NewHandler(service)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/aichat/skills/config", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("account_id", uuid.NewString())
	util.SetOrganizationID(ctx, uuid.NewString())

	handler.UpdateSkillConfig(ctx)
	return recorder
}
