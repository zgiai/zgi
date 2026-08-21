package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

func TestChannelWebhookReplayedTenTimesCreatesOneInvocation(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeGroup)
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-1", ProviderMessageID: "message-1",
		ExternalTenantID: "tenant-1", ExternalConversationID: binding.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeGroup, SenderExternalID: "ou_sender", Text: "请总结",
		Mentions: []channelruntime.Mention{{ExternalID: installation.ExternalBotID}}, OccurredAt: time.Now().UTC(),
	}
	request := channelruntime.WebhookRequest{Body: []byte(`{"safe":"fixture"}`), ReceivedAt: time.Now().UTC()}
	for attempt := 0; attempt < 10; attempt++ {
		result, err := service.HandleWebhook(t.Context(), installation.ID, request)
		if err != nil {
			t.Fatalf("HandleWebhook(attempt=%d) error = %v", attempt, err)
		}
		if attempt == 0 && (!result.Accepted || result.Duplicate) {
			t.Fatalf("first result = %#v", result)
		}
		if attempt > 0 && !result.Duplicate {
			t.Fatalf("duplicate result(attempt=%d) = %#v", attempt, result)
		}
	}
	if runtime.calls != 1 {
		t.Fatalf("SubmitInvocation() calls = %d, want 1", runtime.calls)
	}
	if len(store.events) != 1 {
		t.Fatalf("inbound events = %d, want 1", len(store.events))
	}
	if len(service.transient.(*channelTransientDispatcherStub).requests) != 1 {
		t.Fatalf("transient acknowledgement requests = %d, want 1", len(service.transient.(*channelTransientDispatcherStub).requests))
	}
	if len(store.deliveries) != 0 {
		t.Fatalf("durable acknowledgement deliveries = %d, want 0", len(store.deliveries))
	}
}

func TestChannelWebhookSkipsAcknowledgementWhenDisabled(t *testing.T) {
	service, _, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	runtime.channelPolicy = &ChannelResponsePolicy{
		IntermediateMessagesEnabled: true,
		AcknowledgementEnabled:      false,
	}
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeDirect)
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-no-ack", ProviderMessageID: "message-no-ack",
		ExternalTenantID: "tenant-1", ExternalConversationID: binding.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeDirect, SenderExternalID: "ou_sender", Text: "请处理",
		OccurredAt: time.Now().UTC(),
	}
	result, err := service.HandleWebhook(t.Context(), installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)})
	if err != nil || !result.Accepted {
		t.Fatalf("HandleWebhook() = %#v, %v", result, err)
	}
	if requests := service.transient.(*channelTransientDispatcherStub).requests; len(requests) != 0 {
		t.Fatalf("transient acknowledgement requests = %#v, want none", requests)
	}
}

func TestChannelTransientDeliveryBypassesDurableOutbox(t *testing.T) {
	service, store, _, adapter, scope, digitalHumanID := newChannelTestService(t)
	runID := uuid.New()
	store.transientTarget = &ChannelTransientTarget{
		OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, ActorID: scope.ActorID,
		InstallationID: uuid.New(), ConnectionID: uuid.New(), SubjectType: runtimeidentity.SubjectTypeDigitalHuman,
		SubjectID: digitalHumanID, Provider: "feishu", ExternalConversationID: "oc_transient",
	}
	adapter.receipt = &channelruntime.DeliveryReceipt{ProviderMessageID: "om_transient"}
	request := ChannelTransientDeliveryRequest{
		RunID: runID, Kind: channelruntime.DeliveryKindProgress,
		DeliveryKey: "process:1", Sequence: 1, Text: "我先确认当前配置。",
	}
	if err := service.DeliverTransient(t.Context(), request); err != nil {
		t.Fatalf("DeliverTransient() error = %v", err)
	}
	if adapter.deliveryCalls != 1 || adapter.lastDelivery.Text != request.Text {
		t.Fatalf("adapter delivery = %#v", adapter.lastDelivery)
	}
	if len(store.deliveries) != 0 {
		t.Fatalf("durable deliveries = %#v, want none", store.deliveries)
	}
}

func TestChannelWebhookSameMessageWithNewEventIDCreatesOneInvocation(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeGroup)
	occurredAt := time.Now().UTC()
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-message-original", ProviderMessageID: "message-stable",
		ExternalTenantID: "tenant-1", ExternalConversationID: binding.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeGroup, SenderExternalID: "ou_sender", Text: "请总结",
		Mentions: []channelruntime.Mention{{ExternalID: installation.ExternalBotID}}, OccurredAt: occurredAt,
	}
	if _, err := service.HandleWebhook(t.Context(), installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"first"}`)}); err != nil {
		t.Fatalf("first HandleWebhook() error = %v", err)
	}
	adapter.message.ProviderEventID = "event-message-retry"
	result, err := service.HandleWebhook(t.Context(), installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"second"}`)})
	if err != nil {
		t.Fatalf("replayed HandleWebhook() error = %v", err)
	}
	if !result.Duplicate || !result.Accepted || runtime.calls != 1 || len(store.events) != 1 {
		t.Fatalf("result = %#v, SubmitInvocation() calls = %d, events = %d", result, runtime.calls, len(store.events))
	}
	if runtime.lastEventID != "event-message-original" {
		t.Fatalf("canonical invocation event ID = %q, want original persisted event ID", runtime.lastEventID)
	}
}

