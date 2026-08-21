package digitalhuman

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	defaultDeliveryBatch       = 50
	defaultDeliveryLease       = 2 * time.Minute
	maxDeliveryAttempts        = 6
	channelAcknowledgementText = "⏳ 已收到，正在处理…"
)

var errChannelDeliveryAttemptFailed = errors.New("channel delivery attempt failed")

type ChannelInvocationRuntime interface {
	SubmitInvocation(context.Context, Scope, invocation.Event, string, *uuid.UUID) (*Run, bool, error)
}

type ChannelSubjectStore interface {
	Definition(context.Context, Scope, uuid.UUID) (*Definition, error)
}

type ChannelService struct {
	store       ChannelStore
	subjects    ChannelSubjectStore
	runtime     ChannelInvocationRuntime
	adapters    *channelruntime.Registry
	credentials ChannelCredentialResolver
	transient   ChannelTransientDispatcher
	artifacts   channelArtifactLinker
	now         func() time.Time
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

func (service *ChannelService) CreateInstallation(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request CreateChannelInstallationRequest) (*channelruntime.Installation, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.installation.create")
	}
	if _, err := service.subjects.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, applicationError(err, "digital_human.channel.installation.create")
	}
	request.Provider = strings.ToLower(strings.TrimSpace(request.Provider))
	request.Name = strings.TrimSpace(request.Name)
	if request.Provider == "" || request.ConnectionID == uuid.Nil || request.Name == "" ||
		!validRuneLength(request.Name, maxChannelNameRunes) {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.installation.create")
	}
	if _, err := service.adapters.Resolve(request.Provider); err != nil {
		return nil, applicationError(ErrChannelNotReady, "digital_human.channel.installation.create")
	}
	now := service.now()
	defaultSubjectType := runtimeidentity.SubjectTypeDigitalHuman
	defaultSubjectID := digitalHumanID
	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		Provider: request.Provider, ConnectionID: request.ConnectionID, Name: request.Name,
		DefaultSubjectType: &defaultSubjectType, DefaultSubjectID: &defaultSubjectID,
		Status: channelruntime.InstallationStatusActive, Revision: 1,
		CreatedBy: scope.ActorID, UpdatedBy: scope.ActorID, CreatedAt: now, UpdatedAt: now,
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
	return items, applicationError(err, "digital_human.channel.installation.list")
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

func (service *ChannelService) DeleteInstallation(ctx context.Context, scope Scope, digitalHumanID, installationID uuid.UUID, request DeleteChannelInstallationRequest) error {
	if !validScope(scope) || digitalHumanID == uuid.Nil || installationID == uuid.Nil {
		return applicationError(ErrInvalidRequest, "digital_human.channel.installation.delete")
	}
	if _, err := service.subjects.Definition(ctx, scope, digitalHumanID); err != nil {
		return applicationError(err, "digital_human.channel.installation.delete")
	}
	return applicationError(service.store.DeleteChannelInstallation(ctx, scope, installationID, request), "digital_human.channel.installation.delete")
}

func (service *ChannelService) CreateBinding(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request CreateChannelBindingRequest) (*ChannelBindingView, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil || request.InstallationID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.binding.create")
	}
	if _, err := service.subjects.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, applicationError(err, "digital_human.channel.binding.create")
	}
	installation, err := service.store.ScopedChannelInstallationByID(ctx, scope, request.InstallationID)
	if err != nil {
		return nil, applicationError(err, "digital_human.channel.binding.create")
	}
	if installation.Status != channelruntime.InstallationStatusActive {
		return nil, applicationError(ErrChannelNotReady, "digital_human.channel.binding.create")
	}
	request.ExternalConversationID = strings.TrimSpace(request.ExternalConversationID)
	if request.WakePolicy == "" {
		request.WakePolicy = channelruntime.WakePolicyDirectAndMention
	}
	if request.ExternalConversationID == "" || !validRuneLength(request.ExternalConversationID, maxExternalChannelIDRunes) ||
		(request.ConversationType != channelruntime.ConversationTypeDirect && request.ConversationType != channelruntime.ConversationTypeGroup) ||
		!validWakePolicy(request.WakePolicy) {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.binding.create")
	}
	if request.ConversationType == channelruntime.ConversationTypeDirect && request.WakePolicy != channelruntime.WakePolicyDirectAndMention {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.binding.create")
	}
	if request.ConversationType == channelruntime.ConversationTypeGroup && strings.TrimSpace(installation.ExternalBotID) == "" {
		return nil, applicationError(ErrChannelNotReady, "digital_human.channel.binding.create")
	}
	now := service.now()
	mapping := &channelruntime.ConversationMapping{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		InstallationID: installation.ID, ExternalConversationID: request.ExternalConversationID,
		ConversationType: request.ConversationType, LocalConversationID: uuidPointer(uuid.New()),
		CreatedAt: now, UpdatedAt: now,
	}
	binding := &channelruntime.Binding{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		InstallationID: installation.ID, ConversationMappingID: mapping.ID,
		SubjectType: runtimeidentity.SubjectTypeDigitalHuman, SubjectID: digitalHumanID,
		WakePolicy: request.WakePolicy, Enabled: true, Revision: 1,
		CreatedBy: scope.ActorID, UpdatedBy: scope.ActorID, CreatedAt: now, UpdatedAt: now,
	}
	if err := service.store.CreateChannelBinding(ctx, mapping, binding); err != nil {
		return nil, applicationError(err, "digital_human.channel.binding.create")
	}
	return &ChannelBindingView{Binding: binding, Mapping: mapping}, nil
}

