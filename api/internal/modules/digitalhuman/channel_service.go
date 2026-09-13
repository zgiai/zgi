package digitalhuman

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	maxChannelNameRunes        = 128
	maxExternalChannelIDRunes  = 255
	maxChannelPreviewRunes     = 280
	defaultDeliveryBatch       = 50
	defaultDeliveryLease       = 2 * time.Minute
	maxGroupMentionCallbackAge = 10 * time.Minute
	maxDeliveryAttempts        = 6
	channelAcknowledgementText = "⏳ 已收到，正在处理…"
	channelWakeFallback        = "@DigitalHuman"
	channelRuntimeInputReason  = "channel.runtime_input.accepted"
	channelTaskApprovalReason  = "channel.task_approval.accepted"
	channelConnectionTestTTL   = 5 * time.Minute
)

var errChannelDeliveryAttemptFailed = errors.New("channel delivery attempt failed")

var channelConnectionTestNamespace = uuid.MustParse("4be6d9f2-347e-4a19-bba4-08cc76a0704f")

type ChannelInvocationRuntime interface {
	SubmitInvocation(context.Context, Scope, invocation.Event, string, *uuid.UUID) (*Run, bool, error)
	SubmitChannelRuntimeInput(context.Context, Scope, uuid.UUID, uuid.UUID, ChannelRuntimeInputRequest) (bool, error)
}

// ChannelRuntimeInputRequest preserves the provider-neutral attributes needed
// to decide whether a callback may join the current turn or must be serialized
// as the next turn in the same external conversation.
type ChannelRuntimeInputRequest struct {
	ClientInputID    uuid.UUID
	Content          string
	ConversationType channelruntime.ConversationType
	SenderExternalID string
}

type ChannelSubjectStore interface {
	Definition(context.Context, Scope, uuid.UUID) (*Definition, error)
}

type embeddedChannelCredentialManager interface {
	CreateEmbeddedInstallation(context.Context, *channelruntime.Installation, map[string]string) error
	ReplaceEmbeddedCredential(context.Context, Scope, *channelruntime.Installation, map[string]string, int64, time.Time) error
}

type ChannelService struct {
	store       ChannelStore
	subjects    ChannelSubjectStore
	runtime     ChannelInvocationRuntime
	adapters    *channelruntime.Registry
	credentials ChannelCredentialResolver
	transient   ChannelTransientDispatcher
	markers     ChannelProcessingMarkerStore
	artifacts   channelArtifactLinker
	receivers   interface {
		Status(uuid.UUID) channelruntime.ReceiverStatus
		Retry(uuid.UUID)
	}
	apiBaseURL string
	now        func() time.Time
}

func (service *ChannelService) WithReceiverStatus(resolver interface {
	Status(uuid.UUID) channelruntime.ReceiverStatus
	Retry(uuid.UUID)
}) *ChannelService {
	if service != nil {
		service.receivers = resolver
	}
	return service
}

func NewChannelService(store ChannelStore, subjects ChannelSubjectStore, runtime ChannelInvocationRuntime, adapters *channelruntime.Registry, credentials ChannelCredentialResolver, transient ChannelTransientDispatcher) *ChannelService {
	if store == nil || subjects == nil || runtime == nil || adapters == nil || credentials == nil || transient == nil {
		panic("channel service requires store, subjects, runtime, adapters, credentials, and transient dispatcher")
	}
	service := &ChannelService{
		store: store, subjects: subjects, runtime: runtime, adapters: adapters, credentials: credentials,
		transient: transient,
		artifacts: toolFileChannelArtifactLinker{},
		now:       func() time.Time { return time.Now().UTC() },
	}
	service.markers, _ = store.(ChannelProcessingMarkerStore)
	return service
}

// WithAPIBaseURL configures the externally reachable ZGI API base URL used in
// channel setup instructions.
func (service *ChannelService) WithAPIBaseURL(rawURL string) *ChannelService {
	if service == nil {
		return service
	}
	value := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		service.apiBaseURL = ""
		return service
	}
	service.apiBaseURL = value
	return service
}

func (service *ChannelService) CreateInstallation(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request CreateChannelInstallationRequest) (*channelruntime.Installation, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.installation.create")
	}
	if _, err := service.subjects.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.create")
	}
	request.Provider = strings.ToLower(strings.TrimSpace(request.Provider))
	request.Name = strings.TrimSpace(request.Name)
	hasEmbeddedCredential := len(request.Credentials) > 0
	if request.Provider == "" || request.Name == "" ||
		(hasEmbeddedCredential == (request.ConnectionID != uuid.Nil)) ||
		!validRuneLength(request.Name, maxChannelNameRunes) {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.installation.create")
	}
	if _, err := service.adapters.Resolve(request.Provider); err != nil {
		return nil, applicationError(ErrChannelNotReady, "digital_human.channel.installation.create")
	}
	now := service.now()
	defaultSubjectType := runtimeidentity.SubjectTypeDigitalHuman
	defaultSubjectID := digitalHumanID
	installationID := uuid.New()
	connectionID := request.ConnectionID
	if hasEmbeddedCredential {
		connectionID = installationID
	}
	installation := &channelruntime.Installation{
		ID: installationID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		Provider: request.Provider, ConnectionID: connectionID, Name: request.Name,
		DefaultSubjectType: &defaultSubjectType, DefaultSubjectID: &defaultSubjectID,
		Status: channelruntime.InstallationStatusActive, Revision: 1,
		CreatedBy: scope.ActorID, UpdatedBy: scope.ActorID, CreatedAt: now, UpdatedAt: now,
	}
	if hasEmbeddedCredential {
		manager, ok := service.credentials.(embeddedChannelCredentialManager)
		if !ok {
			return nil, applicationError(ErrChannelNotReady, "digital_human.channel.installation.create")
		}
		if err := manager.CreateEmbeddedInstallation(ctx, installation, request.Credentials); err != nil {
			return nil, applicationError(err, "digital_human.channel.installation.create")
		}
		return installation, nil
	}
	credential, err := service.credentials.ResolveCredential(ctx, installation)
	if err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.create")
	}
	credential.Destroy()
	if err := service.store.CreateChannelInstallation(ctx, installation); err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.create")
	}
	return installation, nil
}

func (service *ChannelService) ListInstallations(ctx context.Context, scope Scope, digitalHumanID uuid.UUID) ([]*ChannelInstallationView, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.installation.list")
	}
	if _, err := service.subjects.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.list")
	}
	items, err := service.store.ListChannelInstallations(ctx, scope, digitalHumanID)
	if err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.list")
	}
	views := make([]*ChannelInstallationView, 0, len(items))
	for _, installation := range items {
		credentialMode := "connection"
		if installation.ConnectionID == installation.ID {
			credentialMode = "embedded"
		}
		receiver := service.receiverStatus(ctx, installation)
		webhookURL := ""
		if receiver.Mode == "webhook" {
			webhookURL = service.webhookURL(installation)
		}
		views = append(views, &ChannelInstallationView{
			Installation:     installation,
			ChannelContext:   service.credentials.ContextReadiness(ctx, installation),
			WebhookURL:       webhookURL,
			CredentialMode:   credentialMode,
			IdentityVerified: strings.TrimSpace(installation.ExternalTenantID) != "" && strings.TrimSpace(installation.ExternalBotID) != "",
			Receiver:         receiver,
		})
	}
	return views, nil
}

func (service *ChannelService) receiverStatus(ctx context.Context, installation *channelruntime.Installation) channelruntime.ReceiverStatus {
	status := channelruntime.ReceiverStatus{Mode: "webhook", State: channelruntime.ReceiverStateNotApplicable}
	provider, err := service.adapters.ResolveProvider(installation.Provider)
	if err != nil || provider.Receiver == nil {
		return status
	}
	// Derive mode from saved configuration, not the timing of the background
	// reconciler. Disabled installations and fresh API replicas must agree.
	credential, err := service.credentials.ResolveCredential(ctx, installation)
	if err != nil {
		return channelruntime.ReceiverStatus{Mode: "unknown", State: channelruntime.ReceiverStateError, ErrorCode: "credential_unavailable"}
	}
	status.Mode = provider.Receiver.Mode(*credential)
	credential.Destroy()
	if status.Mode == "webhook" {
		return status
	}
	status.State = channelruntime.ReceiverStatePending
	if installation.Status != channelruntime.InstallationStatusActive {
		status.State = channelruntime.ReceiverStateDisconnected
		return status
	}
	if service.receivers != nil {
		observed := service.receivers.Status(installation.ID)
		if observed.Revision == installation.Revision && (observed.Mode == status.Mode || observed.Mode == "") {
			observed.Mode = status.Mode
			return observed
		}
	}
	return status
}