func TestChannelWebhookGroupMessageWithoutBotMentionIsIgnored(t *testing.T) {
	service, _, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeGroup)
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-no-mention", ProviderMessageID: "message-no-mention",
		ExternalTenantID: "tenant-1", ExternalConversationID: binding.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeGroup, SenderExternalID: "ou_sender", Text: "普通消息",
		OccurredAt: time.Now().UTC(),
	}
	result, err := service.HandleWebhook(t.Context(), installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"fixture"}`)})
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	if !result.Ignored || runtime.calls != 0 {
		t.Fatalf("result = %#v, SubmitInvocation() calls = %d", result, runtime.calls)
	}
}

func TestChannelDirectMessageAutoBindsDefaultSubjectAndUsesFirstMessage(t *testing.T) {
	service, _, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu direct default",
	})
	if err != nil {
		t.Fatal(err)
	}
	if installation.DefaultSubjectType == nil || *installation.DefaultSubjectType != runtimeidentity.SubjectTypeDigitalHuman ||
		installation.DefaultSubjectID == nil || *installation.DefaultSubjectID != digitalHumanID {
		t.Fatalf("installation default subject = (%v, %v), want (%q, %s)", installation.DefaultSubjectType, installation.DefaultSubjectID, runtimeidentity.SubjectTypeDigitalHuman, digitalHumanID)
	}
	conversationID := "oc_" + uuid.NewString()
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-first-direct", ProviderMessageID: "message-first-direct",
		ExternalTenantID: "tenant-1", ExternalConversationID: conversationID,
		ConversationType: channelruntime.ConversationTypeDirect, SenderExternalID: "ou_private_sender",
		Text: "hello", OccurredAt: time.Now().UTC(),
	}
	result, err := service.HandleWebhook(t.Context(), installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"first"}`)})
	if err != nil {
		t.Fatalf("HandleWebhook(first) error = %v", err)
	}
	if !result.Accepted || result.Ignored || runtime.calls != 1 || runtime.lastConversationID == nil {
		t.Fatalf("first direct result = %#v, runtime = %#v", result, runtime)
	}
	discoveries, err := service.ListDiscoveries(t.Context(), scope, digitalHumanID)
	if err != nil {
		t.Fatalf("ListDiscoveries() error = %v", err)
	}
	if len(discoveries) != 0 {
		t.Fatalf("discoveries = %#v, want no pending direct conversation", discoveries)
	}
	bindings, err := service.ListBindings(t.Context(), scope, digitalHumanID)
	if err != nil {
		t.Fatalf("ListBindings() error = %v", err)
	}
	if len(bindings) != 1 || bindings[0].Mapping.LocalConversationID == nil ||
		*bindings[0].Mapping.LocalConversationID != *runtime.lastConversationID {
		t.Fatalf("auto binding = %#v, runtime conversation = %v", bindings, runtime.lastConversationID)
	}
	firstConversationID := *runtime.lastConversationID
	adapter.message.ProviderEventID = "event-private-follow-up"
	adapter.message.ProviderMessageID = "message-private-follow-up"
	adapter.message.Text = "follow up"
	result, err = service.HandleWebhook(t.Context(), installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"follow-up"}`)})
	if err != nil {
		t.Fatalf("HandleWebhook(follow-up) error = %v", err)
	}
	if !result.Accepted || runtime.calls != 2 || runtime.lastConversationID == nil || *runtime.lastConversationID != firstConversationID {
		t.Fatalf("follow-up result = %#v, runtime = %#v", result, runtime)
	}
}

func TestChannelDirectMessageWithoutDefaultSubjectRemainsDiscoverable(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Legacy Feishu installation",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.installations[installation.ID].DefaultSubjectType = nil
	store.installations[installation.ID].DefaultSubjectID = nil
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-legacy-direct", ProviderMessageID: "message-legacy-direct",
		ExternalTenantID: "tenant-1", ExternalConversationID: "oc_" + uuid.NewString(),
		ConversationType: channelruntime.ConversationTypeDirect, SenderExternalID: "ou_private_sender",
		Text: "hello", OccurredAt: time.Now().UTC(),
	}

	result, err := service.HandleWebhook(t.Context(), installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"legacy"}`)})
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	if !result.Ignored || result.Accepted || runtime.calls != 0 {
		t.Fatalf("legacy result = %#v, runtime calls = %d", result, runtime.calls)
	}
	discoveries, err := service.ListDiscoveries(t.Context(), scope, digitalHumanID)
	if err != nil {
		t.Fatalf("ListDiscoveries() error = %v", err)
	}
	if len(discoveries) != 1 || discoveries[0].ConversationType != channelruntime.ConversationTypeDirect {
		t.Fatalf("discoveries = %#v, want one pending direct conversation", discoveries)
	}
}

