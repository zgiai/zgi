package integrations

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	completionRecoveryBatchSize        = 25
	completionRecoveryInterval         = 5 * time.Second
	completionRecoveryOperationTimeout = 10 * time.Second
)

type Executor struct {
	registry          *Registry
	audit             ExecutionRepository
	quota             DailyQuota
	safety            SafetyChecker
	hmacKey           []byte
	timeout           time.Duration
	outbox            ExecutionCompletionOutbox
	connections       ConnectionResolver
	actionPolicies    ActionPolicyResolver
	agentAuthorizer   AgentConnectionAuthorizer
	accessAuthorizer  ConnectionAccessAuthorizer
	healthSignals     ConnectionHealthSignalSink
	operationReceipts OperationReceiptRepository
}

func (e *Executor) WithOperationReceiptRepository(repository OperationReceiptRepository) *Executor {
	if e != nil {
		e.operationReceipts = repository
	}
	return e
}

func (e *Executor) WithConnectionAccessAuthorizer(authorizer ConnectionAccessAuthorizer) *Executor {
	if e != nil {
		e.accessAuthorizer = authorizer
	}
	return e
}

func (e *Executor) WithConnectionHealthSignalSink(sink ConnectionHealthSignalSink) *Executor {
	if e != nil {
		e.healthSignals = sink
	}
	return e
}

func (e *Executor) WithCompletionOutbox(outbox ExecutionCompletionOutbox) *Executor {
	if e != nil {
		e.outbox = outbox
	}
	return e
}

func (e *Executor) WithConnectionResolver(resolver ConnectionResolver) *Executor {
	if e != nil {
		e.connections = resolver
	}
	return e
}

func (e *Executor) WithActionPolicyResolver(resolver ActionPolicyResolver) *Executor {
	if e != nil {
		e.actionPolicies = resolver
	}
	return e
}

func (e *Executor) WithAgentConnectionAuthorizer(authorizer AgentConnectionAuthorizer) *Executor {
	if e != nil {
		e.agentAuthorizer = authorizer
	}
	return e
}

func NewExecutor(registry *Registry, audit ExecutionRepository, quota DailyQuota, safety SafetyChecker, hmacKey []byte, timeout time.Duration) *Executor {
	if safety == nil {
		safety = DefaultSafetyChecker{}
	}
	return &Executor{
		registry:       registry,
		audit:          audit,
		quota:          quota,
		safety:         safety,
		hmacKey:        append([]byte(nil), hmacKey...),
		timeout:        timeout,
		actionPolicies: providerDefaultActionPolicyResolver{},
	}
}

type providerDefaultActionPolicyResolver struct{}

func (providerDefaultActionPolicyResolver) Resolve(_ context.Context, _, _ string, action ActionDefinition) (ActionPolicyDecision, error) {
	return defaultActionPolicyDecision(action), nil
}