func (service *ChannelService) webhookURL(installation *channelruntime.Installation) string {
	if service == nil || service.apiBaseURL == "" || installation == nil || installation.ID == uuid.Nil {
		return ""
	}
	provider := strings.ToLower(strings.TrimSpace(installation.Provider))
	if provider == "" {
		return ""
	}
	return service.apiBaseURL + "/console/api/webhooks/channels/" + url.PathEscape(provider) + "/" + installation.ID.String()
}

func (service *ChannelService) UpdateInstallation(ctx context.Context, scope Scope, digitalHumanID, installationID uuid.UUID, request UpdateChannelInstallationRequest) (*channelruntime.Installation, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil || installationID == uuid.Nil || request.ExpectedRevision < 1 {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.installation.update")
	}
	if _, err := service.subjects.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.update")
	}
	installation, err := service.store.ScopedChannelInstallationByID(ctx, scope, digitalHumanID, installationID)
	if err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.update")
	}
	changed := false
	if request.Name != nil {
		value := strings.TrimSpace(*request.Name)
		if value == "" || !validRuneLength(value, maxChannelNameRunes) {
			return nil, applicationError(ErrInvalidRequest, "digital_human.channel.installation.update")
		}
		installation.Name, changed = value, true
	}
	if request.Status != nil {
		if *request.Status != channelruntime.InstallationStatusActive && *request.Status != channelruntime.InstallationStatusDisabled {
			return nil, applicationError(ErrInvalidRequest, "digital_human.channel.installation.update")
		}
		installation.Status, changed = *request.Status, true
	}
	if !changed {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.installation.update")
	}
	installation.UpdatedAt = service.now()
	if err := service.store.UpdateChannelInstallation(ctx, scope, installation, request.ExpectedRevision); err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.update")
	}
	return installation, nil
}

func (service *ChannelService) UpdateInstallationCredential(
	ctx context.Context,
	scope Scope,
	digitalHumanID, installationID uuid.UUID,
	request UpdateChannelInstallationCredentialRequest,
) (*channelruntime.Installation, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil || installationID == uuid.Nil || request.ExpectedRevision < 1 || len(request.Credentials) == 0 {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.installation.credential.update")
	}
	if _, err := service.subjects.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.credential.update")
	}
	installation, err := service.store.ScopedChannelInstallationByID(ctx, scope, digitalHumanID, installationID)
	if err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.credential.update")
	}
	manager, ok := service.credentials.(embeddedChannelCredentialManager)
	if !ok {
		return nil, applicationError(ErrChannelNotReady, "digital_human.channel.installation.credential.update")
	}
	if err := manager.ReplaceEmbeddedCredential(ctx, scope, installation, request.Credentials, request.ExpectedRevision, service.now()); err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.credential.update")
	}
	return installation, nil
}

// VerifyInstallation rechecks only the capabilities that are safe to verify
// without sending a message or reading an arbitrary external conversation:
// the application credential and the provider bot identity. Callback receipt,
// delivery, history, and member-name permissions remain runtime-verified after
// the first real message.
func (service *ChannelService) VerifyInstallation(
	ctx context.Context,
	scope Scope,
	digitalHumanID, installationID uuid.UUID,
) (*ChannelInstallationVerification, error) {
	const operation = "digital_human.channel.installation.verify"
	if !validScope(scope) || digitalHumanID == uuid.Nil || installationID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, operation)
	}
	if _, err := service.subjects.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, applicationError(err, operation)
	}
	installation, err := service.store.ScopedChannelInstallationByID(ctx, scope, digitalHumanID, installationID)
	if err != nil {
		return nil, applicationError(err, operation)
	}
	checkedAt := service.now()
	if service.receivers != nil {
		service.receivers.Retry(installation.ID)
	}
	externalTenantID, externalSelfID, err := service.credentials.ResolveIdentity(ctx, installation)
	externalTenantID = strings.TrimSpace(externalTenantID)
	externalSelfID = strings.TrimSpace(externalSelfID)
	if err != nil || externalTenantID == "" || externalSelfID == "" ||
		!validRuneLength(externalTenantID, maxExternalChannelIDRunes) ||
		!validRuneLength(externalSelfID, maxExternalChannelIDRunes) {
		if err == nil {
			err = ErrChannelNotReady
		}
		return failedChannelInstallationVerification(err, service.credentials.ContextReadiness(ctx, installation), checkedAt), nil
	}
	if installation.ExternalTenantID != externalTenantID || installation.ExternalBotID != externalSelfID {
		expectedRevision := installation.Revision
		installation.ExternalTenantID = externalTenantID
		installation.ExternalBotID = externalSelfID
		installation.UpdatedAt = checkedAt
		if err := service.store.UpdateChannelInstallation(ctx, scope, installation, expectedRevision); err != nil {
			return nil, applicationError(err, operation)
		}
	}
	return &ChannelInstallationVerification{
		Status:              "verified",
		CredentialVerified:  true,
		BotIdentityVerified: true,
		ChannelContext:      service.credentials.ContextReadiness(ctx, installation),
		CheckedAt:           checkedAt,
	}, nil
}

func failedChannelInstallationVerification(err error, readiness ChannelContextReadiness, checkedAt time.Time) *ChannelInstallationVerification {
	code := integrations.ErrorCode(err)
	reasonCode := integrations.ErrorReasonCode(err)
	diagnostics := integrations.ProviderDiagnosticsFromError(err)
	failureCode := "temporarily_unavailable"
	retryable := false
	switch code {
	case integrations.ErrorCodeAuthInvalid, integrations.ErrorCodeConnectionExpired, integrations.ErrorCodeReconnectRequired:
		failureCode = "credentials_invalid"
	case integrations.ErrorCodeAccessDenied, integrations.ErrorCodeInsufficientScope:
		failureCode = "access_denied"
	case integrations.ErrorCodeInvalidInput:
		failureCode = "configuration_invalid"
	case integrations.ErrorCodeRateLimited:
		failureCode, retryable = "rate_limited", true
	case integrations.ErrorCodeTimeout, integrations.ErrorCodeUpstream:
		failureCode, retryable = "temporarily_unavailable", true
	case integrations.ErrorCodeResponseInvalid:
		failureCode = "response_invalid"
	case integrations.ErrorCodeProviderRejected:
		failureCode = "provider_rejected"
	}
	return &ChannelInstallationVerification{
		Status:         "failed",
		ChannelContext: readiness,
		CheckedAt:      checkedAt,
		Failure: &ChannelInstallationVerificationFailure{
			Code:                 failureCode,
			ReasonCode:           reasonCode,
			ProviderErrorCode:    diagnostics.ErrorCode,
			ProviderRequestID:    diagnostics.RequestID,
			ProviderHTTPStatus:   diagnostics.HTTPStatus,
			ProviderInvalidField: diagnostics.InvalidField,
			Retryable:            retryable,
		},
	}
}

// StartConnectionTest first performs the safe credential and identity check,
// then issues a stateless, bounded real-message test. The displayed code is
// not a credential; success is derived later from persisted inbound and final
// delivery evidence.
func (service *ChannelService) StartConnectionTest(
	ctx context.Context,
	scope Scope,
	digitalHumanID, installationID uuid.UUID,
	request StartChannelConnectionTestRequest,
) (*ChannelConnectionTest, error) {
	const operation = "digital_human.channel.connection_test.start"
	conversationType, err := connectionTestConversationType(request.ConversationType)
	if err != nil {
		return nil, applicationError(err, operation)
	}
	verification, err := service.VerifyInstallation(ctx, scope, digitalHumanID, installationID)
	if err != nil {
		return nil, err
	}
	if verification.Status != "verified" {
		return &ChannelConnectionTest{
			Status: ChannelConnectionTestConfigurationFailed, Readiness: ChannelConnectionTestReadinessUnavailable,
			ConversationType: conversationType, Verification: verification,
			Capabilities: connectionTestCapabilities(conversationType, verification.CredentialVerified, verification.BotIdentityVerified),
		}, nil
	}
	installation, err := service.store.ScopedChannelInstallationByID(ctx, scope, digitalHumanID, installationID)
	if err != nil {
		return nil, applicationError(err, operation)
	}
	adapter, err := service.adapters.Resolve(installation.Provider)
	if err != nil || !adapter.Capabilities().MessageRoundTripTest {
		return nil, applicationError(ErrChannelNotReady, operation)
	}
	if conversationType == channelruntime.ConversationTypeGroup &&
		(!adapter.Capabilities().ConversationCapabilityTest || !adapter.Capabilities().ContextRead) {
		return nil, applicationError(ErrChannelNotReady, operation)
	}
	if conversationType == channelruntime.ConversationTypeGroup {
		if _, ok := adapter.(channelruntime.ConversationCapabilityTester); !ok {
			return nil, applicationError(ErrChannelNotReady, operation)
		}
	}
	startedAt := service.now().UTC().Truncate(time.Millisecond)
	testID := channelConnectionTestID(installation.ID, conversationType, startedAt)
	testCode, testMessage := channelConnectionTestMessage(testID, conversationType)
	return &ChannelConnectionTest{
		ID: testID, Status: ChannelConnectionTestAwaitingMessage, Readiness: ChannelConnectionTestReadinessPending,
		ConversationType: conversationType, Verification: verification,
		TestCode: testCode, TestMessage: testMessage,
		StartedAt: startedAt, ExpiresAt: startedAt.Add(channelConnectionTestTTL),
		Capabilities: connectionTestCapabilities(conversationType, true, true),
	}, nil
}