func TestChannelGroupConversationAutoBindsOnFirstBotMention(t *testing.T) {
	service, _, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu group default",
	})
	if err != nil {
		t.Fatal(err)
	}
	conversationID := "oc_" + uuid.NewString()
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-group-ordinary", ProviderMessageID: "message-group-ordinary",
		ExternalTenantID: "tenant-1", ExternalConversationID: conversationID,
		ConversationType: channelruntime.ConversationTypeGroup, SenderExternalID: "ou_group_sender",
		Text: "ordinary group message", OccurredAt: time.Now().UTC(),
	}

	result, err := service.HandleWebhook(t.Context(), installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"ordinary"}`)})
	if err != nil {
		t.Fatalf("HandleWebhook(ordinary) error = %v", err)
	}
	if !result.Ignored || runtime.calls != 0 {
		t.Fatalf("ordinary group result = %#v, runtime calls = %d", result, runtime.calls)
	}
	bindings, err := service.ListBindings(t.Context(), scope, digitalHumanID)
	if err != nil {
		t.Fatalf("ListBindings(ordinary) error = %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("ordinary group bindings = %#v, want none", bindings)
	}

	adapter.message.ProviderEventID = "event-group-mention"
	adapter.message.ProviderMessageID = "message-group-mention"
	adapter.message.Text = "please help"
	adapter.message.Mentions = []channelruntime.Mention{{ExternalID: installation.ExternalBotID}}
	result, err = service.HandleWebhook(t.Context(), installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"mention"}`)})
	if err != nil {
		t.Fatalf("HandleWebhook(mention) error = %v", err)
	}
	if !result.Accepted || runtime.calls != 1 {
		t.Fatalf("group mention result = %#v, runtime calls = %d", result, runtime.calls)
	}
	bindings, err = service.ListBindings(t.Context(), scope, digitalHumanID)
	if err != nil {
		t.Fatalf("ListBindings(mention) error = %v", err)
	}
	if len(bindings) != 1 || bindings[0].Binding.WakePolicy != channelruntime.WakePolicyMentionOnly {
		t.Fatalf("group mention bindings = %#v, want one mention-only route", bindings)
	}
}

func TestChannelInstallationUsageBlocksDeleteUntilBindingIsRemoved(t *testing.T) {
	service, store, _, _, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeGroup)

	items, err := service.ListInstallations(t.Context(), scope, digitalHumanID)
	if err != nil {
		t.Fatalf("ListInstallations() error = %v", err)
	}
	var listed *ChannelInstallationView
	for _, item := range items {
		if item.ID == installation.ID {
			listed = item
			break
		}
	}
	if listed == nil || listed.BindingCount != 1 || len(listed.UsedBy) != 1 || listed.UsedBy[0].DigitalHumanID != digitalHumanID {
		t.Fatalf("installation usage = %#v", listed)
	}

	err = service.DeleteInstallation(t.Context(), scope, digitalHumanID, installation.ID, DeleteChannelInstallationRequest{})
	if !errors.Is(err, ErrChannelInstallationInUse) {
		t.Fatalf("DeleteInstallation() error = %v, want ErrChannelInstallationInUse", err)
	}
	if err := service.DeleteInstallation(t.Context(), scope, digitalHumanID, installation.ID, DeleteChannelInstallationRequest{RemoveBindings: true}); err != nil {
		t.Fatalf("DeleteInstallation(remove bindings) error = %v", err)
	}
	if _, exists := store.bindings[binding.Binding.ID]; exists {
		t.Fatalf("binding %s still exists after installation cleanup", binding.Binding.ID)
	}
}

func TestChannelWebhookReplayRepairsFailedInvocationSubmission(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeDirect)
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-recover", ProviderMessageID: "message-recover",
		ExternalTenantID: "tenant-1", ExternalConversationID: binding.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeDirect, SenderExternalID: "ou_sender", Text: "恢复执行",
		OccurredAt: time.Now().UTC(),
	}
	runtime.remainingFailures = 1
	request := channelruntime.WebhookRequest{Body: []byte(`{"safe":"fixture"}`), ReceivedAt: time.Now().UTC()}
	if _, err := service.HandleWebhook(t.Context(), installation.ID, request); err == nil {
		t.Fatal("first HandleWebhook() error = nil")
	}
	result, err := service.HandleWebhook(t.Context(), installation.ID, request)
	if err != nil {
		t.Fatalf("replayed HandleWebhook() error = %v", err)
	}
	if !result.Accepted || !result.Duplicate || result.RunID == nil {
		t.Fatalf("replayed result = %#v", result)
	}
	if runtime.calls != 2 || len(store.events) != 1 {
		t.Fatalf("SubmitInvocation() calls = %d, inbound events = %d", runtime.calls, len(store.events))
	}
}

func TestChannelWebhookRejectsReusedEventIDWithDifferentPayload(t *testing.T) {
	service, _, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeDirect)
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-collision", ProviderMessageID: "message-original",
		ExternalTenantID: "tenant-1", ExternalConversationID: binding.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeDirect, SenderExternalID: "ou_sender", Text: "原始内容",
		OccurredAt: time.Now().UTC(),
	}
	if _, err := service.HandleWebhook(t.Context(), installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"payload":"original"}`)}); err != nil {
		t.Fatalf("first HandleWebhook() error = %v", err)
	}
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-collision", ProviderMessageID: "message-forged",
		ExternalTenantID: "tenant-1", ExternalConversationID: binding.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeDirect, SenderExternalID: "ou_sender", Text: "替换内容",
		OccurredAt: time.Now().UTC(),
	}
	_, err := service.HandleWebhook(t.Context(), installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"payload":"changed"}`)})
	if !errors.Is(err, channelruntime.ErrInvalidWebhook) {
		t.Fatalf("reused event ID error = %v, want ErrInvalidWebhook", err)
	}
	if runtime.calls != 1 {
		t.Fatalf("SubmitInvocation() calls = %d, want 1", runtime.calls)
	}
}

