package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	pluginrunner_client "github.com/zgiai/zgi/api/internal/modules/pluginrunner/client"
	pluginrunner_model "github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
	pluginrunner_service "github.com/zgiai/zgi/api/internal/modules/pluginrunner/service"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	defaultInvokeTimeoutSeconds = 30
	defaultWaitMode             = "first"
	defaultStreamMode           = "first"
	defaultSessionPolicy        = "no_reuse"
	defaultSessionIdleTTL       = 60 * time.Second
	defaultSessionMaxLifetime   = 60 * time.Minute

	sessionPolicyReuseWithinRun = "reuse_within_run"
)

type executionPolicy struct {
	WaitMode             string
	StreamMode           string
	SessionPolicy        string
	SessionIdleTTL       time.Duration
	SessionMaxLifetime   time.Duration
	SerializeInvocations bool
}

type reusablePluginSession struct {
	key           string
	workflowRunID string
	sessionID     string
	tenantID      string
	provider      string
	idleTTL       time.Duration
	lastUsedAt    time.Time
	invokeMu      sync.Mutex
}

// PluginRunnerToolAdapter is an adapter that connects the plugin runner service to the tool system
type PluginRunnerToolAdapter struct {
	service                    pluginrunner_service.PluginRunnerService
	accountInstallationService pluginrunner_service.AccountInstallationService

	mu               sync.Mutex
	reusableSessions map[string]*reusablePluginSession
}

// NewPluginRunnerToolAdapter creates a new PluginRunnerToolAdapter
func NewPluginRunnerToolAdapter(
	svc pluginrunner_service.PluginRunnerService,
	accountInstallService pluginrunner_service.AccountInstallationService,
) *PluginRunnerToolAdapter {
	return &PluginRunnerToolAdapter{
		service:                    svc,
		accountInstallationService: accountInstallService,
		reusableSessions:           make(map[string]*reusablePluginSession),
	}
}

// Ensure PluginRunnerToolAdapter implements PluginRunnerToolManagerInterface
var _ PluginRunnerToolManagerInterface = (*PluginRunnerToolAdapter)(nil)

// FetchToolProviders fetches all tool providers for the given tenant
func (a *PluginRunnerToolAdapter) FetchToolProviders(ctx context.Context, tenantID string) ([]ToolProviderEntity, error) {
	// TODO: enforce subscription checks for runner plugins when listing providers.
	// Get installed plugins from the plugin runner service
	installations, err := a.service.ListInstalledPlugins(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list installed plugins: %w", err)
	}

	var providers []ToolProviderEntity
	for _, installation := range installations {
		provider := a.manifestToProviderEntity(installation.Manifest)
		providers = append(providers, provider)
	}

	return providers, nil
}

// FetchToolProvider fetches a specific tool provider
func (a *PluginRunnerToolAdapter) FetchToolProvider(ctx context.Context, tenantID, providerName string) (*ToolProviderEntity, error) {
	installations, err := a.service.ListInstalledPlugins(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list installed plugins: %w", err)
	}

	if installation := findRunnerInstallationByName(installations, providerName); installation != nil {
		provider := a.manifestToProviderEntity(installation.Manifest)
		return &provider, nil
	}

	for _, installation := range installations {
		if !a.matchesInstalledProvider(ctx, tenantID, providerName, installation.Manifest) {
			continue
		}
		if installation.Manifest.Name == providerName {
			continue
		}
		logger.Warn("resolved plugin provider through legacy runner manifest name",
			"tenant_id", tenantID,
			"provider", providerName,
			"runner_provider", installation.Manifest.Name,
			"marketplace_version_id", installation.Manifest.MarketplaceVersionID)
		provider := a.manifestToProviderEntity(installation.Manifest)
		return &provider, nil
	}

	return nil, fmt.Errorf("plugin provider %s not found", providerName)
}