func (service *ChannelService) GetConnectionTest(
	ctx context.Context,
	scope Scope,
	digitalHumanID, installationID, testID uuid.UUID,
	conversationType channelruntime.ConversationType,
	startedAt time.Time,
) (*ChannelConnectionTest, error) {
	const operation = "digital_human.channel.connection_test.get"
	if !validScope(scope) || digitalHumanID == uuid.Nil || installationID == uuid.Nil || testID == uuid.Nil || startedAt.IsZero() {
		return nil, applicationError(ErrInvalidRequest, operation)
	}
	conversationType, err := connectionTestConversationType(conversationType)
	if err != nil {
		return nil, applicationError(err, operation)
	}
	startedAt = startedAt.UTC().Truncate(time.Millisecond)
	if channelConnectionTestID(installationID, conversationType, startedAt) != testID {
		return nil, applicationError(ErrInvalidRequest, operation)
	}
	if _, err := service.subjects.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, applicationError(err, operation)
	}
	installation, err := service.store.ScopedChannelInstallationByID(ctx, scope, digitalHumanID, installationID)
	if err != nil {
		return nil, applicationError(err, operation)
	}
	adapter, err := service.adapters.Resolve(installation.Provider)
	if err != nil || !adapter.Capabilities().MessageRoundTripTest {
		return nil, applicationError(ErrChannelNotReady, operation)
	}
	if conversationType == channelruntime.ConversationTypeGroup && !adapter.Capabilities().ConversationCapabilityTest {
		return nil, applicationError(ErrChannelNotReady, operation)
	}
	if conversationType == channelruntime.ConversationTypeGroup {
		if _, ok := adapter.(channelruntime.ConversationCapabilityTester); !ok {
			return nil, applicationError(ErrChannelNotReady, operation)
		}
	}
	evidenceStore, ok := service.store.(ChannelConnectionTestStore)
	if !ok {
		return nil, applicationError(ErrChannelNotReady, operation)
	}
	testCode, testMessage := channelConnectionTestMessage(testID, conversationType)
	result := &ChannelConnectionTest{
		ID: testID, Status: ChannelConnectionTestAwaitingMessage, Readiness: ChannelConnectionTestReadinessPending,
		ConversationType: conversationType,
		TestCode:         testCode, TestMessage: testMessage,
		StartedAt: startedAt, ExpiresAt: startedAt.Add(channelConnectionTestTTL),
		Capabilities: connectionTestCapabilities(conversationType, true, true),
	}
	event, delivery, err := evidenceStore.ChannelConnectionTestEvidence(ctx, installationID, conversationType, testMessage, startedAt)
	if err != nil {
		return nil, applicationError(err, operation)
	}
	if event == nil {
		if !service.now().Before(result.ExpiresAt) {
			result.Status, result.FailureCode = ChannelConnectionTestExpired, "message_not_received"
			result.Readiness = ChannelConnectionTestReadinessUnavailable
			setConnectionTestCapability(result, inboundConnectionTestCapability(conversationType), ChannelConnectionCapabilityFailed, result.FailureCode, nil)
		}
		return result, nil
	}
	result.MessageReceived = true
	result.InboundEventID = &event.ID
	setConnectionTestCapability(result, inboundConnectionTestCapability(conversationType), ChannelConnectionCapabilityPassed, "", nil)
	if event.Status == channelruntime.InboundEventFailed || event.Status == channelruntime.InboundEventIgnored {
		result.Status = ChannelConnectionTestFailed
		result.FailureCode = connectionTestInboundFailureCode(event.ReasonCode)
		result.Readiness = ChannelConnectionTestReadinessUnavailable
		setConnectionTestCapability(result, inboundConnectionTestCapability(conversationType), ChannelConnectionCapabilityFailed, result.FailureCode, nil)
		return result, nil
	}
	if delivery == nil {
		result.Status = ChannelConnectionTestMessageReceived
		if !service.now().Before(result.ExpiresAt) {
			result.Status, result.FailureCode = ChannelConnectionTestFailed, "reply_not_created"
			result.Readiness = ChannelConnectionTestReadinessUnavailable
			setConnectionTestCapability(result, ChannelConnectionCapabilityReplyDelivery, ChannelConnectionCapabilityFailed, result.FailureCode, nil)
		}
		return result, nil
	}
	result.DeliveryID = &delivery.ID
	switch delivery.Status {
	case channelruntime.DeliverySucceeded:
		result.Status, result.ReplyDelivered = ChannelConnectionTestSucceeded, true
		setConnectionTestCapability(result, ChannelConnectionCapabilityReplyDelivery, ChannelConnectionCapabilityPassed, "", nil)
	case channelruntime.DeliveryFailed, channelruntime.DeliveryUnknown, channelruntime.DeliveryDismissed:
		result.Status = ChannelConnectionTestFailed
		result.FailureCode = connectionTestDeliveryFailureCode(delivery.ErrorCode)
		setConnectionTestCapability(result, ChannelConnectionCapabilityReplyDelivery, ChannelConnectionCapabilityFailed, result.FailureCode, nil)
	default:
		result.Status = ChannelConnectionTestReplying
	}
	if result.Status == ChannelConnectionTestSucceeded || result.Status == ChannelConnectionTestFailed {
		service.testConversationCapabilities(ctx, scope, digitalHumanID, installation, adapter, event, result)
	}
	result.Readiness = connectionTestReadiness(result)
	return result, nil
}

func channelConnectionTestID(installationID uuid.UUID, conversationType channelruntime.ConversationType, startedAt time.Time) uuid.UUID {
	name := installationID.String() + "|" + startedAt.UTC().Truncate(time.Millisecond).Format(time.RFC3339Nano)
	if conversationType == channelruntime.ConversationTypeGroup {
		name += "|group"
	}
	return uuid.NewSHA1(channelConnectionTestNamespace, []byte(name))
}

func channelConnectionTestMessage(testID uuid.UUID, conversationType channelruntime.ConversationType) (string, string) {
	compact := strings.ToUpper(strings.ReplaceAll(testID.String(), "-", ""))
	code := "ZGI-" + compact[:8]
	if conversationType == channelruntime.ConversationTypeGroup {
		return code, "群聊连接测试 " + code + "，请回复“群聊连接成功”。"
	}
	return code, "连接测试 " + code + "，请回复“连接成功”。"
}

func connectionTestConversationType(value channelruntime.ConversationType) (channelruntime.ConversationType, error) {
	switch value {
	case "", channelruntime.ConversationTypeDirect:
		return channelruntime.ConversationTypeDirect, nil
	case channelruntime.ConversationTypeGroup:
		return channelruntime.ConversationTypeGroup, nil
	default:
		return "", ErrInvalidRequest
	}
}

func connectionTestCapabilities(
	conversationType channelruntime.ConversationType,
	credentialsVerified, botIdentityVerified bool,
) []ChannelConnectionCapability {
	credentialStatus, botIdentityStatus := ChannelConnectionCapabilityPending, ChannelConnectionCapabilityPending
	if credentialsVerified {
		credentialStatus = ChannelConnectionCapabilityPassed
	}
	if botIdentityVerified {
		botIdentityStatus = ChannelConnectionCapabilityPassed
	}
	capabilities := []ChannelConnectionCapability{
		{ID: ChannelConnectionCapabilityCredentials, Status: credentialStatus},
		{ID: ChannelConnectionCapabilityBotIdentity, Status: botIdentityStatus},
		{ID: inboundConnectionTestCapability(conversationType), Status: ChannelConnectionCapabilityPending},
	}
	if conversationType == channelruntime.ConversationTypeGroup {
		capabilities = append(capabilities,
			ChannelConnectionCapability{ID: ChannelConnectionCapabilityMessageHistory, Status: ChannelConnectionCapabilityPending},
			ChannelConnectionCapability{ID: ChannelConnectionCapabilityMemberNames, Status: ChannelConnectionCapabilityPending},
		)
	}
	return append(capabilities, ChannelConnectionCapability{ID: ChannelConnectionCapabilityReplyDelivery, Status: ChannelConnectionCapabilityPending})
}

