package agents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
)

func TestExternalAgentInvocationContextPreservesAPISource(t *testing.T) {
	ctx := externalAgentInvocationContext(context.Background())
	if got, _ := ctx.Value("invoke_from").(string); got != "external-api" {
		t.Fatalf("invoke_from = %q, want external-api", got)
	}
}

func TestPublishedAgentConfigRequiresExternalUserWheneverMemoryEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  dto.AgentConfigResponse
		want bool
	}{
		{
			name: "disabled memory",
			cfg: dto.AgentConfigResponse{
				AgentMemoryEnabled: false,
				AgentMemorySlots:   []dto.AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
			},
			want: false,
		},
		{
			name: "enabled memory without enabled slots",
			cfg: dto.AgentConfigResponse{
				AgentMemoryEnabled: true,
				AgentMemorySlots: []dto.AgentMemorySlotConfig{
					{Key: "profile", Enabled: false},
					{Key: "", Enabled: true},
				},
			},
			want: true,
		},
		{
			name: "enabled memory with enabled slot",
			cfg: dto.AgentConfigResponse{
				AgentMemoryEnabled: true,
				AgentMemorySlots:   []dto.AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := publishedAgentConfigRequiresExternalUser(tt.cfg)
			if got != tt.want {
				t.Fatalf("publishedAgentConfigRequiresExternalUser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExternalAgentMemoryUserIDStableAcrossAPIKeys(t *testing.T) {
	workspaceID := uuid.New()
	agentID := uuid.New()

	first := externalAgentMemoryUserID(workspaceID, agentID, " external-user-1 ")
	second := externalAgentMemoryUserID(workspaceID, agentID, "external-user-1")
	if first != second {
		t.Fatalf("externalAgentMemoryUserID() changed for same workspace/agent/user: %s != %s", first, second)
	}

	otherUser := externalAgentMemoryUserID(workspaceID, agentID, "External-User-1")
	if otherUser == first {
		t.Fatalf("externalAgentMemoryUserID() ignored user case, got same id %s", first)
	}

	otherAgent := externalAgentMemoryUserID(workspaceID, uuid.New(), "external-user-1")
	if otherAgent == first {
		t.Fatalf("externalAgentMemoryUserID() ignored agent scope, got same id %s", first)
	}
}

func TestAPIKeyContinuationRequiresAndResolvesUserWhenMemoryEnabled(t *testing.T) {
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
		conversationID: uuid.New(),
		messageID:      uuid.New(),
	}
	appService := newAgentRuntimePermissionAppService(ids)
	appService.agentMemoryEnabled = true
	handler := NewAgentsHandler(appService, nil, nil, nil, nil)

	newContext := func(rawQuery string) (*gin.Context, *httptest.ResponseRecorder) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/workflow-continuation"+rawQuery, nil)
		ctx.Set("account_id", ids.accountID.String())
		ctx.Set("organization_id", ids.organizationID.String())
		ctx.Set("workspace_id", ids.workspaceID.String())
		ctx.Set("agent_id", ids.agentID.String())
		return ctx, recorder
	}

	ctx, recorder := newContext("")
	if _, ok := handler.apiKeyAgentContinuationRuntimeContext(ctx); ok {
		t.Fatal("continuation accepted a missing user while Agent memory was enabled")
	}
	if !strings.Contains(recorder.Body.String(), externalAgentMemoryUserRequiredMessage) {
		t.Fatalf("response = %s, want missing-user error", recorder.Body.String())
	}

	ctx, _ = newContext("?user=customer_123")
	runtimeCtx, ok := handler.apiKeyAgentContinuationRuntimeContext(ctx)
	if !ok || runtimeCtx.Scope.AgentMemoryUserID == nil {
		t.Fatalf("continuation context = %#v, ok = %v; want resolved API end-user", runtimeCtx, ok)
	}
	want := externalAgentMemoryUserID(ids.workspaceID, ids.agentID, "customer_123")
	if *runtimeCtx.Scope.AgentMemoryUserID != want {
		t.Fatalf("resolved user = %s, want %s", *runtimeCtx.Scope.AgentMemoryUserID, want)
	}
}