func (service *ChannelService) ListBindings(ctx context.Context, scope Scope, digitalHumanID uuid.UUID) ([]*ChannelBindingView, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.binding.list")
	}
	items, err := service.store.ListChannelBindings(ctx, scope, digitalHumanID)
	return items, applicationError(err, "digital_human.channel.binding.list")
}

func (service *ChannelService) ListDiscoveries(ctx context.Context, scope Scope, digitalHumanID uuid.UUID) ([]*ChannelConversationDiscovery, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.channel.discovery.list")
	}
	if _, err := service.subjects.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, applicationError(err, "digital_human.channel.discovery.list")
	}
	items, err := service.store.ListDiscoveredChannelConversations(ctx, scope)
	return items, applicationError(err, "digital_human.channel.discovery.list")
}

func (service *ChannelService) DeleteBinding(ctx context.Context, scope Scope, digitalHumanID, bindingID uuid.UUID) error {
	return applicationError(service.store.DeleteChannelBinding(ctx, scope, digitalHumanID, bindingID), "digital_human.channel.binding.delete")
}

func (service *ChannelService) HandleWebhook(ctx context.Context, installationID uuid.UUID, request channelruntime.WebhookRequest) (*ChannelWebhookResult, error) {
	installation, err := service.store.ChannelInstallationByID(ctx, installationID)
	if err != nil || installation.Status != channelruntime.InstallationStatusActive {
		return nil, ErrChannelInstallationNotFound
	}
	adapter, err := service.adapters.Resolve(installation.Provider)
	if err != nil {
		return nil, ErrChannelNotReady
	}
	security, cleanup, err := service.credentials.ResolveSecurity(ctx, installation)
	if err != nil {
		return nil, ErrChannelNotReady
	}
	defer func() {
		security.Destroy()
		cleanup()
	}()
	verified, err := adapter.Verify(ctx, request, security)
	if err != nil {
		return nil, err
	}
	if verified == nil {
		return nil, channelruntime.ErrInvalidWebhook
	}
	if verified.Challenge != "" {
		return &ChannelWebhookResult{Challenge: verified.Challenge}, nil
	}
	message, err := adapter.Normalize(ctx, verified)
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, channelruntime.ErrInvalidWebhook
	}
	if installation.ExternalTenantID != "" && installation.ExternalTenantID != message.ExternalTenantID {
		return nil, channelruntime.ErrWebhookUnauthorized
	}
	route, routeErr := service.resolveOrCreateChannelRoute(ctx, installation, message)
	var bindingID *uuid.UUID
	if routeErr == nil && route != nil && route.Binding != nil {
		id := route.Binding.ID
		bindingID = &id
	}
	now := service.now()
	inbound := &channelruntime.InboundEvent{
		ID: uuid.New(), OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID,
		InstallationID: installation.ID, BindingID: bindingID,
		ProviderEventID: message.ProviderEventID, ProviderMessageID: message.ProviderMessageID,
		ExternalConversationID: message.ExternalConversationID, SenderExternalID: message.SenderExternalID,
		EventType: feishuEventType(message), PayloadDigest: channelMessageDigest(message),
		Status: channelruntime.InboundEventReceived, ReceivedAt: request.ReceivedAt.UTC(), CreatedAt: now, UpdatedAt: now,
	}
	if inbound.ReceivedAt.IsZero() {
		inbound.ReceivedAt = now
	}
	stored, created, err := service.store.RecordInboundEvent(ctx, inbound)
	if err != nil {
		return nil, err
	}
	if !created {
		if !sameInboundEvent(stored, inbound) {
			return nil, channelruntime.ErrInvalidWebhook
		}
		result := &ChannelWebhookResult{Duplicate: true, EventID: stored.ID, Ignored: stored.Status == channelruntime.InboundEventIgnored}
		if stored.InvocationRunID != nil {
			result.Accepted, result.RunID = true, stored.InvocationRunID
			return result, nil
		}
		if stored.Status == channelruntime.InboundEventIgnored {
			return result, nil
		}
	}
	if routeErr != nil || route == nil || route.Binding == nil || route.Mapping == nil {
		_ = service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventIgnored, "channel.binding.not_found", nil, now)
		return &ChannelWebhookResult{Duplicate: !created, Ignored: true, EventID: stored.ID}, nil
	}
	if !shouldWakeChannel(route, message) {
		_ = service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventIgnored, "channel.wake_policy.not_matched", nil, now)
		return &ChannelWebhookResult{Duplicate: !created, Ignored: true, EventID: stored.ID}, nil
	}
	conversationID, err := service.store.EnsureChannelConversationID(ctx, route.Mapping.ID, uuid.New(), message.OccurredAt)
	if err != nil {
		_ = service.store.MarkInboundEvent(ctx, stored.ID, channelruntime.InboundEventFailed, "channel.conversation.reserve_failed", nil, now)
		return nil, err
	}
	route.Mapping.LocalConversationID = &conversationID
	return service.submitChannelInvocation(ctx, installation, route, message, stored, now, !created)
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
	if message.ConversationType == channelruntime.ConversationTypeGroup && !channelMessageMentionsBot(installation, message) {
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
			"channel_inbound_event_id": stored.ID.String(),
			"channel_binding_id":       route.Binding.ID.String(),
		},
		ReplyTo: &invocation.ReplyAddress{
			Provider: installation.Provider, InstallationID: installation.ID.String(),
			ConversationID: message.ExternalConversationID, ThreadID: message.ExternalThreadID,
		},
	}
	scope := Scope{OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID, ActorID: installation.CreatedBy}
	run, _, err := service.runtime.SubmitInvocation(ctx, scope, event, message.Text, route.Mapping.LocalConversationID)
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
			ExternalConversationID: delivery.ExternalConversationID, ExternalThreadID: delivery.ExternalThreadID, Text: text,
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