func inboundConnectionTestCapability(conversationType channelruntime.ConversationType) string {
	if conversationType == channelruntime.ConversationTypeGroup {
		return ChannelConnectionCapabilityGroupMention
	}
	return ChannelConnectionCapabilityDirectInbound
}

func setConnectionTestCapability(
	result *ChannelConnectionTest,
	id string,
	status ChannelConnectionCapabilityStatus,
	failureCode string,
	requiredPermissions []string,
) {
	if result == nil {
		return
	}
	for index := range result.Capabilities {
		if result.Capabilities[index].ID != id {
			continue
		}
		result.Capabilities[index].Status = status
		result.Capabilities[index].FailureCode = strings.TrimSpace(failureCode)
		result.Capabilities[index].RequiredPermissions = append([]string(nil), requiredPermissions...)
		return
	}
}

func (service *ChannelService) testConversationCapabilities(
	ctx context.Context,
	scope Scope,
	digitalHumanID uuid.UUID,
	installation *channelruntime.Installation,
	adapter channelruntime.Adapter,
	event *channelruntime.InboundEvent,
	result *ChannelConnectionTest,
) {
	if result == nil || result.ConversationType != channelruntime.ConversationTypeGroup || installation == nil || event == nil {
		return
	}
	tester, ok := adapter.(channelruntime.ConversationCapabilityTester)
	if !ok {
		return
	}
	checks := tester.TestConversationCapabilities(ctx, channelruntime.ConversationCapabilityTestRequest{
		OrganizationID: installation.OrganizationID,
		WorkspaceID:    installation.WorkspaceID,
		ActorID:        scope.ActorID,
		InstallationID: installation.ID,
		ConnectionID:   installation.ConnectionID,
		Subject: runtimeidentity.SubjectRef{
			Type:           runtimeidentity.SubjectTypeDigitalHuman,
			ID:             digitalHumanID,
			OrganizationID: installation.OrganizationID,
			WorkspaceID:    installation.WorkspaceID,
		},
		ExternalConversationID: event.ExternalConversationID,
		CurrentMessageID:       event.ProviderMessageID,
		CurrentSenderID:        event.SenderExternalID,
		OccurredAt:             event.ReceivedAt,
	})
	for _, check := range checks {
		status := ChannelConnectionCapabilityFailed
		if check.Status == channelruntime.CapabilityCheckPassed {
			status = ChannelConnectionCapabilityPassed
		}
		setConnectionTestCapability(result, string(check.Capability), status, check.FailureCode, check.RequiredPermissions)
	}
}

func connectionTestReadiness(result *ChannelConnectionTest) ChannelConnectionTestReadiness {
	if result == nil {
		return ChannelConnectionTestReadinessUnavailable
	}
	switch result.Status {
	case ChannelConnectionTestConfigurationFailed, ChannelConnectionTestFailed, ChannelConnectionTestExpired:
		return ChannelConnectionTestReadinessUnavailable
	case ChannelConnectionTestSucceeded:
		for _, capability := range result.Capabilities {
			if capability.Status == ChannelConnectionCapabilityFailed {
				return ChannelConnectionTestReadinessPartial
			}
		}
		return ChannelConnectionTestReadinessAvailable
	default:
		return ChannelConnectionTestReadinessPending
	}
}

func connectionTestInboundFailureCode(reason string) string {
	switch strings.TrimSpace(reason) {
	case "channel.binding.not_found":
		return "digital_human_unavailable"
	case "channel.conversation.reserve_failed", "channel.invocation.submit_failed", "channel.runtime_input.submit_failed":
		return "runtime_unavailable"
	case "channel.callback.stale":
		return "callback_stale"
	default:
		return "message_rejected"
	}
}

func connectionTestDeliveryFailureCode(code string) string {
	switch strings.TrimSpace(code) {
	case "channel.provider.permission_missing", "channel.provider.send_not_allowed":
		return "send_permission_missing"
	case "channel.provider.permission_or_resource_denied":
		return "permission_or_resource_denied"
	case "channel.provider.bot_capability_disabled":
		return "bot_capability_disabled"
	case "channel.provider.app_unavailable":
		return "app_unavailable"
	case "channel.provider.bot_not_in_chat":
		return "bot_not_in_chat"
	case "channel.delivery.outcome_unknown":
		return "reply_outcome_unknown"
	default:
		return "reply_failed"
	}
}

func (service *ChannelService) DeleteInstallation(ctx context.Context, scope Scope, digitalHumanID, installationID uuid.UUID) error {
	if !validScope(scope) || digitalHumanID == uuid.Nil || installationID == uuid.Nil {
		return applicationError(ErrInvalidRequest, "digital_human.channel.installation.delete")
	}
	if _, err := service.subjects.Definition(ctx, scope, digitalHumanID); err != nil {
		return applicationError(err, "digital_human.channel.installation.delete")
	}
	return applicationError(service.store.DeleteChannelInstallation(ctx, scope, digitalHumanID, installationID), "digital_human.channel.installation.delete")
}

// ProbeWebhook verifies that a public callback address resolves to one active
// installation and registered provider. Provider consoles do not include a
// signature in this reachability request, so the probe must remain read-only.
func (service *ChannelService) ProbeWebhook(ctx context.Context, provider string, installationID uuid.UUID) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || installationID == uuid.Nil {
		return ErrChannelInstallationNotFound
	}
	installation, err := service.store.ChannelInstallationByID(ctx, installationID)
	if err != nil || installation == nil || installation.Status != channelruntime.InstallationStatusActive ||
		!strings.EqualFold(strings.TrimSpace(installation.Provider), provider) {
		return ErrChannelInstallationNotFound
	}
	if _, err := service.adapters.Resolve(installation.Provider); err != nil {
		return ErrChannelNotReady
	}
	return nil
}

func (service *ChannelService) HandleWebhook(ctx context.Context, provider string, installationID uuid.UUID, request channelruntime.WebhookRequest) (*ChannelWebhookResult, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	installation, err := service.store.ChannelInstallationByID(ctx, installationID)
	if err != nil || installation.Status != channelruntime.InstallationStatusActive ||
		provider == "" || !strings.EqualFold(installation.Provider, provider) {
		return nil, ErrChannelInstallationNotFound
	}
	adapter, err := service.adapters.Resolve(installation.Provider)
	if err != nil {
		return nil, ErrChannelNotReady
	}
	credential, err := service.credentials.ResolveCredential(ctx, installation)
	if err != nil {
		return nil, ErrChannelNotReady
	}
	defer credential.Destroy()
	verified, err := adapter.Verify(ctx, request, *credential)
	if err != nil {
		return nil, err
	}
	if verified == nil {
		return nil, channelruntime.ErrInvalidWebhook
	}
	if verified.ImmediateResponse != nil {
		return &ChannelWebhookResult{Response: *verified.ImmediateResponse}, nil
	}
	message, err := adapter.Normalize(ctx, verified, channelruntime.NormalizationContext{
		Identity: channelruntime.InstallationIdentity{
			ExternalTenantID: installation.ExternalTenantID,
			ExternalSelfID:   installation.ExternalBotID,
		},
	})
	if err != nil {
		return nil, err
	}
	return service.handleInboundMessage(ctx, installation, adapter, message, request.ReceivedAt)
}

// HandleInboundMessage routes a provider-normalized message through the same
// dedupe, wake, conversation, and invocation pipeline used by HTTP callbacks.
// Long-lived receivers must never bypass this boundary.
func (service *ChannelService) HandleInboundMessage(
	ctx context.Context,
	installationID uuid.UUID,
	expectedRevision int64,
	message *channelruntime.InboundMessage,
	receivedAt time.Time,
) (*ChannelWebhookResult, error) {
	installation, err := service.store.ChannelInstallationByID(ctx, installationID)
	if err != nil || installation == nil || installation.Status != channelruntime.InstallationStatusActive {
		return nil, ErrChannelInstallationNotFound
	}
	// Fence events from the old connection after credential rotation, transport
	// switching or disabling, even before the next receiver reconciliation.
	if expectedRevision < 1 || installation.Revision != expectedRevision {
		return nil, ErrChannelConflict
	}
	adapter, err := service.adapters.Resolve(installation.Provider)
	if err != nil {
		return nil, ErrChannelNotReady
	}
	return service.handleInboundMessage(ctx, installation, adapter, message, receivedAt)
}