func (e *Executor) Execute(ctx context.Context, req ActionRequest) (*ActionResult, error) {
	if e == nil || e.registry == nil {
		return nil, NewError(ErrorCodeDisabled, "external integrations are not configured", nil)
	}
	if err := validateActionContext(&req); err != nil {
		return nil, err
	}
	e.reconcilePendingCompletionsBestEffort(ctx)
	resolved, err := e.registry.Resolve(req.IntegrationID, req.ActionID)
	if err != nil {
		return nil, err
	}
	if !actionSupportsCaller(resolved.Definition, req.InvokeFrom) {
		return nil, invalidInput("integration action is not available to this caller", nil)
	}
	operationCtx := ctx
	cancelOperation := func() {}
	if e.timeout > 0 {
		operationCtx, cancelOperation = context.WithTimeout(ctx, e.timeout)
	}
	defer cancelOperation()
	requestedConnectionID := strings.TrimSpace(req.ConnectionID)
	if req.InvokeFrom == tools.ToolInvokeFromAgent {
		if requestedConnectionID == "" {
			return nil, NewError(ErrorCodeAccessDenied, "Agent must bind an integration connection before using this action", nil)
		}
		authorization := AgentConnectionAuthorizationRequest{
			OrganizationID: req.OrganizationID,
			AgentID:        req.AgentID,
			ConnectionID:   requestedConnectionID,
			IntegrationID:  resolved.IntegrationID,
			ActionID:       resolved.Definition.ID,
		}
		if req.VerifyAgentConnection != nil {
			allowed, err := req.VerifyAgentConnection(operationCtx, authorization)
			if err != nil || !allowed {
				return nil, NewError(ErrorCodeAccessDenied, "Agent is not authorized to use this integration connection action", err)
			}
		} else if e.agentAuthorizer == nil {
			return nil, NewError(ErrorCodeAccessDenied, "Agent integration authorization is unavailable", nil)
		} else if err := e.agentAuthorizer.AuthorizeAgentConnection(operationCtx, authorization); err != nil {
			return nil, NewError(ErrorCodeAccessDenied, "Agent is not authorized to use this integration connection action", err)
		}
		agentGrantAuthorizer, ok := e.accessAuthorizer.(AgentConnectionAccessAuthorizer)
		if !ok {
			return nil, NewError(ErrorCodeAccessDenied, "Agent integration connection grant authorization is unavailable", nil)
		}
		grantRequest, grantRequestErr := connectionAccessRequest(req, resolved.Definition, resolved.IntegrationID, requestedConnectionID)
		if grantRequestErr != nil {
			return nil, grantRequestErr
		}
		if err := agentGrantAuthorizer.AuthorizeAgentConnectionUse(operationCtx, grantRequest); err != nil {
			return nil, NewError(ErrorCodeAccessDenied, "Agent is not authorized by a current shared integration connection grant", err)
		}
	}
	decision, policyErr := e.actionPolicies.Resolve(operationCtx, req.OrganizationID, resolved.IntegrationID, resolved.Definition)
	if policyErr != nil {
		return nil, NewError(ErrorCodeAccessDenied, "integration action policy could not be resolved", policyErr)
	}
	if !decision.Enabled {
		return nil, NewError(ErrorCodeDisabled, "integration action is disabled by organization policy", nil)
	}
	if resolved.Definition.DataEgress && !decision.DataEgressAllowed {
		return nil, NewError(ErrorCodeAccessDenied, "organization policy blocks data egress for this integration action", nil)
	}
	if requestedConnectionID != "" && (req.InvokeFrom == tools.ToolInvokeFromAIChat || req.InvokeFrom == tools.ToolInvokeFromAPI) && e.connections != nil {
		if e.accessAuthorizer == nil {
			return nil, NewError(ErrorCodeAccessDenied, "integration connection authorization is unavailable", nil)
		}
		authorization, authorizationErr := connectionAccessRequest(req, resolved.Definition, resolved.IntegrationID, requestedConnectionID)
		if authorizationErr != nil {
			return nil, authorizationErr
		}
		if err := e.accessAuthorizer.AuthorizeConnectionUse(operationCtx, authorization); err != nil {
			return nil, NewError(ErrorCodeAccessDenied, "account is not authorized to use this integration connection action", err)
		}
	}
	if req.Input == nil {
		req.Input = map[string]interface{}{}
	}
	if err := e.safety.Check(operationCtx, resolved.Definition, req.Input); err != nil {
		if operationCtx.Err() != nil {
			return nil, NewError(ErrorCodeTimeout, "external integration preflight timed out", operationCtx.Err())
		}
		return nil, err
	}
	if operationCtx.Err() != nil {
		return nil, NewError(ErrorCodeTimeout, "external integration preflight timed out", operationCtx.Err())
	}
	if err := ValidateActionInput(resolved.IntegrationID, resolved.Definition, req.Input); err != nil {
		return nil, err
	}
	// Resolve and decrypt credentials only after authorization, organization
	// policy, sensitive-data checks, and schema validation have all succeeded.
	// The resolved secret remains request-scoped and is destroyed after this call.
	if e.connections != nil {
		connection, resolveErr := e.connections.Resolve(operationCtx, ConnectionResolveRequest{
			OrganizationID:            req.OrganizationID,
			IntegrationID:             resolved.IntegrationID,
			DriverID:                  resolved.Adapter.DriverID(),
			ConnectionID:              requestedConnectionID,
			DisallowAccountCredential: req.InvokeFrom == tools.ToolInvokeFromAgent,
		})
		if resolveErr != nil {
			if operationCtx.Err() != nil {
				return nil, NewError(ErrorCodeTimeout, "external integration credential resolution timed out", operationCtx.Err())
			}
			return nil, resolveErr
		}
		if connection == nil {
			return nil, NewError(ErrorCodeAuthInvalid, "external integration credentials are unavailable", nil)
		}
		if req.InvokeFrom == tools.ToolInvokeFromAgent && connection.CredentialSource == ConnectionCredentialSourceAccount {
			connection.Destroy()
			return nil, NewError(ErrorCodeAccessDenied, "personal integration credentials are not available to Agents", nil)
		}
		if !ActionSupportsAuthMethod(resolved.Definition, connection.AuthMethodID) {
			connection.Destroy()
			return nil, NewError(ErrorCodeAccessDenied, "integration action is not available for this connection authentication method", nil)
		}
		req.Connection = connection
		defer connection.Destroy()
		if connection.ID != "" {
			req.ConnectionID = connection.ID
		}
		scopeRequirement := ActionScopeRequirement(resolved.Definition)
		if (len(scopeRequirement.AllOf) > 0 || len(scopeRequirement.AnyOf) > 0) &&
			(connection.AuthType == ConnectionAuthTypeOAuth2 || len(connection.GrantedScopes) > 0) {
			if err := AuthorizeConnectionScopes(connection.GrantedScopes, scopeRequirement); err != nil {
				return nil, err
			}
		}
	}
	var operationReceipt *OperationReceipt
	if resolved.Definition.SuccessDeduplication != nil {
		if e.operationReceipts == nil || len(e.hmacKey) == 0 {
			return nil, NewError(ErrorCodeAuditFailed, "external operation replay protection is unavailable", nil)
		}
		identity, identityErr := deriveOperationIdentity(e.hmacKey, req, resolved)
		if identityErr != nil {
			return nil, NewError(ErrorCodeInvalidInput, "external operation identity could not be established", identityErr)
		}
		candidate, candidateErr := newOperationReceipt(req, resolved, identity)
		if candidateErr != nil {
			return nil, NewError(ErrorCodeInvalidInput, "external operation identity could not be established", candidateErr)
		}
		claim, claimErr := e.operationReceipts.Claim(operationCtx, candidate)
		if claimErr != nil {
			return nil, NewError(ErrorCodeAuditFailed, "external operation replay protection is unavailable", claimErr)
		}
		if !claim.Claimed {
			switch {
			case claim.Receipt == nil:
				return nil, NewError(ErrorCodeAuditFailed, "external operation replay protection returned an invalid claim", nil)
			case claim.Receipt.Status == OperationReceiptStatusSucceeded:
				return replayOperationResult(claim.Receipt, resolved.Definition.OutputSchema)
			case claim.Receipt.Status == OperationReceiptStatusExecuting:
				return nil, NewError(ErrorCodeOperationInProgress, "the same external operation is already in progress", nil)
			default:
				return nil, NewError(ErrorCodeOperationOutcomeUnknown, "the provider outcome for this external operation is unknown; verify it before sending again", nil)
			}
		}
		operationReceipt = claim.Receipt
	}
	releaseOperationClaim := func() error {
		if operationReceipt == nil {
			return nil
		}
		releaseCtx, releaseCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer releaseCancel()
		return e.operationReceipts.Release(releaseCtx, operationReceipt.ID, operationReceipt.ClaimToken)
	}
	if e.quota == nil {
		_ = releaseOperationClaim()
		return nil, NewError(ErrorCodeQuotaExceeded, "external integration quota service is unavailable", nil)
	}
	if err := e.quota.Acquire(operationCtx, req.OrganizationID); err != nil {
		_ = releaseOperationClaim()
		if errors.Is(err, ErrQuotaExceeded) {
			return nil, NewError(ErrorCodeQuotaExceeded, "organization external integration daily limit has been reached", err)
		}
		return nil, NewError(ErrorCodeQuotaExceeded, "external integration quota service is unavailable", err)
	}
	if e.audit == nil || len(e.hmacKey) == 0 {
		_ = releaseOperationClaim()
		return nil, NewError(ErrorCodeAuditFailed, "external integration audit service is unavailable", nil)
	}

	record, err := e.newExecutionRecord(req, resolved)
	if err != nil {
		_ = releaseOperationClaim()
		return nil, NewError(ErrorCodeAuditFailed, "external integration execution could not be audited", err)
	}
	if err := e.audit.Create(operationCtx, record); err != nil {
		_ = releaseOperationClaim()
		return nil, NewError(ErrorCodeAuditFailed, "external integration execution could not be audited", err)
	}
	if operationReceipt != nil {
		if err := e.operationReceipts.MarkProviderStarted(operationCtx, operationReceipt.ID, operationReceipt.ClaimToken, record.ID); err != nil {
			_ = releaseOperationClaim()
			completion := completionForResult(nil, 0, NewError(ErrorCodeAuditFailed, "external operation start could not be recorded", err))
			finalizeCtx, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			_ = e.audit.Complete(finalizeCtx, record.ID, completion)
			finalizeCancel()
			return nil, NewError(ErrorCodeAuditFailed, "external operation start could not be recorded", err)
		}
	}

	startedAt := time.Now()
	result, callErr := resolved.Adapter.Execute(operationCtx, req)
	durationMS := time.Since(startedAt).Milliseconds()
	if callErr == nil {
		if result == nil || result.Output == nil {
			callErr = NewError(ErrorCodeResponseInvalid, "integration returned an invalid response", nil)
		} else if err := tools.ValidateJSONSchemaValue(resolved.Definition.OutputSchema, result.Output); err != nil {
			callErr = NewError(ErrorCodeResponseInvalid, "integration returned an invalid response", err)
		}
	}
	if operationReceipt != nil {
		receiptCtx, receiptCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		if callErr == nil {
			if err := e.operationReceipts.CompleteSuccess(receiptCtx, operationReceipt.ID, operationReceipt.ClaimToken, result); err != nil {
				_ = e.operationReceipts.MarkOutcomeUnknown(receiptCtx, operationReceipt.ID, operationReceipt.ClaimToken, record.ID)
				callErr = NewError(ErrorCodeAuditFailed, "external operation succeeded but its replay receipt could not be completed", err)
			}
		} else if shouldReleaseOperationClaim(callErr) {
			if err := e.operationReceipts.Release(receiptCtx, operationReceipt.ID, operationReceipt.ClaimToken); err != nil {
				callErr = NewError(ErrorCodeAuditFailed, "external operation failure could not release its replay claim", errors.Join(callErr, err))
			}
		} else if err := e.operationReceipts.MarkOutcomeUnknown(receiptCtx, operationReceipt.ID, operationReceipt.ClaimToken, record.ID); err != nil {
			callErr = NewError(ErrorCodeAuditFailed, "external operation outcome could not be recorded safely", errors.Join(callErr, err))
		}
		receiptCancel()
	}
	completion := completionForResult(result, durationMS, callErr)
	finalizeCtx, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer finalizeCancel()
	if err := completeAuditWithRetry(finalizeCtx, e.audit, record.ID, completion); err != nil {
		pending := PendingExecutionCompletion{ExecutionID: record.ID, Completion: completion}
		queueCtx, queueCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		queueErr := error(nil)
		if e.outbox == nil {
			queueErr = fmt.Errorf("integration audit completion outbox is unavailable")
		} else {
			queueErr = e.outbox.Enqueue(queueCtx, pending)
		}
		queueCancel()
		if queueErr != nil {
			return nil, NewError(ErrorCodeAuditFailed, "external integration execution audit could not be completed", errors.Join(err, queueErr))
		}
		logger.WarnContext(ctx, "external integration execution audit queued for recovery", "execution_id", record.ID.String())
	}
	e.publishConnectionHealthSignalBestEffort(ctx, req, resolved, record.ID, result, durationMS, callErr)
	if callErr != nil {
		return nil, callErr
	}
	return result, nil
}