func TestDeliveryRetryDoesNotSubmitInvocationAgain(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeDirect)
	payload, digest, err := channelDeliveryPayload("reply")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	delivery := &channelruntime.Delivery{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		InstallationID: installation.ID, BindingID: binding.Binding.ID, InboundEventID: uuid.New(), RunID: uuid.New(),
		Provider: "feishu", ConnectionID: installation.ConnectionID, ActorID: scope.ActorID,
		SubjectType: runtimeidentity.SubjectTypeDigitalHuman, SubjectID: digitalHumanID,
		ExternalConversationID: binding.Mapping.ExternalConversationID, Payload: payload, PayloadDigest: digest,
		Status: channelruntime.DeliveryPending, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	store.deliveries[delivery.ID] = delivery
	adapter.deliveryErr = errors.New("temporary provider failure with sensitive upstream detail")
	service.now = func() time.Time { return now }
	if err := service.DispatchDeliveries(t.Context(), 10); !errors.Is(err, errChannelDeliveryAttemptFailed) || strings.Contains(err.Error(), "sensitive upstream detail") {
		t.Fatalf("DispatchDeliveries() error = %v, want safe delivery failure", err)
	}
	adapter.deliveryErr = nil
	adapter.receipt = &channelruntime.DeliveryReceipt{ProviderMessageID: "om_1"}
	now = now.Add(time.Minute)
	if err := service.DispatchDeliveries(t.Context(), 10); err != nil {
		t.Fatalf("DispatchDeliveries(retry) error = %v", err)
	}
	if runtime.calls != 0 || adapter.deliveryCalls != 2 {
		t.Fatalf("SubmitInvocation() calls = %d, Deliver() calls = %d", runtime.calls, adapter.deliveryCalls)
	}
	stored := store.deliveries[delivery.ID]
	if stored.Status != channelruntime.DeliverySucceeded || stored.AttemptCount != 2 || stored.ProviderMessageID != "om_1" {
		t.Fatalf("delivery = %#v", stored)
	}
}

func TestDeliveryUnknownOutcomeIsNotAutomaticallyRetried(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeDirect)
	delivery := newChannelTestDelivery(t, scope, digitalHumanID, installation, binding, time.Now().UTC())
	store.deliveries[delivery.ID] = delivery
	adapter.deliveryErr = channelruntime.ErrDeliveryOutcomeUnknown
	service.now = func() time.Time { return delivery.AvailableAt }
	if err := service.DispatchDeliveries(t.Context(), 10); err == nil {
		t.Fatal("DispatchDeliveries() unknown outcome error = nil")
	}
	if stored := store.deliveries[delivery.ID]; stored.Status != channelruntime.DeliveryUnknown || stored.AttemptCount != 1 {
		t.Fatalf("delivery = %#v", stored)
	}
	adapter.deliveryErr = nil
	if err := service.DispatchDeliveries(t.Context(), 10); err != nil {
		t.Fatalf("second DispatchDeliveries() error = %v", err)
	}
	if runtime.calls != 0 || adapter.deliveryCalls != 1 {
		t.Fatalf("SubmitInvocation() calls = %d, Deliver() calls = %d", runtime.calls, adapter.deliveryCalls)
	}
}

func TestDeliveryMissingReceiptIsMarkedUnknown(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeDirect)
	delivery := newChannelTestDelivery(t, scope, digitalHumanID, installation, binding, time.Now().UTC())
	store.deliveries[delivery.ID] = delivery
	service.now = func() time.Time { return delivery.AvailableAt }
	if err := service.DispatchDeliveries(t.Context(), 10); err == nil {
		t.Fatal("DispatchDeliveries() missing receipt error = nil")
	}
	if stored := store.deliveries[delivery.ID]; stored.Status != channelruntime.DeliveryUnknown || stored.AttemptCount != 1 {
		t.Fatalf("delivery = %#v", stored)
	}
	if runtime.calls != 0 || adapter.deliveryCalls != 1 {
		t.Fatalf("SubmitInvocation() calls = %d, Deliver() calls = %d", runtime.calls, adapter.deliveryCalls)
	}
}