// InvokeTool invokes a tool through the plugin runner
func (a *PluginRunnerToolAdapter) InvokeTool(
	ctx context.Context,
	tenantID, userID, provider, tool string,
	params map[string]interface{},
) ([]ToolInvokeMessage, error) {
	// TODO: enforce subscription checks for runner plugins before invocation.
	logger.Debug("plugin runner adapter invoking tool",
		"tenant_id", tenantID,
		"user_id", userID,
		"provider", provider,
		"tool", tool)

	runnerProvider, err := a.resolveRunnerProviderName(ctx, tenantID, provider)
	if err != nil {
		return nil, err
	}
	policy := a.resolveExecutionPolicy(ctx, tenantID, provider)
	entrypoint, version, err := a.resolvePluginRuntime(ctx, runnerProvider)
	if err != nil {
		return nil, err
	}

	workflowRunID := getWorkflowRunIDFromContext(ctx)
	if policy.SessionPolicy == sessionPolicyReuseWithinRun && workflowRunID != "" {
		return a.invokeWithReusableSession(ctx, tenantID, runnerProvider, tool, params, policy, workflowRunID, entrypoint, version)
	}

	return a.invokeWithEphemeralSession(ctx, tenantID, runnerProvider, tool, params, policy, entrypoint, version)
}