func connectionAccessRequest(req ActionRequest, action ActionDefinition, integrationID, connectionID string) (ConnectionAccessRequest, error) {
	organizationID, err := uuid.Parse(strings.TrimSpace(req.OrganizationID))
	if err != nil || organizationID == uuid.Nil {
		return ConnectionAccessRequest{}, invalidInput("organization id is required", err)
	}
	accountID, err := uuid.Parse(strings.TrimSpace(req.UserID))
	if err != nil || accountID == uuid.Nil {
		return ConnectionAccessRequest{}, invalidInput("account id is required", err)
	}
	parsedConnectionID, err := uuid.Parse(strings.TrimSpace(connectionID))
	if err != nil || parsedConnectionID == uuid.Nil {
		return ConnectionAccessRequest{}, NewError(ErrorCodeConnectionNotFound, "integration connection was not found", err)
	}
	var workspaceID *uuid.UUID
	if value := strings.TrimSpace(req.WorkspaceID); value != "" {
		parsed, parseErr := uuid.Parse(value)
		if parseErr != nil || parsed == uuid.Nil {
			return ConnectionAccessRequest{}, invalidInput("workspace id is invalid", parseErr)
		}
		workspaceID = &parsed
	}
	return ConnectionAccessRequest{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		AccountID:      accountID,
		ConnectionID:   parsedConnectionID,
		IntegrationID:  strings.ToLower(strings.TrimSpace(integrationID)),
		ActionID:       action.ID,
		Effect:         action.Effect,
	}, nil
}