func (service *ChannelService) handleInboundMessage(
	ctx context.Context,
	installation *channelruntime.Installation,
	adapter channelruntime.Adapter,
	message *channelruntime.InboundMessage,
	receivedAt time.Time,
) (*ChannelWebhookResult, error) {
	if !validInboundChannelMessage(message) || !strings.EqualFold(message.Provider, installation.Provider) {
		return nil, channelruntime.ErrInvalidWebhook
	}
	acknowledgement := adapter.Acknowledgement()
	if installation.ExternalTenantID != "" && installation.ExternalTenantID != message.ExternalTenantID {
		return nil, channelruntime.ErrWebhookUnauthorized
	}
	now := service.now()
	receivedAt = receivedAt.UTC()
	if receivedAt.IsZero() {
		receivedAt = now
	}
	staleWake := staleGroupMentionCallback(message, receivedAt)
	var route *ChannelRoute
	var routeErr error
	if staleWake {
		// A delayed provider retry must remain auditable, but it must not create
		// a new channel route or wake a digital human long after the conversation
		// has moved on.
		route, routeErr = service.store.ResolveChannelRoute(ctx, installation.ID, message.ExternalConversationID)
	} else {
		route, routeErr = service.resolveOrCreateChannelRoute(ctx, installation, message)
	}
	var bindingID *uuid.UUID
	if routeErr == nil && route != nil && route.Binding != nil {
		id := route.Binding.ID
		bindingID = &id
	}
	inbound := &channelruntime.InboundEvent{
		ID: uuid.New(), OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID,
		InstallationID: installation.ID, BindingID: bindingID,
		ProviderEventID: message.ProviderEventID, ProviderMessageID: message.ProviderMessageID,
		ExternalConversationID: message.ExternalConversationID, SenderExternalID: message.SenderExternalID,
		ConversationType: message.ConversationType, ContentPreview: channelMessagePreview(message), MentionOnly: message.MentionOnly,
		EventType: channelEventType(message), PayloadDigest: channelMessageDigest(message),
		Status: channelruntime.InboundEventReceived, ReceivedAt: receivedAt, CreatedAt: now, UpdatedAt: now,
	}
	stored, created, err := service.store.RecordInboundEvent(ctx, inbound)
	if err != nil {
		return nil, err
	}
	if !created {
		if !sameInboundEvent(stored, inbound) {
			return nil, channelruntime.ErrInvalidWebhook
		}
		result := &ChannelWebhookResult{Response: acknowledgement, Duplicate: true, EventID: stored.ID, Ignored: stored.Status == channelruntime.InboundEventIgnored}
		if stored.InvocationRunID != nil {
			result.Accepted, result.RunID = true, stored.InvocationRunID
			return result, nil
		}
		if stored.Status == channelruntime.InboundEventAccepted && stored.ReasonCode == channelRuntimeInputReason {
			result.Accepted = true
			return result, nil
		}
		if stored.Status == channelruntime.InboundEventIgnored {
			return result, nil
		}
	}
	if staleWake {
		_ = service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventIgnored, "channel.callback.stale", nil, now)
		return &ChannelWebhookResult{Response: acknowledgement, Duplicate: !created, Ignored: true, EventID: stored.ID}, nil
	}
	if routeErr != nil || route == nil || route.Binding == nil || route.Mapping == nil {
		_ = service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventIgnored, "channel.binding.not_found", nil, now)
		return &ChannelWebhookResult{Response: acknowledgement, Duplicate: !created, Ignored: true, EventID: stored.ID}, nil
	}
	if !shouldWakeChannel(route, message) {
		_ = service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventIgnored, "channel.wake_policy.not_matched", nil, now)
		return &ChannelWebhookResult{Response: acknowledgement, Duplicate: !created, Ignored: true, EventID: stored.ID}, nil
	}
	conversationID, err := service.store.EnsureChannelConversationID(ctx, route.Mapping.ID, uuid.New(), message.OccurredAt)
	if err != nil {
		_ = service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventFailed, "channel.conversation.reserve_failed", nil, now)
		return nil, err
	}
	route.Mapping.LocalConversationID = &conversationID
	if approvalRuntime, ok := service.runtime.(interface {
		ContinueChannelTaskApproval(context.Context, Scope, uuid.UUID, uuid.UUID, string) (bool, error)
	}); ok {
		handled, approvalErr := approvalRuntime.ContinueChannelTaskApproval(
			ctx,
			Scope{OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID, ActorID: installation.CreatedBy},
			route.Binding.SubjectID,
			conversationID,
			channelInvocationQuery(message),
		)
		if approvalErr != nil {
			_ = service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventFailed, "channel.task_approval.failed", nil, now)
			return nil, approvalErr
		}
		if handled {
			if err := service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventAccepted, channelTaskApprovalReason, nil, now); err != nil {
				return nil, err
			}
			return &ChannelWebhookResult{Response: acknowledgement, Accepted: true, Duplicate: !created, EventID: stored.ID}, nil
		}
	}
	acceptedAsRuntimeInput, err := service.runtime.SubmitChannelRuntimeInput(
		ctx,
		Scope{OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID, ActorID: installation.CreatedBy},
		route.Binding.SubjectID,
		conversationID,
		ChannelRuntimeInputRequest{
			ClientInputID: stored.ID, Content: channelRuntimeInputContent(installation.ID, message),
			ConversationType: message.ConversationType, SenderExternalID: message.SenderExternalID,
		},
	)
	if err != nil {
		_ = service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventFailed, "channel.runtime_input.submit_failed", nil, now)
		return nil, err
	}
	if acceptedAsRuntimeInput {
		if err := service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventAccepted, channelRuntimeInputReason, nil, now); err != nil {
			return nil, err
		}
		return &ChannelWebhookResult{Response: acknowledgement, Accepted: true, Duplicate: !created, EventID: stored.ID}, nil
	}
	result, err := service.submitChannelInvocation(ctx, installation, route, message, stored, now, !created)
	if result != nil {
		result.Response = acknowledgement
	}
	return result, err
}

func channelRuntimeInputContent(installationID uuid.UUID, message *channelruntime.InboundMessage) string {
	content := channelInvocationQuery(message)
	if message == nil || message.ConversationType != channelruntime.ConversationTypeGroup {
		return content
	}
	author := strings.TrimSpace(message.SenderDisplayName)
	if author == "" {
		author = stableChannelAuthorAlias(installationID, message.ExternalConversationID, message.SenderExternalID)
	}
	return "Channel message from " + author + ":\n" + content
}

func (service *ChannelService) resolveOrCreateChannelRoute(
	ctx context.Context,
	installation *channelruntime.Installation,
	message *channelruntime.InboundMessage,
) (*ChannelRoute, error) {
	route, err := service.store.ResolveChannelRoute(ctx, installation.ID, message.ExternalConversationID)
	if err == nil || !errors.Is(err, ErrChannelBindingNotFound) {
		return route, err
	}
	if installation.DefaultSubjectType == nil || installation.DefaultSubjectID == nil ||
		*installation.DefaultSubjectType != runtimeidentity.SubjectTypeDigitalHuman || *installation.DefaultSubjectID == uuid.Nil {
		return route, err
	}
	if message.ConversationType == channelruntime.ConversationTypeGroup &&
		message.WakeReason != channelruntime.WakeReasonMention && message.WakeReason != channelruntime.WakeReasonReplyToSelf {
		return route, err
	}

	scope := Scope{
		OrganizationID: installation.OrganizationID,
		WorkspaceID:    installation.WorkspaceID,
		ActorID:        installation.CreatedBy,
	}
	if _, definitionErr := service.subjects.Definition(ctx, scope, *installation.DefaultSubjectID); definitionErr != nil {
		return route, definitionErr
	}

	now := service.now()
	localConversationID := uuid.New()
	lastEventAt := message.OccurredAt
	if lastEventAt.IsZero() {
		lastEventAt = now
	}
	mapping := &channelruntime.ConversationMapping{
		ID: uuid.New(), OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID,
		InstallationID: installation.ID, ExternalConversationID: message.ExternalConversationID,
		ConversationType: message.ConversationType, LocalConversationID: &localConversationID,
		LastEventAt: &lastEventAt, CreatedAt: now, UpdatedAt: now,
	}
	wakePolicy := channelruntime.WakePolicyDirectAndMention
	if message.ConversationType == channelruntime.ConversationTypeGroup {
		wakePolicy = channelruntime.WakePolicyMentionOnly
	}
	binding := &channelruntime.Binding{
		ID: uuid.New(), OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID,
		InstallationID: installation.ID, ConversationMappingID: mapping.ID,
		SubjectType: *installation.DefaultSubjectType, SubjectID: *installation.DefaultSubjectID,
		WakePolicy: wakePolicy, Enabled: true, Revision: 1,
		CreatedBy: installation.CreatedBy, UpdatedBy: installation.UpdatedBy, CreatedAt: now, UpdatedAt: now,
	}
	if createErr := service.store.CreateChannelBinding(ctx, mapping, binding); createErr != nil {
		if !errors.Is(createErr, ErrChannelConflict) {
			return nil, createErr
		}
		return service.store.ResolveChannelRoute(ctx, installation.ID, message.ExternalConversationID)
	}
	return &ChannelRoute{Installation: installation, Mapping: mapping, Binding: binding}, nil
}

