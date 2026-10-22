package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type countingLLMClient struct {
	llmclient.LLMClient
	calls int
}

func (c *countingLLMClient) AppChat(context.Context, *llmclient.AppContext, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	c.calls++
	return &adapter.ChatResponse{}, nil
}

func (c *countingLLMClient) AppChatStream(context.Context, *llmclient.AppContext, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	c.calls++
	return nil, nil
}

type recordingQuotaGuard struct {
	refs []EndUserQuotaRef
	err  error
}

func (g *recordingQuotaGuard) CheckEndUserQuota(_ context.Context, ref EndUserQuotaRef) error {
	g.refs = append(g.refs, ref)
	return g.err
}

func TestModelCallsOfEndUsersPassTheQuotaGuard(t *testing.T) {
	guard := &recordingQuotaGuard{}
	UseEndUserQuotaGuard(guard)
	t.Cleanup(func() { UseEndUserQuotaGuard(nil) })
	inner := &countingLLMClient{}
	client := withEndUserQuotaGuard(inner)
	if withEndUserQuotaGuard(nil) != nil {
		t.Fatal("a missing client was replaced")
	}

	if _, err := client.AppChat(t.Context(), &llmclient.AppContext{AppID: "app"}, nil); err != nil || len(guard.refs) != 0 || inner.calls != 1 {
		t.Fatalf("a call without an end user was checked: %v %v", guard.refs, err)
	}
	subjectID := uuid.New()
	endUser := &llmclient.AppContext{AppID: "app", AppType: "digital_human", EndUserSubjectID: subjectID.String()}
	if _, err := client.AppChatStream(t.Context(), endUser, nil); err != nil || len(guard.refs) != 1 || guard.refs[0].SubjectID != subjectID || guard.refs[0].AppID != "app" {
		t.Fatalf("end user call = %v %v", guard.refs, err)
	}

	guard.err = errors.New("quota spent")
	if _, err := client.AppChat(t.Context(), endUser, nil); !errors.Is(err, guard.err) || inner.calls != 2 {
		t.Fatalf("an exhausted end user reached the model: %v, calls %d", err, inner.calls)
	}
}

func TestEndUserBillingFollowsTheScopeThenTheRunConfig(t *testing.T) {
	runtimeWorkspaceID, subjectID, configured := uuid.New(), uuid.New(), uuid.New()
	prepared := &PreparedChat{Scope: Scope{RuntimeWorkspaceID: &runtimeWorkspaceID, RuntimeSubjectID: &subjectID},
		RunConfig: RunConfig{BillingEndUserSubjectID: configured.String()}}
	appCtx := &llmclient.AppContext{}
	applyEndUserBilling(appCtx, prepared)
	if appCtx.RuntimeWorkspaceID != runtimeWorkspaceID.String() || appCtx.EndUserSubjectID != subjectID.String() {
		t.Fatalf("scoped billing = %+v", appCtx)
	}
	prepared.Scope = Scope{}
	appCtx = &llmclient.AppContext{}
	applyEndUserBilling(appCtx, prepared)
	if appCtx.RuntimeWorkspaceID != "" || appCtx.EndUserSubjectID != configured.String() {
		t.Fatalf("configured billing = %+v", appCtx)
	}
}