func (e *Executor) publishConnectionHealthSignalBestEffort(ctx context.Context, req ActionRequest, resolved ResolvedAction, executionID uuid.UUID, result *ActionResult, durationMS int64, callErr error) {
	if e == nil || e.healthSignals == nil || req.Connection == nil || strings.TrimSpace(req.Connection.ID) == "" {
		return
	}
	organizationID, organizationErr := uuid.Parse(strings.TrimSpace(req.OrganizationID))
	connectionID, connectionErr := uuid.Parse(strings.TrimSpace(req.Connection.ID))
	if organizationErr != nil || connectionErr != nil || organizationID == uuid.Nil || connectionID == uuid.Nil {
		return
	}
	diagnostics := providerDiagnosticsForResult(result, callErr)
	signalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 250*time.Millisecond)
	defer cancel()
	err := e.healthSignals.PublishConnectionHealthSignal(signalCtx, ConnectionHealthSignal{
		OrganizationID:     organizationID,
		ConnectionID:       connectionID,
		IntegrationID:      resolved.IntegrationID,
		DriverID:           resolved.Adapter.DriverID(),
		ActionID:           resolved.Definition.ID,
		CredentialVersion:  req.Connection.CredentialVersion,
		ExecutionID:        executionID,
		ProviderRequestID:  diagnostics.RequestID,
		ProviderErrorCode:  diagnostics.ErrorCode,
		ProviderHTTPStatus: providerHTTPStatusPointer(diagnostics.HTTPStatus),
		RetryAfterAt:       cloneTimePointer(diagnostics.RetryAfterAt),
		DurationMS:         durationMS,
		ErrorCode:          ErrorCode(callErr),
		ObservedAt:         time.Now().UTC(),
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to publish integration connection health signal", "execution_id", executionID.String(), "connection_id", connectionID.String())
	}
}

