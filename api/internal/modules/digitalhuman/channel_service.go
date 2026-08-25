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
)

var errChannelDeliveryAttemptFailed = errors.New("channel delivery attempt failed")

type ChannelInvocationRuntime interface {
	SubmitInvocation(context.Context, Scope, invocation.Event, string, *uuid.UUID) (*Run, bool, error)
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
	artifacts   channelArtifactLinker
	receivers   interface {
		Status(uuid.UUID) channelruntime.ReceiverStatus
	}
	apiBaseURL string
	now        func() time.Time
}

func (service *ChannelService) WithReceiverStatus(resolver interface {
	Status(uuid.UUID) channelruntime.ReceiverStatus
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
	return &ChannelService{
		store: store, subjects: subjects, runtime: runtime, adapters: adapters, credentials: credentials,
		transient: transient,
		artifacts: toolFileChannelArtifactLinker{},
		now:       func() time.Time { return time.Now().UTC() },
	}
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
	externalTenantID, externalBotID, err := service.credentials.ResolveIdentity(ctx, installation)
	if err != nil || !validRuneLength(externalTenantID, maxExternalChannelIDRunes) ||
		!validRuneLength(externalBotID, maxExternalChannelIDRunes) {
		return nil, applicationError(ErrChannelNotReady, "digital_human.channel.installation.create")
	}
	installation.ExternalTenantID = externalTenantID
	installation.ExternalBotID = externalBotID
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
	items, err := service.store.ListChannelInstallations(ctx, scope)
	if err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.list")
	}
	views := make([]*ChannelInstallationView, 0, len(items))
	for _, installation := range items {
		credentialMode := "connection"
		if installation.ConnectionID == installation.ID {
			credentialMode = "embedded"
		}
		receiver := channelruntime.ReceiverStatus{Mode: "webhook", State: channelruntime.ReceiverStateNotApplicable}
		if service.receivers != nil {
			receiver = service.receivers.Status(installation.ID)
		}
		views = append(views, &ChannelInstallationView{
			Installation:   installation,
			ChannelContext: service.credentials.ContextReadiness(ctx, installation),
			WebhookURL:     service.webhookURL(installation),
			CredentialMode: credentialMode,
			Receiver:       receiver,
		})
	}
	return views, nil
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
	installation, err := service.store.ScopedChannelInstallationByID(ctx, scope, installationID)
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
	installation, err := service.store.ScopedChannelInstallationByID(ctx, scope, installationID)
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

func (service *ChannelService) DeleteInstallation(ctx context.Context, scope Scope, digitalHumanID, installationID uuid.UUID) error {
	if !validScope(scope) || digitalHumanID == uuid.Nil || installationID == uuid.Nil {
		return applicationError(ErrInvalidRequest, "digital_human.channel.installation.delete")
	}
	if _, err := service.subjects.Definition(ctx, scope, digitalHumanID); err != nil {
		return applicationError(err, "digital_human.channel.installation.delete")
	}
	return applicationError(service.store.DeleteChannelInstallation(ctx, scope, installationID), "digital_human.channel.installation.delete")
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
	message *channelruntime.InboundMessage,
	receivedAt time.Time,
) (*ChannelWebhookResult, error) {
	installation, err := service.store.ChannelInstallationByID(ctx, installationID)
	if err != nil || installation == nil || installation.Status != channelruntime.InstallationStatusActive {
		return nil, ErrChannelInstallationNotFound
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
	result, err := service.submitChannelInvocation(ctx, installation, route, message, stored, now, !created)
	if result != nil {
		result.Response = acknowledgement
	}
	return result, err
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
		// Acknowledgements are UX-only and intentionally bypass the durable
		// delivery outbox. Their switch is independent from progress messages.
		_ = service.transient.EnqueueTransient(ctx, ChannelTransientDeliveryRequest{
			RunID: run.ID, Kind: channelruntime.DeliveryKindAcknowledgement,
			DeliveryKey: "accepted", Text: channelAcknowledgementText,
		})
	}
	return &ChannelWebhookResult{Accepted: true, Duplicate: duplicate, EventID: stored.ID, RunID: &run.ID}, nil
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
	if delivery.AttemptCount >= maxDeliveryAttempts {
		return safeDeliveryAttemptError(service.store.FailDelivery(
			ctx, delivery.ID, channelruntime.DeliveryFailed, channelErrorCode(deliveryErr), safeChannelErrorMessage(deliveryErr), now,
		))
	}
	return safeDeliveryAttemptError(service.store.RetryDelivery(
		ctx, delivery.ID, now.Add(deliveryBackoff(delivery.AttemptCount)), channelErrorCode(deliveryErr), safeChannelErrorMessage(deliveryErr),
	))
}

func safeDeliveryAttemptError(persistErr error) error {
	if persistErr != nil {
		return errors.Join(errChannelDeliveryAttemptFailed, persistErr)
	}
	return errChannelDeliveryAttemptFailed
}

func (service *ChannelService) ReconcileDeliveries(ctx context.Context) error {
	now := service.now()
	return errors.Join(
		service.store.MarkStaleDeliveriesUnknown(ctx, now.Add(-defaultDeliveryLease), now),
		service.DispatchDeliveries(ctx, defaultDeliveryBatch),
	)
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