func (service *ChannelService) submitChannelInvocation(
	ctx context.Context,
	installation *channelruntime.Installation,
	route *ChannelRoute,
	message *channelruntime.InboundMessage,
	stored *channelruntime.InboundEvent,
	now time.Time,
	duplicate bool,
) (*ChannelWebhookResult, error) {
	target := runtimeidentity.SubjectRef{
		Type: route.Binding.SubjectType, ID: route.Binding.SubjectID,
		OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID,
	}
	event := invocation.Event{
		// The persisted inbound row is the canonical provider identity. This is
		// important when a provider retries the same message under a different
		// event ID while the first callback is still submitting its run.
		ID: stored.ProviderEventID, Type: channelInvocationType(message),
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: installation.Provider, InstallationID: installation.ID.String()},
		Target: target, Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: message.SenderExternalID},
		OccurredAt: message.OccurredAt,
		Context: map[string]any{
			"provider_message_id":      message.ProviderMessageID,
			"external_conversation_id": message.ExternalConversationID,
			"conversation_type":        string(message.ConversationType),
			"sender_display_name":      message.SenderDisplayName,
			"root_message_id":          message.RootMessageID,
			"parent_message_id":        message.ParentMessageID,
			"thread_id":                message.ThreadID,
			"mention_only":             message.MentionOnly,
			"channel_inbound_event_id": stored.ID.String(),
			"channel_binding_id":       route.Binding.ID.String(),
		},
		ReplyTo: &invocation.ReplyAddress{
			Provider: installation.Provider, InstallationID: installation.ID.String(),
			ConversationID: message.ExternalConversationID, ThreadID: message.ExternalThreadID,
		},
	}
	if message.ConversationType == channelruntime.ConversationTypeGroup {
		event.ReplyTo.RecipientID = strings.TrimSpace(message.SenderExternalID)
	}
	scope := Scope{OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID, ActorID: installation.CreatedBy}
	run, _, err := service.runtime.SubmitInvocation(ctx, scope, event, channelInvocationQuery(message), route.Mapping.LocalConversationID)
	if err != nil {
		_ = service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventFailed, "channel.invocation.submit_failed", nil, now)
		return nil, err
	}
	if err := service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventAccepted, "", &run.ID, now); err != nil {
		return nil, err
	}
	policy := channelResponsePolicyFromRun(run)
	if policy.AcknowledgementEnabled {
		adapter, resolveErr := service.adapters.Resolve(installation.Provider)
		_, supportsMarker := adapter.(channelruntime.ProcessingMarkerAdapter)
		if resolveErr == nil && adapter.Capabilities().ProcessingMarker && supportsMarker && service.markers != nil {
			service.scheduleProcessingMarker(ctx, installation, route, stored, run.ID, now)
		} else {
			// Providers without a native processing marker retain the existing
			// best-effort acknowledgement message.
			_ = service.transient.EnqueueTransient(ctx, ChannelTransientDeliveryRequest{
				RunID: run.ID, Kind: channelruntime.DeliveryKindAcknowledgement,
				DeliveryKey: "accepted", Text: channelAcknowledgementText,
			})
		}
	}
	return &ChannelWebhookResult{Accepted: true, Duplicate: duplicate, EventID: stored.ID, RunID: &run.ID}, nil
}

func (service *ChannelService) scheduleProcessingMarker(
	ctx context.Context,
	installation *channelruntime.Installation,
	route *ChannelRoute,
	inbound *channelruntime.InboundEvent,
	runID uuid.UUID,
	now time.Time,
) {
	if service == nil || service.markers == nil || installation == nil || route == nil || route.Binding == nil || inbound == nil {
		return
	}
	marker := &channelruntime.ProcessingMarker{
		ID: processingMarkerID(inbound.ID), OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID,
		InstallationID: installation.ID, InboundEventID: inbound.ID, RunID: runID,
		Provider: strings.ToLower(strings.TrimSpace(installation.Provider)), ConnectionID: installation.ConnectionID,
		ActorID: installation.CreatedBy, SubjectType: route.Binding.SubjectType, SubjectID: route.Binding.SubjectID,
		ProviderMessageID: strings.TrimSpace(inbound.ProviderMessageID), Status: channelruntime.ProcessingMarkerPendingAdd,
		AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	created, err := service.markers.CreateProcessingMarker(ctx, marker)
	if err != nil {
		logger.WarnContext(ctx, "DigitalHuman processing marker could not be scheduled", "run_id", runID.String(), err)
		return
	}
	if !created {
		return
	}
	if dispatcher, ok := service.transient.(ChannelProcessingMarkerDispatcher); ok {
		if err := dispatcher.EnqueueProcessingMarker(ctx, marker.ID); err != nil {
			// The durable sweep will pick the row up even when the low-latency
			// queue notification cannot be published.
			logger.WarnContext(ctx, "DigitalHuman processing marker wake-up was delayed", "run_id", runID.String(), err)
		}
	}
}

func channelMessagePreview(message *channelruntime.InboundMessage) string {
	if message == nil {
		return ""
	}
	preview := strings.Join(strings.Fields(message.Text), " ")
	runes := []rune(preview)
	if len(runes) > maxChannelPreviewRunes {
		preview = string(runes[:maxChannelPreviewRunes])
	}
	return preview
}

func channelInvocationQuery(message *channelruntime.InboundMessage) string {
	if message == nil {
		return ""
	}
	if text := strings.TrimSpace(message.Text); text != "" {
		return text
	}
	if !message.MentionOnly {
		return ""
	}
	for _, mention := range message.Mentions {
		if name := strings.TrimSpace(mention.Name); name != "" {
			return "@" + name
		}
	}
	return channelWakeFallback
}

func (service *ChannelService) ListEvents(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, query ListQuery) (*ChannelEventListResult, error) {
	query, err := normalizeListQuery(scope, query)
	if err != nil || digitalHumanID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.event.list")
	}
	items, total, err := service.store.ListInboundEvents(ctx, scope, digitalHumanID, query)
	if err != nil {
		return nil, applicationError(err, "digital_human.channel.event.list")
	}
	return &ChannelEventListResult{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, HasMore: int64(query.Page*query.PageSize) < total}, nil
}

func (service *ChannelService) ListDeliveries(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, query ListQuery) (*ChannelDeliveryListResult, error) {
	query, err := normalizeListQuery(scope, query)
	if err != nil || digitalHumanID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.delivery.list")
	}
	items, total, err := service.store.ListChannelDeliveries(ctx, scope, digitalHumanID, query)
	if err != nil {
		return nil, applicationError(err, "digital_human.channel.delivery.list")
	}
	return &ChannelDeliveryListResult{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, HasMore: int64(query.Page*query.PageSize) < total}, nil
}

func (service *ChannelService) RetryDelivery(ctx context.Context, scope Scope, digitalHumanID, deliveryID uuid.UUID) error {
	return applicationError(service.store.RequeueDelivery(ctx, scope, digitalHumanID, deliveryID, service.now()), "digital_human.channel.delivery.retry")
}

func (service *ChannelService) DismissDelivery(ctx context.Context, scope Scope, digitalHumanID, deliveryID uuid.UUID) error {
	return applicationError(service.store.DismissDelivery(ctx, scope, digitalHumanID, deliveryID, service.now()), "digital_human.channel.delivery.dismiss")
}