func (e *Executor) reconcilePendingCompletionsBestEffort(ctx context.Context) {
	if e == nil || e.outbox == nil || e.audit == nil {
		return
	}
	reconcileCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := e.reconcilePendingCompletions(reconcileCtx, completionRecoveryBatchSize); err != nil {
		logger.WarnContext(ctx, "failed to reconcile pending integration audit completions", err)
	}
}

func (e *Executor) reconcilePendingCompletions(ctx context.Context, limit int64) (int, error) {
	claim, claimErr := e.outbox.Claim(ctx, limit)
	completionErr := claimErr
	for _, item := range claim.Items {
		if err := e.audit.Complete(ctx, item.ExecutionID, item.Completion); err != nil {
			completionErr = errors.Join(completionErr, err)
			continue
		}
		if err := e.outbox.Delete(ctx, item.ExecutionID); err != nil {
			completionErr = errors.Join(completionErr, err)
		}
	}
	return claim.ClaimedCount, completionErr
}

func (e *Executor) drainPendingCompletions(ctx context.Context) error {
	if e == nil || e.outbox == nil || e.audit == nil {
		return nil
	}
	var drainErr error
	for {
		claimed, err := e.reconcilePendingCompletions(ctx, completionRecoveryBatchSize)
		drainErr = errors.Join(drainErr, err)
		if claimed < completionRecoveryBatchSize {
			return drainErr
		}
		if err := ctx.Err(); err != nil {
			return errors.Join(drainErr, err)
		}
	}
}

// RunCompletionRecovery continuously drains persisted audit completions until ctx is cancelled.
// Redis processing leases make this safe to run in every API instance.
func (e *Executor) RunCompletionRecovery(ctx context.Context) {
	if e == nil || e.outbox == nil || e.audit == nil {
		return
	}
	recover := func() {
		recoverCtx, cancel := context.WithTimeout(ctx, completionRecoveryOperationTimeout)
		defer cancel()
		if err := e.drainPendingCompletions(recoverCtx); err != nil && ctx.Err() == nil {
			logger.WarnContext(ctx, "failed to recover pending integration audit completions", err)
		}
	}
	recover()
	ticker := time.NewTicker(completionRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recover()
		}
	}
}