func TestDeliveryStaleProcessingIsMarkedUnknownWithoutResend(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeDirect)
	now := time.Now().UTC()
	delivery := newChannelTestDelivery(t, scope, digitalHumanID, installation, binding, now)
	delivery.Status = channelruntime.DeliveryProcessing
	delivery.AttemptCount = 1
	claimedAt := now.Add(-defaultDeliveryLease - time.Second)
	delivery.ClaimedAt = &claimedAt
	store.deliveries[delivery.ID] = delivery
	service.now = func() time.Time { return now }
	if err := service.ReconcileDeliveries(t.Context()); err != nil {
		t.Fatalf("ReconcileDeliveries() error = %v", err)
	}
	if stored := store.deliveries[delivery.ID]; stored.Status != channelruntime.DeliveryUnknown {
		t.Fatalf("delivery = %#v", stored)
	}
	if runtime.calls != 0 || adapter.deliveryCalls != 0 {
		t.Fatalf("SubmitInvocation() calls = %d, Deliver() calls = %d", runtime.calls, adapter.deliveryCalls)
	}
}

func newChannelTestDelivery(
	t *testing.T,
	scope Scope,
	digitalHumanID uuid.UUID,
	installation *channelruntime.Installation,
	binding *ChannelBindingView,
	now time.Time,
) *channelruntime.Delivery {
	t.Helper()
	payload, digest, err := channelDeliveryPayload("reply")
	if err != nil {
		t.Fatal(err)
	}
	return &channelruntime.Delivery{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		InstallationID: installation.ID, BindingID: binding.Binding.ID, InboundEventID: uuid.New(), RunID: uuid.New(),
		Provider: "feishu", ConnectionID: installation.ConnectionID, ActorID: scope.ActorID,
		SubjectType: runtimeidentity.SubjectTypeDigitalHuman, SubjectID: digitalHumanID,
		ExternalConversationID: binding.Mapping.ExternalConversationID, Payload: payload, PayloadDigest: digest,
		Status: channelruntime.DeliveryPending, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

func newChannelTestService(t *testing.T) (*ChannelService, *channelStoreStub, *channelRuntimeStub, *channelAdapterStub, Scope, uuid.UUID) {
	t.Helper()
	scope := testScope()
	digitalHumanID := uuid.New()
	store := newChannelStoreStub()
	runtime := &channelRuntimeStub{}
	adapter := &channelAdapterStub{}
	registry, err := channelruntime.NewRegistry(adapter)
	if err != nil {
		t.Fatal(err)
	}
	subjects := channelSubjectStub{definition: &Definition{
		ID: digitalHumanID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		Name: "Channel test", Status: StatusActive,
	}}
	service := NewChannelService(store, subjects, runtime, registry, channelCredentialStub{}, &channelTransientDispatcherStub{})
	return service, store, runtime, adapter, scope, digitalHumanID
}

func createChannelTestRoute(t *testing.T, service *ChannelService, scope Scope, digitalHumanID uuid.UUID, conversationType channelruntime.ConversationType) (*channelruntime.Installation, *ChannelBindingView) {
	t.Helper()
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if installation.ExternalTenantID != "tenant-1" || installation.ExternalBotID != "ou_bot" {
		t.Fatalf("resolved channel identity = (%q, %q)", installation.ExternalTenantID, installation.ExternalBotID)
	}
	binding, err := service.CreateBinding(t.Context(), scope, digitalHumanID, CreateChannelBindingRequest{
		InstallationID: installation.ID, ExternalConversationID: "oc_" + uuid.NewString(),
		ConversationType: conversationType, WakePolicy: channelruntime.WakePolicyDirectAndMention,
	})
	if err != nil {
		t.Fatal(err)
	}
	return installation, binding
}

type channelSubjectStub struct{ definition *Definition }

func (stub channelSubjectStub) Definition(_ context.Context, scope Scope, id uuid.UUID) (*Definition, error) {
	if stub.definition == nil || stub.definition.ID != id || stub.definition.OrganizationID != scope.OrganizationID || stub.definition.WorkspaceID != scope.WorkspaceID {
		return nil, ErrNotFound
	}
	return stub.definition, nil
}

type channelRuntimeStub struct {
	calls              int
	runID              uuid.UUID
	lastEventID        string
	lastConversationID *uuid.UUID
	remainingFailures  int
	channelPolicy      *ChannelResponsePolicy
}

type channelTransientDispatcherStub struct {
	requests []ChannelTransientDeliveryRequest
	err      error
}

func (dispatcher *channelTransientDispatcherStub) EnqueueTransient(_ context.Context, request ChannelTransientDeliveryRequest) error {
	dispatcher.requests = append(dispatcher.requests, request)
	return dispatcher.err
}

func (runtime *channelRuntimeStub) SubmitInvocation(_ context.Context, _ Scope, event invocation.Event, _ string, conversationID *uuid.UUID) (*Run, bool, error) {
	runtime.calls++
	runtime.lastEventID = event.ID
	runtime.lastConversationID = cloneUUIDPointer(conversationID)
	if runtime.remainingFailures > 0 {
		runtime.remainingFailures--
		return nil, false, errors.New("temporary invocation submission failure")
	}
	if runtime.runID == uuid.Nil {
		runtime.runID = uuid.New()
	}
	run := &Run{ID: runtime.runID}
	if runtime.channelPolicy != nil {
		encoded, _ := json.Marshal(struct {
			ChannelResponsePolicy *ChannelResponsePolicy `json:"channel_response_policy"`
		}{ChannelResponsePolicy: runtime.channelPolicy})
		run.ExecutionSnapshot = encoded
	}
	return run, true, nil
}

type channelCredentialStub struct{}

func (channelCredentialStub) ResolveIdentity(context.Context, *channelruntime.Installation) (string, string, error) {
	return "tenant-1", "ou_bot", nil
}
func (channelCredentialStub) ResolveSecurity(context.Context, *channelruntime.Installation) (channelruntime.SecurityMaterial, func(), error) {
	return channelruntime.SecurityMaterial{VerificationToken: "safe"}, func() {}, nil
}

type channelAdapterStub struct {
	message       *channelruntime.InboundMessage
	receipt       *channelruntime.DeliveryReceipt
	deliveryErr   error
	deliveryCalls int
	lastDelivery  channelruntime.DeliveryRequest
}

func (*channelAdapterStub) Provider() string { return "feishu" }
func (*channelAdapterStub) Verify(_ context.Context, request channelruntime.WebhookRequest, _ channelruntime.SecurityMaterial) (*channelruntime.VerifiedWebhook, error) {
	return &channelruntime.VerifiedWebhook{Body: append([]byte(nil), request.Body...)}, nil
}
func (adapter *channelAdapterStub) Normalize(context.Context, *channelruntime.VerifiedWebhook) (*channelruntime.InboundMessage, error) {
	return adapter.message, nil
}
func (adapter *channelAdapterStub) Deliver(_ context.Context, request channelruntime.DeliveryRequest) (*channelruntime.DeliveryReceipt, error) {
	adapter.deliveryCalls++
	adapter.lastDelivery = request
	return adapter.receipt, adapter.deliveryErr
}
func (*channelAdapterStub) Edit(context.Context, channelruntime.DeliveryRequest, string) (*channelruntime.DeliveryReceipt, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (*channelAdapterStub) Retract(context.Context, channelruntime.DeliveryRequest, string) error {
	return channelruntime.ErrUnsupportedEvent
}

type channelStoreStub struct {
	installations   map[uuid.UUID]*channelruntime.Installation
	mappings        map[uuid.UUID]*channelruntime.ConversationMapping
	bindings        map[uuid.UUID]*channelruntime.Binding
	events          map[string]*channelruntime.InboundEvent
	deliveries      map[uuid.UUID]*channelruntime.Delivery
	transientTarget *ChannelTransientTarget
}

func newChannelStoreStub() *channelStoreStub {
	return &channelStoreStub{
		installations: make(map[uuid.UUID]*channelruntime.Installation), mappings: make(map[uuid.UUID]*channelruntime.ConversationMapping),
		bindings: make(map[uuid.UUID]*channelruntime.Binding), events: make(map[string]*channelruntime.InboundEvent),
		deliveries: make(map[uuid.UUID]*channelruntime.Delivery),
	}
}

func (store *channelStoreStub) CreateChannelInstallation(_ context.Context, installation *channelruntime.Installation) error {
	store.installations[installation.ID] = cloneInstallation(installation)
	return nil
}
func (store *channelStoreStub) ChannelInstallationByID(_ context.Context, id uuid.UUID) (*channelruntime.Installation, error) {
	installation := store.installations[id]
	if installation == nil {
		return nil, ErrChannelInstallationNotFound
	}
	return cloneInstallation(installation), nil
}
func (store *channelStoreStub) ScopedChannelInstallationByID(ctx context.Context, scope Scope, id uuid.UUID) (*channelruntime.Installation, error) {
	installation, err := store.ChannelInstallationByID(ctx, id)
	if err != nil || installation.OrganizationID != scope.OrganizationID || installation.WorkspaceID != scope.WorkspaceID {
		return nil, ErrChannelInstallationNotFound
	}
	return installation, nil
}
func (store *channelStoreStub) ListChannelInstallations(context.Context, Scope) ([]*ChannelInstallationView, error) {
	items := make([]*ChannelInstallationView, 0, len(store.installations))
	for _, installation := range store.installations {
		item := &ChannelInstallationView{
			Installation: cloneInstallation(installation),
			UsedBy:       make([]*ChannelInstallationUsage, 0),
		}
		countsBySubject := make(map[uuid.UUID]int64)
		for _, binding := range store.bindings {
			if binding.InstallationID == installation.ID {
				countsBySubject[binding.SubjectID]++
			}
		}
		for subjectID, count := range countsBySubject {
			item.BindingCount += count
			item.UsedBy = append(item.UsedBy, &ChannelInstallationUsage{
				DigitalHumanID: subjectID, BindingCount: count,
			})
		}
		items = append(items, item)
	}
	return items, nil
}
func (store *channelStoreStub) UpdateChannelInstallation(_ context.Context, _ Scope, installation *channelruntime.Installation, expected int64) error {
	current := store.installations[installation.ID]
	if current == nil || current.Revision != expected {
		return ErrChannelConflict
	}
	store.installations[installation.ID] = cloneInstallation(installation)
	return nil
}
func (store *channelStoreStub) DeleteChannelInstallation(_ context.Context, _ Scope, id uuid.UUID, request DeleteChannelInstallationRequest) error {
	for _, binding := range store.bindings {
		if binding.InstallationID == id && !request.RemoveBindings {
			return ErrChannelInstallationInUse
		}
	}
	for bindingID, binding := range store.bindings {
		if binding.InstallationID == id {
			delete(store.bindings, bindingID)
		}
	}
	delete(store.installations, id)
	return nil
}
func (store *channelStoreStub) CreateChannelBinding(_ context.Context, mapping *channelruntime.ConversationMapping, binding *channelruntime.Binding) error {
	store.mappings[mapping.ID], store.bindings[binding.ID] = mapping, binding
	return nil
}
func (store *channelStoreStub) ListChannelBindings(_ context.Context, _ Scope, subjectID uuid.UUID) ([]*ChannelBindingView, error) {
	items := make([]*ChannelBindingView, 0)
	for _, binding := range store.bindings {
		if binding.SubjectID == subjectID {
			items = append(items, &ChannelBindingView{Binding: binding, Mapping: store.mappings[binding.ConversationMappingID]})
		}
	}
	return items, nil
}
func (store *channelStoreStub) ListDiscoveredChannelConversations(_ context.Context, _ Scope) ([]*ChannelConversationDiscovery, error) {
	items := make([]*ChannelConversationDiscovery, 0)
	for _, event := range store.events {
		bound := false
		for _, mapping := range store.mappings {
			if mapping.InstallationID == event.InstallationID && mapping.ExternalConversationID == event.ExternalConversationID {
				for _, binding := range store.bindings {
					if binding.ConversationMappingID == mapping.ID {
						bound = true
						break
					}
				}
			}
		}
		if bound {
			continue
		}
		conversationType := channelruntime.ConversationTypeDirect
		if event.EventType == "channel.mention" {
			conversationType = channelruntime.ConversationTypeGroup
		}
		items = append(items, &ChannelConversationDiscovery{
			InstallationID: event.InstallationID, ExternalConversationID: event.ExternalConversationID,
			ConversationType: conversationType, SenderExternalID: event.SenderExternalID,
			EventCount: 1, LastEventAt: event.ReceivedAt,
		})
	}
	return items, nil
}
func (store *channelStoreStub) DeleteChannelBinding(_ context.Context, _ Scope, subjectID, bindingID uuid.UUID) error {
	if binding := store.bindings[bindingID]; binding == nil || binding.SubjectID != subjectID {
		return ErrChannelBindingNotFound
	}
	delete(store.bindings, bindingID)
	return nil
}
func (store *channelStoreStub) ResolveChannelRoute(_ context.Context, installationID uuid.UUID, conversationID string) (*ChannelRoute, error) {
	installation := store.installations[installationID]
	if installation == nil {
		return nil, ErrChannelInstallationNotFound
	}
	for _, mapping := range store.mappings {
		if mapping.InstallationID != installationID || mapping.ExternalConversationID != conversationID {
			continue
		}
		for _, binding := range store.bindings {
			if binding.ConversationMappingID == mapping.ID && binding.Enabled {
				return &ChannelRoute{Installation: installation, Mapping: mapping, Binding: binding}, nil
			}
		}
	}
	return &ChannelRoute{Installation: installation}, ErrChannelBindingNotFound
}
func (store *channelStoreStub) EnsureChannelConversationID(_ context.Context, mappingID, suggestedID uuid.UUID, eventAt time.Time) (uuid.UUID, error) {
	mapping := store.mappings[mappingID]
	if mapping == nil {
		return uuid.Nil, ErrChannelBindingNotFound
	}
	if mapping.LocalConversationID == nil {
		mapping.LocalConversationID = cloneUUIDPointer(&suggestedID)
	}
	if mapping.LastEventAt == nil || mapping.LastEventAt.Before(eventAt) {
		value := eventAt
		mapping.LastEventAt = &value
	}
	return *mapping.LocalConversationID, nil
}
func (store *channelStoreStub) RecordInboundEvent(_ context.Context, event *channelruntime.InboundEvent) (*channelruntime.InboundEvent, bool, error) {
	key := event.InstallationID.String() + ":" + event.ProviderEventID
	if existing := store.events[key]; existing != nil {
		return existing, false, nil
	}
	for _, existing := range store.events {
		if existing.InstallationID == event.InstallationID && existing.ProviderMessageID != "" && existing.ProviderMessageID == event.ProviderMessageID {
			return existing, false, nil
		}
	}
	store.events[key] = event
	return event, true, nil
}
func (store *channelStoreStub) MarkInboundEvent(_ context.Context, eventID uuid.UUID, status channelruntime.InboundEventStatus, reason string, runID *uuid.UUID, now time.Time) error {
	for _, event := range store.events {
		if event.ID != eventID {
			continue
		}
		event.Status, event.ReasonCode, event.ProcessedAt = status, reason, &now
		if runID != nil {
			id := *runID
			event.InvocationRunID = &id
		}
		return nil
	}
	return ErrChannelBindingNotFound
}
func (store *channelStoreStub) EnqueueRunChannelDelivery(_ context.Context, runID uuid.UUID, kind channelruntime.DeliveryKind, deliveryKey string, sequence int64, text string, now time.Time) error {
	for _, event := range store.events {
		if event.InvocationRunID == nil || *event.InvocationRunID != runID || event.BindingID == nil {
			continue
		}
		binding := store.bindings[*event.BindingID]
		installation := store.installations[event.InstallationID]
		if binding == nil || installation == nil {
			return ErrChannelBindingNotFound
		}
		payload, digest, err := channelDeliveryPayloadWithKind(kind, text)
		if err != nil {
			return err
		}
		id := channelDeliveryStageID(runID, kind, deliveryKey)
		if _, exists := store.deliveries[id]; exists {
			return nil
		}
		store.deliveries[id] = &channelruntime.Delivery{
			ID: id, OrganizationID: event.OrganizationID, WorkspaceID: event.WorkspaceID,
			InstallationID: installation.ID, BindingID: binding.ID, InboundEventID: event.ID, RunID: runID,
			Kind: kind, DeliveryKey: deliveryKey, Sequence: sequence,
			Provider: installation.Provider, ConnectionID: installation.ConnectionID, ActorID: installation.CreatedBy,
			SubjectType: binding.SubjectType, SubjectID: binding.SubjectID,
			ExternalConversationID: event.ExternalConversationID, Payload: payload, PayloadDigest: digest,
			Status: channelruntime.DeliveryPending, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
		}
		return nil
	}
	return nil
}
func (store *channelStoreStub) RunChannelTransientTarget(context.Context, uuid.UUID) (*ChannelTransientTarget, error) {
	return store.transientTarget, nil
}
func (*channelStoreStub) ListInboundEvents(context.Context, Scope, uuid.UUID, ListQuery) ([]*channelruntime.InboundEvent, int64, error) {
	return nil, 0, nil
}
func (*channelStoreStub) ListChannelDeliveries(context.Context, Scope, uuid.UUID, ListQuery) ([]*channelruntime.Delivery, int64, error) {
	return nil, 0, nil
}
func (store *channelStoreStub) ClaimDeliveryBatch(_ context.Context, limit int, now time.Time) ([]*channelruntime.Delivery, error) {
	items := make([]*channelruntime.Delivery, 0, limit)
	for _, delivery := range store.deliveries {
		if len(items) >= limit || delivery.Status != channelruntime.DeliveryPending || delivery.AvailableAt.After(now) {
			continue
		}
		delivery.Status, delivery.AttemptCount, delivery.ClaimedAt = channelruntime.DeliveryProcessing, delivery.AttemptCount+1, &now
		items = append(items, delivery)
	}
	return items, nil
}
func (store *channelStoreStub) SucceedDelivery(_ context.Context, id uuid.UUID, receipt *channelruntime.DeliveryReceipt, now time.Time) error {
	delivery := store.deliveries[id]
	if delivery == nil || delivery.Status != channelruntime.DeliveryProcessing {
		return ErrChannelDeliveryConflict
	}
	delivery.Status, delivery.ProviderMessageID, delivery.ProviderRequestID = channelruntime.DeliverySucceeded, receipt.ProviderMessageID, receipt.ProviderRequestID
	delivery.DeliveredAt, delivery.ClaimedAt = &now, nil
	return nil
}
func (store *channelStoreStub) RetryDelivery(_ context.Context, id uuid.UUID, availableAt time.Time, code, message string) error {
	delivery := store.deliveries[id]
	if delivery == nil || delivery.Status != channelruntime.DeliveryProcessing {
		return ErrChannelDeliveryConflict
	}
	delivery.Status, delivery.AvailableAt, delivery.ClaimedAt = channelruntime.DeliveryPending, availableAt, nil
	delivery.ErrorCode, delivery.ErrorMessage = code, message
	return nil
}
func (store *channelStoreStub) FailDelivery(_ context.Context, id uuid.UUID, status channelruntime.DeliveryStatus, code, message string, _ time.Time) error {
	delivery := store.deliveries[id]
	if delivery == nil || delivery.Status != channelruntime.DeliveryProcessing {
		return ErrChannelDeliveryConflict
	}
	delivery.Status, delivery.ErrorCode, delivery.ErrorMessage, delivery.ClaimedAt = status, code, message, nil
	return nil
}
func (store *channelStoreStub) RequeueDelivery(_ context.Context, _ Scope, subjectID, id uuid.UUID, now time.Time) error {
	delivery := store.deliveries[id]
	if delivery == nil || delivery.SubjectID != subjectID || delivery.Status != channelruntime.DeliveryFailed {
		return ErrChannelDeliveryConflict
	}
	delivery.Status, delivery.AvailableAt = channelruntime.DeliveryPending, now
	return nil
}
func (store *channelStoreStub) MarkStaleDeliveriesUnknown(_ context.Context, claimedBefore, _ time.Time) error {
	for _, delivery := range store.deliveries {
		if delivery.Status == channelruntime.DeliveryProcessing && delivery.ClaimedAt != nil && delivery.ClaimedAt.Before(claimedBefore) {
			delivery.Status, delivery.ClaimedAt = channelruntime.DeliveryUnknown, nil
		}
	}
	return nil
}

func cloneInstallation(value *channelruntime.Installation) *channelruntime.Installation {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.DefaultSubjectType != nil {
		defaultSubjectType := *value.DefaultSubjectType
		cloned.DefaultSubjectType = &defaultSubjectType
	}
	cloned.DefaultSubjectID = cloneUUIDPointer(value.DefaultSubjectID)
	return &cloned
}