// DispatchDeliveries executes already-committed send intents without touching
// the originating model run. Deterministic delivery IDs are reused on retry.
func (service *ChannelService) DispatchDeliveries(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = defaultDeliveryBatch
	}
	items, err := service.store.ClaimDeliveryBatch(ctx, limit, service.now())
	if err != nil {
		return err
	}
	var combined error
	for _, delivery := range items {
		adapter, resolveErr := service.adapters.Resolve(delivery.Provider)
		if resolveErr != nil {
			combined = errors.Join(combined, service.finishDeliveryFailure(ctx, delivery, ErrChannelNotReady))
			continue
		}
		payload, decodeErr := decodeDeliveryPayload(delivery)
		if decodeErr != nil {
			combined = errors.Join(combined, service.store.FailDelivery(
				ctx, delivery.ID, channelruntime.DeliveryFailed, "channel.delivery.payload_invalid",
				"The committed delivery payload is invalid.", service.now(),
			))
			continue
		}
		links, linkErr := service.artifacts.Resolve(ctx, delivery, payload.Artifacts)
		if linkErr != nil {
			combined = errors.Join(combined, service.finishDeliveryFailure(ctx, delivery, linkErr))
			continue
		}
		text := renderChannelDeliveryText(payload.Text, links)
		if strings.TrimSpace(text) == "" {
			combined = errors.Join(combined, service.store.FailDelivery(
				ctx, delivery.ID, channelruntime.DeliveryFailed, "channel.delivery.payload_invalid",
				"The committed delivery payload is invalid.", service.now(),
			))
			continue
		}
		receipt, deliveryErr := adapter.Deliver(ctx, channelruntime.DeliveryRequest{
			OrganizationID: delivery.OrganizationID, WorkspaceID: delivery.WorkspaceID, ActorID: delivery.ActorID,
			InstallationID: delivery.InstallationID, ConnectionID: delivery.ConnectionID,
			Subject: runtimeidentity.SubjectRef{
				Type: delivery.SubjectType, ID: delivery.SubjectID,
				OrganizationID: delivery.OrganizationID, WorkspaceID: delivery.WorkspaceID,
			},
			RunID: delivery.RunID, DeliveryID: delivery.ID,
			ExternalConversationID: delivery.ExternalConversationID, ExternalThreadID: delivery.ExternalThreadID,
			Text: text, Mentions: append([]channelruntime.DeliveryMention(nil), payload.Mentions...),
		})
		if deliveryErr != nil {
			combined = errors.Join(combined, service.finishDeliveryFailure(ctx, delivery, deliveryErr))
			continue
		}
		if receipt == nil || strings.TrimSpace(receipt.ProviderMessageID) == "" {
			combined = errors.Join(combined, service.finishDeliveryFailure(ctx, delivery, channelruntime.ErrDeliveryOutcomeUnknown))
			continue
		}
		if err := service.store.SucceedDelivery(ctx, delivery.ID, receipt, service.now()); err != nil {
			combined = errors.Join(combined, err)
		}
	}
	return combined
}

func (service *ChannelService) finishDeliveryFailure(ctx context.Context, delivery *channelruntime.Delivery, deliveryErr error) error {
	if delivery == nil {
		return ErrInvalidRequest
	}
	now := service.now()
	if errors.Is(deliveryErr, channelruntime.ErrDeliveryOutcomeUnknown) {
		return safeDeliveryAttemptError(service.store.FailDelivery(
			ctx, delivery.ID, channelruntime.DeliveryUnknown, channelErrorCode(deliveryErr), safeChannelErrorMessage(deliveryErr), now,
		))
	}
	if !channelDeliveryRetryable(deliveryErr) || delivery.AttemptCount >= maxDeliveryAttempts {
		return safeDeliveryAttemptError(service.store.FailDelivery(
			ctx, delivery.ID, channelruntime.DeliveryFailed, channelErrorCode(deliveryErr), safeChannelErrorMessage(deliveryErr), now,
		))
	}
	return safeDeliveryAttemptError(service.store.RetryDelivery(
		ctx, delivery.ID, now.Add(deliveryBackoff(delivery.AttemptCount)), channelErrorCode(deliveryErr), safeChannelErrorMessage(deliveryErr),
	))
}

// channelDeliveryRetryable limits automatic retries to failures that can
// plausibly recover without configuration or authorization changes. Confirmed
// provider rejections become visible terminal failures after the first attempt.
func channelDeliveryRetryable(err error) bool {
	if err == nil || errors.Is(err, channelruntime.ErrDeliveryOutcomeUnknown) {
		return false
	}
	var integrationError *integrations.Error
	if !errors.As(err, &integrationError) {
		return true
	}
	switch integrationError.Code {
	case integrations.ErrorCodeRateLimited,
		integrations.ErrorCodeTimeout,
		integrations.ErrorCodeUpstream,
		integrations.ErrorCodeOperationInProgress:
		return true
	default:
		return false
	}
}

func safeDeliveryAttemptError(persistErr error) error {
	if persistErr != nil {
		return errors.Join(errChannelDeliveryAttemptFailed, persistErr)
	}
	return errChannelDeliveryAttemptFailed
}

func (service *ChannelService) ReconcileDeliveries(ctx context.Context) error {
	now := service.now()
	var markerRecovery error
	if service.markers != nil {
		markerRecovery = service.markers.RecoverStaleProcessingMarkers(ctx, now.Add(-defaultDeliveryLease), now)
	}
	return errors.Join(
		service.store.MarkStaleDeliveriesUnknown(ctx, now.Add(-defaultDeliveryLease), now),
		markerRecovery,
		service.DispatchDeliveries(ctx, defaultDeliveryBatch),
		service.DispatchProcessingMarkers(ctx, defaultDeliveryBatch),
	)
}

// DispatchProcessingMarkers reconciles provider-native working indicators. A
// failed add is never retried because the provider write is non-idempotent; an
// empty marker ID is later reconciled by the provider using app ownership.
func (service *ChannelService) DispatchProcessingMarkers(ctx context.Context, limit int) error {
	if service == nil || service.markers == nil {
		return nil
	}
	if limit <= 0 {
		limit = defaultDeliveryBatch
	}
	items, err := service.markers.ClaimProcessingMarkerBatch(ctx, limit, service.now())
	if err != nil {
		return err
	}
	var combined error
	for _, marker := range items {
		adapter, resolveErr := service.adapters.Resolve(marker.Provider)
		provider, supported := adapter.(channelruntime.ProcessingMarkerAdapter)
		if resolveErr != nil || !supported {
			if resolveErr == nil {
				resolveErr = channelruntime.ErrUnsupportedEvent
			}
			combined = errors.Join(combined, service.markers.FailProcessingMarker(
				ctx, marker.ID, channelErrorCode(resolveErr), safeChannelErrorMessage(resolveErr), service.now(),
			))
			continue
		}
		request := channelruntime.ProcessingMarkerRequest{
			OrganizationID: marker.OrganizationID, WorkspaceID: marker.WorkspaceID,
			InstallationID: marker.InstallationID, ConnectionID: marker.ConnectionID,
			RunID: marker.RunID, InboundMessageID: marker.ProviderMessageID,
		}
		switch marker.Status {
		case channelruntime.ProcessingMarkerAdding:
			receipt, addErr := provider.AddProcessingMarker(ctx, request)
			providerMarkerID, providerRequestID := "", ""
			if receipt != nil {
				providerMarkerID = strings.TrimSpace(receipt.ProviderMarkerID)
				providerRequestID = strings.TrimSpace(receipt.ProviderRequestID)
			}
			// A write timeout may still have created the reaction. Move to active
			// even on error so cleanup can list and remove an app-owned Typing.
			if persistErr := service.markers.ActivateProcessingMarker(ctx, marker.ID, providerMarkerID, providerRequestID, service.now()); persistErr != nil {
				combined = errors.Join(combined, persistErr)
			}
			if addErr != nil {
				logger.WarnContext(ctx, "DigitalHuman processing marker was not confirmed", "run_id", marker.RunID.String(), addErr)
			}
		case channelruntime.ProcessingMarkerRemoving:
			removeErr := provider.RemoveProcessingMarker(ctx, request, marker.ProviderMarkerID)
			if removeErr == nil {
				combined = errors.Join(combined, service.markers.RemoveProcessingMarker(ctx, marker.ID, service.now()))
				continue
			}
			if channelDeliveryRetryable(removeErr) && marker.AttemptCount < maxDeliveryAttempts {
				combined = errors.Join(combined, service.markers.RetryProcessingMarkerRemoval(
					ctx, marker.ID, service.now().Add(deliveryBackoff(marker.AttemptCount)),
					channelErrorCode(removeErr), safeChannelErrorMessage(removeErr),
				))
				continue
			}
			combined = errors.Join(combined, service.markers.FailProcessingMarker(
				ctx, marker.ID, channelErrorCode(removeErr), safeChannelErrorMessage(removeErr), service.now(),
			))
		}
	}
	return combined
}