// ValidateCredentials validates credentials for a provider
func (a *PluginRunnerToolAdapter) ValidateCredentials(
	ctx context.Context,
	tenantID, userID, provider string,
	credentials map[string]interface{},
) (bool, error) {
	// Check if the plugin exists
	_, err := a.FetchToolProvider(ctx, tenantID, provider)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// manifestToProviderEntity converts a plugin manifest to a tool provider entity
func (a *PluginRunnerToolAdapter) manifestToProviderEntity(manifest pluginrunner_model.PluginManifest) ToolProviderEntity {
	return ToolProviderEntity{
		Identity: ToolProviderIdentity{
			Name:        manifest.Name,
			Label:       I18nText{"en_US": manifest.Name},
			Description: I18nText{"en_US": manifest.Description},
		},
		ProviderType: ToolProviderTypePluginRunner,
		// Tools would be populated from the actual plugin manifest
		Tools: []ToolEntity{},
	}
}

// convertResponse converts a plugin runner response to tool invoke messages
func (a *PluginRunnerToolAdapter) convertResponse(resp *pluginrunner_model.InvokeResponse) ([]ToolInvokeMessage, error) {
	var messages []ToolInvokeMessage

	if resp.Success {
		if resp.Data != nil {
			// Handle different response types
			if text, ok := resp.Data["text"].(string); ok && text != "" {
				messages = append(messages, ToolInvokeMessage{
					Type: ToolInvokeMessageTypeText,
					Text: text,
				})
			} else {
				messages = append(messages, ToolInvokeMessage{
					Type: ToolInvokeMessageTypeJSON,
					Data: resp.Data,
				})
			}
		} else {
			messages = append(messages, ToolInvokeMessage{
				Type: ToolInvokeMessageTypeText,
				Text: "Tool executed successfully",
			})
		}
	} else {
		return nil, fmt.Errorf("tool invocation failed: %s", resp.Error)
	}

	return messages, nil
}

// parseTenantIDString converts string tenant ID to uint
func parseTenantIDString(tenantID string) uint {
	if tenantID == "" {
		return 0
	}
	var id uint
	fmt.Sscanf(tenantID, "%d", &id)
	return id
}

func (a *PluginRunnerToolAdapter) invokeWithEphemeralSession(
	ctx context.Context,
	tenantID, provider, tool string,
	params map[string]interface{},
	policy executionPolicy,
	entrypoint, version string,
) ([]ToolInvokeMessage, error) {
	session, err := a.startPluginSession(ctx, provider, tenantID, entrypoint, version)
	if err != nil {
		return nil, err
	}

	defer func() {
		if stopErr := a.service.StopSession(ctx, session.ID); stopErr != nil {
			logger.Warn("failed to stop session", "session_id", session.ID, "error", stopErr)
		}
	}()

	resp, err := a.invokeWithSession(ctx, session.ID, provider, tool, params, policy)
	if err != nil {
		return nil, err
	}

	return a.convertResponse(resp)
}

func (a *PluginRunnerToolAdapter) invokeWithReusableSession(
	ctx context.Context,
	tenantID, provider, tool string,
	params map[string]interface{},
	policy executionPolicy,
	workflowRunID, entrypoint, version string,
) ([]ToolInvokeMessage, error) {
	a.cleanupExpiredReusableSessions()

	sessionKey := buildReusableSessionKey(tenantID, workflowRunID, provider)
	reusableSession, err := a.getOrCreateReusableSession(ctx, sessionKey, workflowRunID, tenantID, provider, entrypoint, version, policy)
	if err != nil {
		return nil, err
	}

	if policy.SerializeInvocations {
		reusableSession.invokeMu.Lock()
		defer reusableSession.invokeMu.Unlock()
	}

	resp, err := a.invokeWithSession(ctx, reusableSession.sessionID, provider, tool, params, policy)
	if err != nil {
		return nil, err
	}

	a.touchReusableSession(sessionKey)
	return a.convertResponse(resp)
}

func (a *PluginRunnerToolAdapter) invokeWithSession(
	ctx context.Context,
	sessionID, provider, tool string,
	params map[string]interface{},
	policy executionPolicy,
) (*pluginrunner_model.InvokeResponse, error) {
	resp, err := a.service.InvokeTool(ctx, pluginrunner_model.ToolInvokeRequest{
		SessionID:  sessionID,
		Provider:   provider,
		Tool:       tool,
		Parameters: params,
		Timeout:    defaultInvokeTimeoutSeconds,
		WaitMode:   policy.WaitMode,
		StreamMode: policy.StreamMode,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to invoke tool: %w", err)
	}

	return resp, nil
}

func (a *PluginRunnerToolAdapter) getOrCreateReusableSession(
	ctx context.Context,
	sessionKey, workflowRunID, tenantID, provider, entrypoint, version string,
	policy executionPolicy,
) (*reusablePluginSession, error) {
	now := time.Now()

	a.mu.Lock()
	if existing, ok := a.reusableSessions[sessionKey]; ok {
		existing.lastUsedAt = now
		a.mu.Unlock()
		return existing, nil
	}
	a.mu.Unlock()

	sessionOptions := []pluginrunner_client.RequestOption{
		pluginrunner_client.WithWorkflowRunID(workflowRunID),
		pluginrunner_client.WithSessionPolicy(sessionPolicyReuseWithinRun),
		pluginrunner_client.WithSessionIdleTTLSeconds(int(policy.SessionIdleTTL / time.Second)),
		pluginrunner_client.WithSessionMaxLifetimeSeconds(int(policy.SessionMaxLifetime / time.Second)),
	}
	session, err := a.startPluginSession(ctx, provider, tenantID, entrypoint, version, sessionOptions...)
	if err != nil {
		return nil, err
	}

	newReusableSession := &reusablePluginSession{
		key:           sessionKey,
		workflowRunID: workflowRunID,
		sessionID:     session.ID,
		tenantID:      tenantID,
		provider:      provider,
		idleTTL:       policy.SessionIdleTTL,
		lastUsedAt:    now,
	}

	var stopNewSession bool
	a.mu.Lock()
	if existing, ok := a.reusableSessions[sessionKey]; ok {
		existing.lastUsedAt = now
		stopNewSession = true
		a.mu.Unlock()

		if stopNewSession {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = a.service.StopSession(stopCtx, session.ID)
		}
		return existing, nil
	}

	a.reusableSessions[sessionKey] = newReusableSession
	a.mu.Unlock()

	return newReusableSession, nil
}

func (a *PluginRunnerToolAdapter) touchReusableSession(sessionKey string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if existing, ok := a.reusableSessions[sessionKey]; ok {
		existing.lastUsedAt = time.Now()
	}
}

func (a *PluginRunnerToolAdapter) cleanupExpiredReusableSessions() {
	now := time.Now()

	staleSessions := make([]*reusablePluginSession, 0)

	a.mu.Lock()
	for key, session := range a.reusableSessions {
		if session.idleTTL <= 0 {
			session.idleTTL = defaultSessionIdleTTL
		}
		if now.Sub(session.lastUsedAt) <= session.idleTTL {
			continue
		}

		delete(a.reusableSessions, key)
		staleSessions = append(staleSessions, session)
	}
	a.mu.Unlock()

	for _, session := range staleSessions {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := a.service.StopSession(stopCtx, session.sessionID); err != nil {
			logger.Warn("failed to stop stale reusable session", "session_id", session.sessionID, "error", err)
		}
		cancel()
	}
}

// StopReusableSessionsByWorkflowRunID force-cleans reusable sessions for a workflow run.
func (a *PluginRunnerToolAdapter) StopReusableSessionsByWorkflowRunID(ctx context.Context, workflowRunID string) (int, error) {
	workflowRunID = strings.TrimSpace(workflowRunID)
	if workflowRunID == "" {
		return 0, nil
	}

	sessionIDs := a.collectLocalReusableSessionIDsByRunID(workflowRunID)

	runnerSessions, err := a.service.ListSessions(ctx)
	if err != nil {
		stoppedCount, stopErr := a.stopSessionIDs(ctx, sessionIDs)
		if stopErr != nil {
			return stoppedCount, stopErr
		}
		return stoppedCount, fmt.Errorf("failed to list runner sessions for workflow cleanup: %w", err)
	}

	for _, session := range runnerSessions {
		if !isReusableRunnerSession(session) || session.Metadata == nil {
			continue
		}
		if strings.TrimSpace(session.Metadata.WorkflowRunID) != workflowRunID {
			continue
		}
		sessionIDs = append(sessionIDs, session.ID)
	}

	return a.stopSessionIDs(ctx, sessionIDs)
}

// SweepStaleReusableSessions stops reusable sessions idle for longer than maxAge.
func (a *PluginRunnerToolAdapter) SweepStaleReusableSessions(ctx context.Context, maxAge time.Duration) (int, error) {
	if maxAge <= 0 {
		return 0, nil
	}

	sessionIDs := a.collectLocalStaleReusableSessionIDs(maxAge)

	runnerSessions, err := a.service.ListSessions(ctx)
	if err != nil {
		stoppedCount, stopErr := a.stopSessionIDs(ctx, sessionIDs)
		if stopErr != nil {
			return stoppedCount, stopErr
		}
		return stoppedCount, fmt.Errorf("failed to list runner sessions for stale sweep: %w", err)
	}

	now := time.Now()
	for _, session := range runnerSessions {
		if !isReusableRunnerSession(session) {
			continue
		}
		lastActivityAt := sessionLastActivityAt(session)
		if now.Sub(lastActivityAt) <= maxAge {
			continue
		}
		sessionIDs = append(sessionIDs, session.ID)
	}

	return a.stopSessionIDs(ctx, sessionIDs)
}

func (a *PluginRunnerToolAdapter) collectLocalReusableSessionIDsByRunID(workflowRunID string) []string {
	sessionIDs := make([]string, 0)

	a.mu.Lock()
	for key, session := range a.reusableSessions {
		if strings.TrimSpace(session.workflowRunID) != workflowRunID {
			continue
		}
		sessionIDs = append(sessionIDs, session.sessionID)
		delete(a.reusableSessions, key)
	}
	a.mu.Unlock()

	return sessionIDs
}

func (a *PluginRunnerToolAdapter) collectLocalStaleReusableSessionIDs(maxAge time.Duration) []string {
	now := time.Now()
	sessionIDs := make([]string, 0)

	a.mu.Lock()
	for key, session := range a.reusableSessions {
		if now.Sub(session.lastUsedAt) <= maxAge {
			continue
		}
		sessionIDs = append(sessionIDs, session.sessionID)
		delete(a.reusableSessions, key)
	}
	a.mu.Unlock()

	return sessionIDs
}

func (a *PluginRunnerToolAdapter) stopSessionIDs(ctx context.Context, sessionIDs []string) (int, error) {
	uniqueIDs := make(map[string]struct{}, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			continue
		}
		uniqueIDs[sessionID] = struct{}{}
	}

	stoppedCount := 0
	var firstErr error

	for sessionID := range uniqueIDs {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := a.service.StopSession(stopCtx, sessionID)
		cancel()
		if err != nil {
			if isSessionNotFoundError(err) {
				continue
			}
			logger.Warn("failed to stop reusable session", "session_id", sessionID, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		stoppedCount++
	}

	return stoppedCount, firstErr
}

func isReusableRunnerSession(session pluginrunner_model.Session) bool {
	if session.Status != pluginrunner_model.SessionStatusRunning {
		return false
	}
	if session.Metadata == nil {
		return false
	}
	return strings.TrimSpace(session.Metadata.SessionPolicy) == sessionPolicyReuseWithinRun
}

func sessionLastActivityAt(session pluginrunner_model.Session) time.Time {
	if session.LastActivityAt != nil {
		return *session.LastActivityAt
	}
	return session.StartedAt
}

func isSessionNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status 404") || strings.Contains(msg, "not found")
}

func (a *PluginRunnerToolAdapter) startPluginSession(
	ctx context.Context,
	provider, tenantID, entrypoint, version string,
	opts ...pluginrunner_client.RequestOption,
) (*PluginSession, error) {
	tenantIDUint := parseTenantIDString(tenantID)

	session, err := a.service.StartSession(ctx, pluginrunner_model.StartSessionRequest{
		Name:       provider,
		Version:    version,
		Entrypoint: entrypoint,
		TenantID:   tenantIDUint,
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to start session: %w", err)
	}

	if err := a.service.WaitForSessionReady(ctx, session.ID, 30*time.Second); err != nil {
		_ = a.service.StopSession(ctx, session.ID)
		return nil, fmt.Errorf("session failed to become ready: %w", err)
	}

	return &PluginSession{
		ID:        session.ID,
		Provider:  provider,
		TenantID:  tenantID,
		StartedAt: time.Now(),
		adapter:   a,
	}, nil
}

func (a *PluginRunnerToolAdapter) resolvePluginRuntime(ctx context.Context, provider string) (string, string, error) {
	entrypoint := "main_runner"
	version := ""

	installations, err := a.service.ListInstalledPlugins(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to list installed plugins: %w", err)
	}

	for _, installation := range installations {
		if installation.Manifest.Name != provider {
			continue
		}

		if installation.Manifest.Runner.Entrypoint != "" {
			entrypoint = installation.Manifest.Runner.Entrypoint
		}
		version = installation.Manifest.Version
		break
	}

	if version == "" {
		return "", "", fmt.Errorf("plugin %s not found or not installed", provider)
	}

	return entrypoint, version, nil
}

func (a *PluginRunnerToolAdapter) resolveRunnerProviderName(ctx context.Context, tenantID, providerName string) (string, error) {
	installations, err := a.service.ListInstalledPlugins(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list installed plugins: %w", err)
	}

	if installation := findRunnerInstallationByName(installations, providerName); installation != nil {
		return installation.Manifest.Name, nil
	}
	for _, installation := range installations {
		if a.matchesInstalledProvider(ctx, tenantID, providerName, installation.Manifest) {
			return installation.Manifest.Name, nil
		}
	}

	return "", fmt.Errorf("plugin %s not found or not installed", providerName)
}

func (a *PluginRunnerToolAdapter) matchesInstalledProvider(ctx context.Context, tenantID, providerName string, manifest pluginrunner_model.PluginManifest) bool {
	if a.accountInstallationService == nil || tenantID == "" || providerName == "" {
		return false
	}
	if manifest.MarketplaceVersionID == "" {
		return false
	}

	info, err := a.accountInstallationService.GetInstalledPluginInfoByProviderName(ctx, tenantID, providerName)
	if err != nil || info == nil {
		if err != nil {
			logger.Warn("failed to resolve installed plugin info by provider name", "tenant_id", tenantID, "provider", providerName, "error", err)
		}
		return false
	}
	return info.MarketplaceVersionID == manifest.MarketplaceVersionID
}

func findRunnerInstallationByName(installations []pluginrunner_model.Installation, providerName string) *pluginrunner_model.Installation {
	for i := range installations {
		if installations[i].Manifest.Name == providerName {
			return &installations[i]
		}
	}
	return nil
}

func (a *PluginRunnerToolAdapter) resolveExecutionPolicy(
	ctx context.Context,
	tenantID, provider string,
) executionPolicy {
	policy := executionPolicy{
		WaitMode:             defaultWaitMode,
		StreamMode:           defaultStreamMode,
		SessionPolicy:        defaultSessionPolicy,
		SessionIdleTTL:       defaultSessionIdleTTL,
		SessionMaxLifetime:   defaultSessionMaxLifetime,
		SerializeInvocations: true,
	}

	if a.accountInstallationService == nil {
		return policy
	}

	declaration, err := a.accountInstallationService.GetDeclarationByProviderName(ctx, tenantID, provider)
	if err != nil {
		logger.Warn("failed to resolve plugin execution policy", "tenant_id", tenantID, "provider", provider, "error", err)
		return policy
	}
	if declaration == nil || declaration.Provider.ExecutionPolicy == nil {
		return policy
	}

	policyDefinition := declaration.Provider.ExecutionPolicy
	if mode := strings.TrimSpace(policyDefinition.WaitMode); mode != "" {
		policy.WaitMode = normalizeWaitMode(mode)
	}
	if mode := strings.TrimSpace(policyDefinition.StreamMode); mode != "" {
		policy.StreamMode = normalizeStreamMode(mode)
	}
	if sessionPolicy := strings.TrimSpace(policyDefinition.SessionPolicy); sessionPolicy != "" {
		policy.SessionPolicy = normalizeSessionPolicy(sessionPolicy)
	}
	if policyDefinition.SessionIdleTTLSeconds > 0 {
		policy.SessionIdleTTL = time.Duration(policyDefinition.SessionIdleTTLSeconds) * time.Second
	}
	if policyDefinition.SerializeInvocations != nil {
		policy.SerializeInvocations = *policyDefinition.SerializeInvocations
	}

	return policy
}

func normalizeWaitMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "terminal":
		return "terminal"
	default:
		return defaultWaitMode
	}
}

func normalizeStreamMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "aggregate":
		return "aggregate"
	default:
		return defaultStreamMode
	}
}

func normalizeSessionPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case sessionPolicyReuseWithinRun:
		return sessionPolicyReuseWithinRun
	default:
		return defaultSessionPolicy
	}
}

func getWorkflowRunIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	runID, ok := ctx.Value("workflow_run_id").(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(runID)
}

func buildReusableSessionKey(tenantID, workflowRunID, provider string) string {
	return fmt.Sprintf("%s:%s:%s", tenantID, workflowRunID, provider)
}

// ============================================
// Session-based Tool Execution
// ============================================

// PluginSession represents an active plugin session
type PluginSession struct {
	ID        string
	Provider  string
	TenantID  string
	StartedAt time.Time
	adapter   *PluginRunnerToolAdapter
}

// StartSession starts a new plugin session for tool invocation
func (a *PluginRunnerToolAdapter) StartSession(ctx context.Context, provider, tenantID string) (*PluginSession, error) {
	entrypoint, version, err := a.resolvePluginRuntime(ctx, provider)
	if err != nil {
		return nil, err
	}
	return a.startPluginSession(ctx, provider, tenantID, entrypoint, version)
}

// InvokeTool invokes a tool within the session
func (s *PluginSession) InvokeTool(ctx context.Context, tool string, params map[string]interface{}) ([]ToolInvokeMessage, error) {
	resp, err := s.adapter.service.InvokeTool(ctx, pluginrunner_model.ToolInvokeRequest{
		SessionID:  s.ID,
		Provider:   s.Provider,
		Tool:       tool,
		Parameters: params,
		Timeout:    defaultInvokeTimeoutSeconds,
		WaitMode:   defaultWaitMode,
		StreamMode: defaultStreamMode,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to invoke tool: %w", err)
	}

	return s.adapter.convertResponse(resp)
}

// Close closes the plugin session
func (s *PluginSession) Close(ctx context.Context) error {
	return s.adapter.service.StopSession(ctx, s.ID)
}

// ============================================
// Batch Tool Execution
// ============================================

// BatchInvokeRequest represents a request to invoke multiple tools
type BatchInvokeRequest struct {
	TenantID string
	UserID   string
	Provider string
	Tools    []BatchToolRequest
}

// BatchToolRequest represents a single tool in a batch request
type BatchToolRequest struct {
	Tool       string
	Parameters map[string]interface{}
}

// BatchInvokeResult represents the result of a batch tool invocation
type BatchInvokeResult struct {
	Results []BatchToolResult
	Error   error
}

// BatchToolResult represents the result of a single tool in a batch
type BatchToolResult struct {
	Tool     string
	Messages []ToolInvokeMessage
	Error    error
}

// BatchInvoke invokes multiple tools in a single session
func (a *PluginRunnerToolAdapter) BatchInvoke(ctx context.Context, req BatchInvokeRequest) (*BatchInvokeResult, error) {
	// Start a single session for all tools
	session, err := a.StartSession(ctx, req.Provider, req.TenantID)
	if err != nil {
		return nil, err
	}
	defer session.Close(ctx)

	result := &BatchInvokeResult{
		Results: make([]BatchToolResult, len(req.Tools)),
	}

	// Invoke each tool
	for i, toolReq := range req.Tools {
		messages, err := session.InvokeTool(ctx, toolReq.Tool, toolReq.Parameters)
		result.Results[i] = BatchToolResult{
			Tool:     toolReq.Tool,
			Messages: messages,
			Error:    err,
		}
	}

	return result, nil
}