func validateActionContext(req *ActionRequest) error {
	if req == nil {
		return invalidInput("invocation context is required", nil)
	}
	fields := []struct {
		name     string
		value    *string
		required bool
	}{
		{name: "organization", value: &req.OrganizationID, required: true},
		{name: "user", value: &req.UserID, required: true},
		{name: "agent", value: &req.AgentID},
		{name: "workspace", value: &req.WorkspaceID},
		{name: "app", value: &req.AppID},
		{name: "conversation", value: &req.ConversationID},
		{name: "message", value: &req.MessageID},
		{name: "connection", value: &req.ConnectionID},
	}
	for _, field := range fields {
		raw := strings.TrimSpace(*field.value)
		if raw == "" {
			if field.required {
				return invalidInput(field.name+" context is required", nil)
			}
			*field.value = ""
			continue
		}
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return invalidInput(field.name+" context is invalid", err)
		}
		*field.value = parsed.String()
	}
	if req.InvokeFrom == tools.ToolInvokeFromAgent && strings.TrimSpace(req.AgentID) == "" {
		return invalidInput("agent context is required for an integration connection", nil)
	}
	return nil
}

func completeAuditWithRetry(ctx context.Context, audit ExecutionRepository, id uuid.UUID, completion ExecutionCompletion) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := audit.Complete(ctx, id, completion); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(100*(attempt+1)) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return lastErr
			case <-timer.C:
			}
		}
	}
	return lastErr
}

func actionSupportsCaller(action ActionDefinition, caller tools.ToolInvokeFrom) bool {
	if len(action.SupportedCallers) == 0 {
		return true
	}
	for _, allowed := range action.SupportedCallers {
		if allowed == caller {
			return true
		}
	}
	return false
}

func (e *Executor) newExecutionRecord(req ActionRequest, resolved ResolvedAction) (*ExecutionRecord, error) {
	organizationID, err := uuid.Parse(strings.TrimSpace(req.OrganizationID))
	if err != nil {
		return nil, fmt.Errorf("invalid organization id: %w", err)
	}
	fingerprint, err := inputFingerprint(e.hmacKey, req.Input)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &ExecutionRecord{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		WorkspaceID:    optionalUUID(req.WorkspaceID),
		AccountID:      optionalUUID(req.UserID),
		AppID:          optionalUUID(req.AppID),
		ConversationID: optionalUUID(req.ConversationID),
		MessageID:      optionalUUID(req.MessageID),
		ConnectionID:   optionalUUID(req.ConnectionID),
		IntegrationID:  resolved.IntegrationID,
		DriverID:       strings.TrimSpace(resolved.Adapter.DriverID()),
		ActionID:       resolved.Definition.ID,
		InvokeFrom:     string(req.InvokeFrom),
		Status:         "running",
		InputHMAC:      &fingerprint,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func completionForResult(result *ActionResult, durationMS int64, callErr error) ExecutionCompletion {
	completion := ExecutionCompletion{Status: "succeeded", DurationMS: durationMS}
	if result != nil {
		completion.CostUSD = result.CostUSD
		completion.ResultCount = result.ResultCount
		completion.AttemptCount = result.AttemptCount
	}
	diagnostics := providerDiagnosticsForResult(result, callErr)
	completion.ProviderRequestID = diagnostics.RequestID
	completion.ProviderErrorCode = diagnostics.ErrorCode
	completion.ProviderHTTPStatus = providerHTTPStatusPointer(diagnostics.HTTPStatus)
	completion.RetryAfterAt = cloneTimePointer(diagnostics.RetryAfterAt)
	if callErr != nil {
		completion.ErrorCode = ErrorCode(callErr)
		if completion.ErrorCode == ErrorCodeTimeout {
			completion.Status = "timed_out"
		} else {
			completion.Status = "failed"
		}
	}
	return completion
}

func providerDiagnosticsForResult(result *ActionResult, callErr error) ProviderDiagnostics {
	resultDiagnostics := ProviderDiagnostics{}
	if result != nil {
		resultDiagnostics = result.ProviderDiagnostics
		if strings.TrimSpace(resultDiagnostics.RequestID) == "" {
			resultDiagnostics.RequestID = result.ProviderRequestID
		}
	}
	return mergeProviderDiagnostics(ProviderDiagnosticsFromError(callErr), resultDiagnostics)
}

func optionalUUID(raw string) *uuid.UUID {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return &parsed
}

func inputFingerprint(key []byte, input map[string]interface{}) (string, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("marshal integration input fingerprint: %w", err)
	}
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write(data)
	return hex.EncodeToString(digest.Sum(nil)), nil
}
