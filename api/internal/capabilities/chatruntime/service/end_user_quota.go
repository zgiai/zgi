package service

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// Runtime Workspace end users have points quotas (ADR 0010). Every model call
// of a run made for such an end user passes through the guard first, so a run
// stops once the end user's quota is spent; the gateway only meters the call.

// EndUserQuotaRef names the end user and app a model call is made for. The
// Runtime Workspace may be unknown to the caller; the guard resolves it from
// the subject.
type EndUserQuotaRef struct {
	RuntimeWorkspaceID uuid.UUID
	SubjectID          uuid.UUID
	AppType            string
	AppID              string
}

// EndUserQuotaGuard rejects a call once an end user's quota is spent.
type EndUserQuotaGuard interface {
	CheckEndUserQuota(ctx context.Context, ref EndUserQuotaRef) error
}

var endUserQuotaGuard atomic.Value

type endUserQuotaGuardHolder struct{ guard EndUserQuotaGuard }

// UseEndUserQuotaGuard registers the guard. It is set where Runtime
// Workspaces are served, so every chat runtime enforces the same quotas.
func UseEndUserQuotaGuard(guard EndUserQuotaGuard) {
	endUserQuotaGuard.Store(endUserQuotaGuardHolder{guard: guard})
}

// CheckEndUserQuota checks an end user's quota with the registered guard. It
// allows the call when no guard is registered.
func CheckEndUserQuota(ctx context.Context, ref EndUserQuotaRef) error {
	holder, _ := endUserQuotaGuard.Load().(endUserQuotaGuardHolder)
	if holder.guard == nil || ref.SubjectID == uuid.Nil {
		return nil
	}
	return holder.guard.CheckEndUserQuota(ctx, ref)
}

// endUserQuotaRefFor reads the end user an app context bills, if any.
func endUserQuotaRefFor(appCtx *llmclient.AppContext) (EndUserQuotaRef, bool) {
	if appCtx == nil {
		return EndUserQuotaRef{}, false
	}
	runtimeWorkspaceID, _ := uuid.Parse(strings.TrimSpace(appCtx.RuntimeWorkspaceID))
	subjectID, err := uuid.Parse(strings.TrimSpace(appCtx.EndUserSubjectID))
	if err != nil || subjectID == uuid.Nil {
		return EndUserQuotaRef{}, false
	}
	return EndUserQuotaRef{RuntimeWorkspaceID: runtimeWorkspaceID, SubjectID: subjectID, AppType: appCtx.AppType, AppID: appCtx.AppID}, true
}

// quotaGuardedLLMClient checks the end user's quota before each chat call.
type quotaGuardedLLMClient struct {
	llmclient.LLMClient
}

// withEndUserQuotaGuard guards a chat runtime's model calls; nil stays nil.
func withEndUserQuotaGuard(client llmclient.LLMClient) llmclient.LLMClient {
	if client == nil {
		return nil
	}
	if _, guarded := client.(quotaGuardedLLMClient); guarded {
		return client
	}
	return quotaGuardedLLMClient{LLMClient: client}
}

func (c quotaGuardedLLMClient) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	if ref, ok := endUserQuotaRefFor(appCtx); ok {
		if err := CheckEndUserQuota(ctx, ref); err != nil {
			return nil, err
		}
	}
	return c.LLMClient.AppChat(ctx, appCtx, req)
}

func (c quotaGuardedLLMClient) AppChatStream(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	if ref, ok := endUserQuotaRefFor(appCtx); ok {
		if err := CheckEndUserQuota(ctx, ref); err != nil {
			return nil, err
		}
	}
	return c.LLMClient.AppChatStream(ctx, appCtx, req)
}

// applyEndUserBilling labels an app context with the run's Runtime Workspace
// end user: the scope's for Agent runtime calls, else the run config's.
func applyEndUserBilling(appCtx *llmclient.AppContext, prepared *PreparedChat) {
	if appCtx == nil || prepared == nil {
		return
	}
	if prepared.Scope.RuntimeWorkspaceID != nil && prepared.Scope.RuntimeSubjectID != nil &&
		*prepared.Scope.RuntimeWorkspaceID != uuid.Nil && *prepared.Scope.RuntimeSubjectID != uuid.Nil {
		appCtx.RuntimeWorkspaceID, appCtx.EndUserSubjectID = prepared.Scope.RuntimeWorkspaceID.String(), prepared.Scope.RuntimeSubjectID.String()
		return
	}
	if prepared.RunConfig.BillingEndUserSubjectID != "" {
		appCtx.RuntimeWorkspaceID, appCtx.EndUserSubjectID = prepared.RunConfig.BillingRuntimeWorkspaceID, prepared.RunConfig.BillingEndUserSubjectID
	}
}