func validWakePolicy(policy channelruntime.WakePolicy) bool {
	switch policy {
	case channelruntime.WakePolicyDirectAndMention, channelruntime.WakePolicyMentionOnly:
		return true
	default:
		return false
	}
}

func shouldWakeChannel(route *ChannelRoute, message *channelruntime.InboundMessage) bool {
	if route == nil || route.Installation == nil || route.Binding == nil || message == nil || !route.Binding.Enabled {
		return false
	}
	if message.ConversationType == channelruntime.ConversationTypeDirect {
		return route.Binding.WakePolicy == channelruntime.WakePolicyDirectAndMention
	}
	return channelMessageMentionsBot(route.Installation, message)
}

func channelMessageMentionsBot(installation *channelruntime.Installation, message *channelruntime.InboundMessage) bool {
	if installation == nil || message == nil {
		return false
	}
	botID := strings.TrimSpace(installation.ExternalBotID)
	if botID == "" {
		return false
	}
	for _, mention := range message.Mentions {
		if strings.TrimSpace(mention.ExternalID) == botID {
			return true
		}
	}
	return false
}

func channelInvocationType(message *channelruntime.InboundMessage) invocation.EventType {
	if message != nil && message.ConversationType == channelruntime.ConversationTypeGroup {
		return invocation.EventTypeChannelMention
	}
	return invocation.EventTypeChannelMessage
}

func feishuEventType(message *channelruntime.InboundMessage) string {
	if message == nil {
		return "channel.message"
	}
	if message.ConversationType == channelruntime.ConversationTypeGroup {
		return "channel.mention"
	}
	return "channel.message"
}

func channelMessageDigest(message *channelruntime.InboundMessage) string {
	payload, _ := json.Marshal(struct {
		ProviderMessageID      string                          `json:"provider_message_id"`
		ExternalTenantID       string                          `json:"external_tenant_id"`
		ExternalConversationID string                          `json:"external_conversation_id"`
		ExternalThreadID       string                          `json:"external_thread_id"`
		ConversationType       channelruntime.ConversationType `json:"conversation_type"`
		SenderExternalID       string                          `json:"sender_external_id"`
		Text                   string                          `json:"text"`
		Mentions               []channelruntime.Mention        `json:"mentions"`
		OccurredAt             time.Time                       `json:"occurred_at"`
	}{
		ProviderMessageID: message.ProviderMessageID, ExternalTenantID: message.ExternalTenantID,
		ExternalConversationID: message.ExternalConversationID, ExternalThreadID: message.ExternalThreadID,
		ConversationType: message.ConversationType, SenderExternalID: message.SenderExternalID,
		Text: message.Text, Mentions: message.Mentions, OccurredAt: message.OccurredAt.UTC(),
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
