package service

import (
	"context"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type fakeMarketTools struct {
	tools []MarketTool
	seen  []string
}

func (f *fakeMarketTools) SubjectMarketTools(_ context.Context, organizationID uuid.UUID, subjectType string, subjectID uuid.UUID) ([]MarketTool, error) {
	f.seen = append(f.seen, organizationID.String()+"|"+subjectType+"|"+subjectID.String())
	return f.tools, nil
}

func TestMarketToolGovernanceRunsReadToolsAndAsksForTheRest(t *testing.T) {
	decide := func(risk string, mode toolgovernance.ApprovalMode) toolgovernance.Decision {
		manifest := marketToolGovernance("tool", risk)
		_, err := toolgovernance.ValidateManifest(*manifest)
		require.NoError(t, err, risk)
		return toolgovernance.Decide(toolgovernance.Request{Manifest: *manifest, ApprovalMode: mode}, toolgovernance.DefaultPolicy())
	}
	assert.Equal(t, toolgovernance.DecisionStatusAllowed, decide("read", toolgovernance.ApprovalModeNonInteractive).Status,
		"a read tool runs without a person")
	for _, risk := range []string{"write", "destructive", ""} {
		assert.Equal(t, toolgovernance.DecisionStatusNeedsApproval, decide(risk, toolgovernance.ApprovalModeInteractive).Status, risk)
		denied := decide(risk, toolgovernance.ApprovalModeNonInteractive)
		assert.Equal(t, toolgovernance.DecisionStatusDenied, denied.Status, "nobody can approve an unattended %q tool", risk)
	}
	assert.True(t, marketToolGovernance("tool", "destructive").ApprovalEveryInvocation)
}

func TestMarketToolsAreOverlaidForAnAgentOrADigitalHumanOnly(t *testing.T) {
	agentID, organizationID := uuid.New(), uuid.New()
	market := &fakeMarketTools{tools: []MarketTool{
		{ProviderID: "agent:" + agentID.String() + ":wiki", Name: "lookup", Description: "Look up", RiskLevel: "read",
			InputSchema: map[string]interface{}{"type": "object"}},
		{ProviderID: "agent:" + agentID.String() + ":notes", Name: "lookup", RiskLevel: "write",
			InputSchema: map[string]interface{}{"type": "object"}},
	}}
	s := &service{marketTools: market}
	prepared := &PreparedChat{
		Scope:  Scope{OrganizationID: organizationID},
		Caller: Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID},
		parts:  &chatRequestParts{SkillIDs: []string{"calculator"}},
	}
	resolved, err := s.overlayMarketTools(context.Background(), prepared, &skills.ResolvedSkills{})
	require.NoError(t, err)
	require.Len(t, resolved.Skills, 1)
	doc := resolved.Skills[0]
	assert.Equal(t, skills.SkillZGIMarketTools, doc.Metadata.ID)
	assert.Equal(t, []string{skills.SkillCallerAgent}, doc.Metadata.SupportedCallers)
	require.Len(t, doc.Tools, 2)
	assert.Equal(t, "wiki__lookup", doc.Tools[0].Name, "a shared tool name is told apart by its listing")
	assert.Equal(t, "notes__lookup", doc.Tools[1].Name)
	assert.Equal(t, tools.ToolProviderTypeMarket, doc.Tools[0].ProviderType)
	assert.Equal(t, toolgovernance.EffectRead, doc.Tools[0].Governance.Effect)
	assert.Equal(t, toolgovernance.EffectInvoke, doc.Tools[1].Governance.Effect)
	assert.Contains(t, prepared.parts.SkillIDs, skills.SkillZGIMarketTools)
	assert.Contains(t, prepared.parts.ToolSkillIDs, skills.SkillZGIMarketTools)
	assert.Equal(t, []string{organizationID.String() + "|agent|" + agentID.String()}, market.seen)

	chat := &PreparedChat{
		Scope: Scope{OrganizationID: organizationID}, Caller: Caller{Type: runtimemodel.ConversationCallerAIChat},
		parts: &chatRequestParts{SkillIDs: []string{skills.SkillZGIMarketTools}},
	}
	resolved, err = s.overlayMarketTools(context.Background(), chat, &skills.ResolvedSkills{})
	require.NoError(t, err)
	assert.Empty(t, resolved.Skills, "an ordinary chat has no bound subject")
	assert.NotContains(t, chat.parts.SkillIDs, skills.SkillZGIMarketTools)
}

// A run activates the market Skill on demand. The Skill is neither in the organization's
// catalog nor a Skill binding of the subject, so the step authorizer has to know it.
func TestTheMarketSkillIsAuthorizedOnlyForARunThatWasGivenIt(t *testing.T) {
	agentID := uuid.New()
	s := &service{}
	for _, callerType := range []string{runtimemodel.ConversationCallerAgent, runtimemodel.ConversationCallerDigitalHuman} {
		bound := &PreparedChat{
			Scope:  Scope{OrganizationID: uuid.New()},
			Caller: Caller{Type: callerType, ID: &agentID},
			parts:  &chatRequestParts{MarketSkillIDs: []string{skills.SkillZGIMarketTools}},
		}
		allowed, err := s.currentAgentSkillStepAuthorizer(bound)(context.Background(), " ZGI-Market-Tools ")
		require.NoError(t, err)
		assert.True(t, allowed, callerType)

		unbound := &PreparedChat{
			Scope:  Scope{OrganizationID: uuid.New()},
			Caller: Caller{Type: callerType, ID: &agentID},
			parts:  &chatRequestParts{SkillIDs: []string{skills.SkillZGIMarketTools}},
		}
		allowed, err = s.currentAgentSkillStepAuthorizer(unbound)(context.Background(), skills.SkillZGIMarketTools)
		require.NoError(t, err)
		assert.False(t, allowed, "naming the Skill in a request does not grant it")
	}
}

// After an approval the transcript still says the call is waiting; the model must read the outcome.
func TestAnApprovedMarketCallReplacesItsPendingToolResult(t *testing.T) {
	request := &adapter.ChatRequest{Messages: []adapter.Message{
		{Role: "user", Content: "create the item"},
		{Role: "tool", ToolCallID: "call_0", Content: `{"status":"success"}`},
		{Role: "tool", ToolCallID: "call_1", Content: `{"governance":{"correlation_id":"corr-1","status":"needs_approval"}}`},
	}}
	assert.False(t, replacePendingMarketToolResult(request, "corr-2", map[string]interface{}{"status": "completed"}), "another approval is left alone")
	assert.Contains(t, request.Messages[2].Content, "needs_approval")

	require.True(t, replacePendingMarketToolResult(request, "corr-1", map[string]interface{}{"status": "completed", "result": "note-1"}))
	assert.Equal(t, "call_1", request.Messages[2].ToolCallID, "the answer stays paired with its call")
	assert.NotContains(t, request.Messages[2].Content, "needs_approval")
	assert.Contains(t, request.Messages[2].Content, "note-1")
	assert.Equal(t, `{"status":"success"}`, request.Messages[1].Content)
	assert.False(t, replacePendingMarketToolResult(nil, "corr-1", map[string]interface{}{"status": "completed"}))
}