func shouldWakeChannel(route *ChannelRoute, message *channelruntime.InboundMessage) bool {
	if route == nil || route.Installation == nil || route.Binding == nil || message == nil || !route.Binding.Enabled {
		return false
	}
	if message.ConversationType == channelruntime.ConversationTypeDirect {
		return route.Binding.WakePolicy == channelruntime.WakePolicyDirectAndMention &&
			message.WakeReason == channelruntime.WakeReasonDirectMessage
	}
	return message.WakeReason == channelruntime.WakeReasonMention ||
		message.WakeReason == channelruntime.WakeReasonReplyToSelf
}

func staleGroupMentionCallback(message *channelruntime.InboundMessage, receivedAt time.Time) bool {
	if message == nil || message.ConversationType != channelruntime.ConversationTypeGroup ||
		message.OccurredAt.IsZero() || receivedAt.IsZero() ||
		(message.WakeReason != channelruntime.WakeReasonMention && message.WakeReason != channelruntime.WakeReasonReplyToSelf) {
		return false
	}
	return receivedAt.Sub(message.OccurredAt.UTC()) > maxGroupMentionCallbackAge
}

func channelInvocationType(message *channelruntime.InboundMessage) invocation.EventType {
	if message != nil && (message.WakeReason == channelruntime.WakeReasonMention || message.WakeReason == channelruntime.WakeReasonReplyToSelf) {
		return invocation.EventTypeChannelMention
	}
	return invocation.EventTypeChannelMessage
}

func channelEventType(message *channelruntime.InboundMessage) string {
	if message == nil {
		return "channel.message"
	}
	switch message.WakeReason {
	case channelruntime.WakeReasonDirectMessage:
		return "channel.direct_message"
	case channelruntime.WakeReasonMention:
		return "channel.mention"
	case channelruntime.WakeReasonReplyToSelf:
		return "channel.reply_to_self"
	case channelruntime.WakeReasonNone:
		if message.ConversationType == channelruntime.ConversationTypeGroup {
			return "channel.group_message"
		}
	}
	return "channel.message"
}

func validInboundChannelMessage(message *channelruntime.InboundMessage) bool {
	if message == nil || strings.TrimSpace(message.Provider) == "" || strings.TrimSpace(message.ProviderEventID) == "" ||
		strings.TrimSpace(message.ProviderMessageID) == "" || strings.TrimSpace(message.ExternalConversationID) == "" ||
		strings.TrimSpace(message.SenderExternalID) == "" {
		return false
	}
	if message.ConversationType != channelruntime.ConversationTypeDirect && message.ConversationType != channelruntime.ConversationTypeGroup {
		return false
	}
	switch message.WakeReason {
	case channelruntime.WakeReasonNone, channelruntime.WakeReasonDirectMessage,
		channelruntime.WakeReasonMention, channelruntime.WakeReasonReplyToSelf:
		return true
	default:
		return false
	}
}

func channelMessageDigest(message *channelruntime.InboundMessage) string {
	payload, _ := json.Marshal(struct {
		ProviderMessageID      string                          `json:"provider_message_id"`
		ExternalTenantID       string                          `json:"external_tenant_id"`
		ExternalConversationID string                          `json:"external_conversation_id"`
		ExternalThreadID       string                          `json:"external_thread_id"`
		RootMessageID          string                          `json:"root_message_id"`
		ParentMessageID        string                          `json:"parent_message_id"`
		ThreadID               string                          `json:"thread_id"`
		ConversationType       channelruntime.ConversationType `json:"conversation_type"`
		SenderExternalID       string                          `json:"sender_external_id"`
		SenderDisplayName      string                          `json:"sender_display_name"`
		Text                   string                          `json:"text"`
		Mentions               []channelruntime.Mention        `json:"mentions"`
		WakeReason             channelruntime.WakeReason       `json:"wake_reason"`
		MentionOnly            bool                            `json:"mention_only"`
		OccurredAt             time.Time                       `json:"occurred_at"`
	}{
		ProviderMessageID: message.ProviderMessageID, ExternalTenantID: message.ExternalTenantID,
		ExternalConversationID: message.ExternalConversationID, ExternalThreadID: message.ExternalThreadID,
		RootMessageID: message.RootMessageID, ParentMessageID: message.ParentMessageID, ThreadID: message.ThreadID,
		ConversationType: message.ConversationType, SenderExternalID: message.SenderExternalID,
		SenderDisplayName: message.SenderDisplayName,
		Text:              message.Text, Mentions: message.Mentions, WakeReason: message.WakeReason,
		MentionOnly: message.MentionOnly, OccurredAt: message.OccurredAt.UTC(),
	})
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func sameInboundEvent(stored, replay *channelruntime.InboundEvent) bool {
	if stored == nil || replay == nil {
		return false
	}
	sameProviderIdentity := stored.ProviderEventID == replay.ProviderEventID ||
		(stored.ProviderMessageID != "" && stored.ProviderMessageID == replay.ProviderMessageID)
	return sameProviderIdentity && stored.InstallationID == replay.InstallationID &&
		stored.ProviderMessageID == replay.ProviderMessageID &&
		stored.ExternalConversationID == replay.ExternalConversationID &&
		stored.SenderExternalID == replay.SenderExternalID &&
		stored.PayloadDigest == replay.PayloadDigest
}

func validChannelText(value string, maxRunes int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= maxRunes
}

func channelErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, channelruntime.ErrDeliveryOutcomeUnknown) {
		return "channel.delivery.outcome_unknown"
	}
	switch reason := integrations.ErrorReasonCode(err); reason {
	case "credentials_invalid", "bot_not_in_chat", "bot_capability_disabled", "app_unavailable",
		"permission_missing", "send_not_allowed", "permission_or_resource_denied":
		return "channel.provider." + reason
	}
	return "channel.delivery.failed"
}

func safeChannelErrorMessage(err error) string {
	if errors.Is(err, channelruntime.ErrDeliveryOutcomeUnknown) {
		return "The provider outcome could not be confirmed; automatic retry was stopped."
	}
	return "The provider rejected or could not complete this delivery."
}

func deliveryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > maxDeliveryAttempts {
		attempt = maxDeliveryAttempts
	}
	return time.Duration(1<<(attempt-1)) * 5 * time.Second
}

func decodeDeliveryPayload(delivery *channelruntime.Delivery) (*channelDeliveryPayloadData, error) {
	if delivery == nil || len(delivery.Payload) == 0 {
		return nil, ErrInvalidRequest
	}
	var payload channelDeliveryPayloadData
	if json.Unmarshal(delivery.Payload, &payload) != nil || len(payload.Artifacts) > maxChannelDeliveryArtifacts ||
		len(payload.Mentions) > maxChannelDeliveryMentions ||
		(strings.TrimSpace(payload.Text) == "" && len(payload.Artifacts) == 0) ||
		(payload.Text != "" && !validChannelText(payload.Text, maxChannelDeliveryRunes)) {
		return nil, fmt.Errorf("decode channel delivery payload")
	}
	for _, artifact := range payload.Artifacts {
		if strings.TrimSpace(artifact.ToolFileID) == "" || utf8.RuneCountInString(artifact.ToolFileID) > 128 ||
			utf8.RuneCountInString(artifact.Filename) > maxChannelArtifactNameRunes ||
			(artifact.Extension != "" && normalizeChannelArtifactExtension(artifact.Extension) == "") {
			return nil, fmt.Errorf("decode channel delivery payload")
		}
	}
	for index := range payload.Mentions {
		mention := &payload.Mentions[index]
		mention.ExternalID = strings.TrimSpace(mention.ExternalID)
		mention.DisplayName = strings.TrimSpace(mention.DisplayName)
		if mention.ExternalID == "" || utf8.RuneCountInString(mention.ExternalID) > maxChannelMentionIDRunes ||
			utf8.RuneCountInString(mention.DisplayName) > maxChannelMentionNameRunes ||
			!utf8.ValidString(mention.ExternalID) || !utf8.ValidString(mention.DisplayName) {
			return nil, fmt.Errorf("decode channel delivery payload")
		}
	}
	payload.Text = strings.TrimSpace(payload.Text)
	return &payload, nil
}

func uuidPointer(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	result := value
	return &result
}
