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
	"github.com/zgiai/zgi/api/internal/modules/integrations"
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
		result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, request)
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
	if runtime.lastEvent.ReplyTo == nil || runtime.lastEvent.ReplyTo.RecipientID != "ou_sender" {
		t.Fatalf("group reply target = %#v, want triggering sender", runtime.lastEvent.ReplyTo)
	}
	if len(store.events) != 1 {
		t.Fatalf("inbound events = %d, want 1", len(store.events))
	}
	for _, event := range store.events {
		if event.ContentPreview != "请总结" || event.ConversationType != channelruntime.ConversationTypeGroup || event.MentionOnly {
			t.Fatalf("inbound event display fields = %#v", event)
		}
	}
	if len(service.transient.(*channelTransientDispatcherStub).requests) != 1 {
		t.Fatalf("transient acknowledgement requests = %d, want 1", len(service.transient.(*channelTransientDispatcherStub).requests))
	}
	if len(store.deliveries) != 0 {
		t.Fatalf("durable acknowledgement deliveries = %d, want 0", len(store.deliveries))
	}
}

func TestChannelWebhookWhileConversationRunsBecomesRuntimeInput(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	runtime.runtimeInputAccepted = true
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeGroup)
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-supplement", ProviderMessageID: "message-supplement",
		ExternalTenantID: "tenant-1", ExternalConversationID: binding.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeGroup, SenderExternalID: "ou_sender", SenderDisplayName: "康源",
		Text: "再补充一个要求", Mentions: []channelruntime.Mention{{ExternalID: installation.ExternalBotID}}, OccurredAt: time.Now().UTC(),
	}
	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)})
	if err != nil || result == nil || !result.Accepted || result.RunID != nil {
		t.Fatalf("HandleWebhook() = %#v, %v", result, err)
	}
	if runtime.calls != 0 || runtime.lastRuntimeInput != "Channel message from 康源:\n再补充一个要求" {
		t.Fatalf("invocation calls = %d, runtime input = %q", runtime.calls, runtime.lastRuntimeInput)
	}
	if runtime.lastRuntimeRequest.ConversationType != channelruntime.ConversationTypeGroup || runtime.lastRuntimeRequest.SenderExternalID != "ou_sender" {
		t.Fatalf("runtime input routing attributes = %#v", runtime.lastRuntimeRequest)
	}
	if replay, replayErr := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)}); replayErr != nil || !replay.Duplicate || !replay.Accepted {
		t.Fatalf("replayed runtime input = %#v, %v", replay, replayErr)
	}
	if runtime.runtimeInputCalls != 1 {
		t.Fatalf("runtime input calls = %d, want 1", runtime.runtimeInputCalls)
	}
	event := store.events[installation.ID.String()+":"+adapter.message.ProviderEventID]
	if event == nil || event.Status != channelruntime.InboundEventAccepted || event.InvocationRunID != nil {
		t.Fatalf("supplemental inbound event = %#v", event)
	}
}

func TestChannelDeliveryMentionsOnlyFinalReplyRecipient(t *testing.T) {
	event := invocation.Event{
		ReplyTo: &invocation.ReplyAddress{RecipientID: " user-1 "},
		Context: map[string]any{"sender_display_name": " 康源 "},
	}
	mentions := channelDeliveryMentions(event, channelruntime.DeliveryKindFinal)
	if len(mentions) != 1 || mentions[0].ExternalID != "user-1" || mentions[0].DisplayName != "康源" {
		t.Fatalf("final mentions = %#v, want attributed triggering sender", mentions)
	}
	if progress := channelDeliveryMentions(event, channelruntime.DeliveryKindProgress); len(progress) != 0 {
		t.Fatalf("progress mentions = %#v, want none", progress)
	}
}

func TestChannelMessagePreviewIsBoundedAndWhitespaceNormalized(t *testing.T) {
	message := &channelruntime.InboundMessage{Text: "  第一行\n\t第二行  " + strings.Repeat("长", maxChannelPreviewRunes)}
	preview := channelMessagePreview(message)
	if !strings.HasPrefix(preview, "第一行 第二行 ") {
		t.Fatalf("preview = %q, want normalized whitespace", preview)
	}
	if got := len([]rune(preview)); got != maxChannelPreviewRunes {
		t.Fatalf("preview runes = %d, want %d", got, maxChannelPreviewRunes)
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
	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)})
	if err != nil || !result.Accepted {
		t.Fatalf("HandleWebhook() = %#v, %v", result, err)
	}
	if requests := service.transient.(*channelTransientDispatcherStub).requests; len(requests) != 0 {
		t.Fatalf("transient acknowledgement requests = %#v, want none", requests)
	}
	if runtime.lastEvent.ReplyTo == nil || runtime.lastEvent.ReplyTo.RecipientID != "" {
		t.Fatalf("direct reply target = %#v, want no redundant mention", runtime.lastEvent.ReplyTo)
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
	if _, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"first"}`)}); err != nil {
		t.Fatalf("first HandleWebhook() error = %v", err)
	}
	adapter.message.ProviderEventID = "event-message-retry"
	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"second"}`)})
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
	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"fixture"}`)})
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	if !result.Ignored || runtime.calls != 0 {
		t.Fatalf("result = %#v, SubmitInvocation() calls = %d", result, runtime.calls)
	}
}

func TestChannelWebhookStaleGroupMentionIsAuditedWithoutInvocation(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	receivedAt := time.Now().UTC().Truncate(time.Second)
	service.now = func() time.Time { return receivedAt }
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeGroup)
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-stale-mention", ProviderMessageID: "message-stale-mention",
		ExternalTenantID: "tenant-1", ExternalConversationID: binding.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeGroup, SenderExternalID: "ou_sender", Text: "请总结",
		Mentions:   []channelruntime.Mention{{ExternalID: installation.ExternalBotID}},
		OccurredAt: receivedAt.Add(-maxGroupMentionCallbackAge - time.Second),
	}

	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{
		Body: []byte(`{"safe":"stale"}`), ReceivedAt: receivedAt,
	})
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	if !result.Ignored || result.Accepted || runtime.calls != 0 {
		t.Fatalf("stale result = %#v, SubmitInvocation() calls = %d, want ignored without invocation", result, runtime.calls)
	}
	event := store.events[installation.ID.String()+":"+adapter.message.ProviderEventID]
	if event == nil || event.Status != channelruntime.InboundEventIgnored || event.ReasonCode != "channel.callback.stale" {
		t.Fatalf("stale inbound event = %#v, want ignored channel.callback.stale", event)
	}
	if requests := service.transient.(*channelTransientDispatcherStub).requests; len(requests) != 0 {
		t.Fatalf("stale acknowledgement requests = %#v, want none", requests)
	}
}

func TestChannelWebhookStaleGroupMentionDoesNotCreateAutomaticRoute(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	receivedAt := time.Now().UTC().Truncate(time.Second)
	service.now = func() time.Time { return receivedAt }
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu stale group",
	})
	if err != nil {
		t.Fatal(err)
	}
	verification, err := service.VerifyInstallation(t.Context(), scope, digitalHumanID, installation.ID)
	if err != nil || verification.Status != "verified" {
		t.Fatalf("VerifyInstallation() = %#v, %v", verification, err)
	}
	installation, err = store.ScopedChannelInstallationByID(t.Context(), scope, digitalHumanID, installation.ID)
	if err != nil {
		t.Fatal(err)
	}
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-stale-unbound", ProviderMessageID: "message-stale-unbound",
		ExternalTenantID: "tenant-1", ExternalConversationID: "oc_" + uuid.NewString(),
		ConversationType: channelruntime.ConversationTypeGroup, SenderExternalID: "ou_sender", Text: "请加入群聊",
		Mentions:   []channelruntime.Mention{{ExternalID: installation.ExternalBotID}},
		OccurredAt: receivedAt.Add(-maxGroupMentionCallbackAge - time.Second),
	}

	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{
		Body: []byte(`{"safe":"stale-unbound"}`), ReceivedAt: receivedAt,
	})
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	if !result.Ignored || runtime.calls != 0 {
		t.Fatalf("stale unbound result = %#v, SubmitInvocation() calls = %d", result, runtime.calls)
	}
	if len(store.mappings) != 0 || len(store.bindings) != 0 {
		t.Fatalf("stale automatic routes = mappings %d, bindings %d; want none", len(store.mappings), len(store.bindings))
	}
	event := store.events[installation.ID.String()+":"+adapter.message.ProviderEventID]
	if event == nil || event.BindingID != nil || event.ReasonCode != "channel.callback.stale" {
		t.Fatalf("stale unbound event = %#v, want unbound channel.callback.stale audit", event)
	}
}

func TestStaleGroupMentionCallbackBoundary(t *testing.T) {
	receivedAt := time.Now().UTC().Truncate(time.Second)
	base := channelruntime.InboundMessage{
		ConversationType: channelruntime.ConversationTypeGroup,
		WakeReason:       channelruntime.WakeReasonMention,
	}
	tests := []struct {
		name    string
		message channelruntime.InboundMessage
		want    bool
	}{
		{name: "at cutoff", message: base, want: false},
		{name: "past cutoff", message: base, want: true},
		{name: "direct message", message: base, want: false},
		{name: "group without bot mention", message: base, want: false},
		{name: "future provider timestamp", message: base, want: false},
	}
	tests[0].message.OccurredAt = receivedAt.Add(-maxGroupMentionCallbackAge)
	tests[1].message.OccurredAt = receivedAt.Add(-maxGroupMentionCallbackAge - time.Nanosecond)
	tests[2].message.ConversationType = channelruntime.ConversationTypeDirect
	tests[2].message.OccurredAt = receivedAt.Add(-time.Hour)
	tests[3].message.WakeReason = channelruntime.WakeReasonNone
	tests[3].message.OccurredAt = receivedAt.Add(-time.Hour)
	tests[4].message.OccurredAt = receivedAt.Add(time.Minute)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := staleGroupMentionCallback(&test.message, receivedAt); got != test.want {
				t.Fatalf("staleGroupMentionCallback() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestChannelWebhookMentionOnlyCreatesContextDrivenInvocation(t *testing.T) {
	service, _, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeGroup)
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-mention-only", ProviderMessageID: "message-mention-only",
		ExternalTenantID: "tenant-1", ExternalConversationID: binding.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeGroup, SenderExternalID: "ou_invoker",
		Mentions:    []channelruntime.Mention{{ExternalID: installation.ExternalBotID, Name: "丁康源的智能助手"}},
		MentionOnly: true, OccurredAt: time.Now().UTC(),
	}

	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":true}`)})
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	if !result.Accepted || result.Ignored || runtime.calls != 1 {
		t.Fatalf("result = %#v, SubmitInvocation() calls = %d", result, runtime.calls)
	}
	if runtime.lastQuery != "@丁康源的智能助手" {
		t.Fatalf("invocation query = %q, want the provider-authored wake representation", runtime.lastQuery)
	}
	if mentionOnly, _ := runtime.lastEvent.Context["mention_only"].(bool); !mentionOnly {
		t.Fatalf("invocation context = %#v, want mention_only=true", runtime.lastEvent.Context)
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
	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"first"}`)})
	if err != nil {
		t.Fatalf("HandleWebhook(first) error = %v", err)
	}
	if !result.Accepted || result.Ignored || runtime.calls != 1 || runtime.lastConversationID == nil {
		t.Fatalf("first direct result = %#v, runtime = %#v", result, runtime)
	}
	route, err := service.store.ResolveChannelRoute(t.Context(), installation.ID, conversationID)
	if err != nil || route.Mapping.LocalConversationID == nil ||
		*route.Mapping.LocalConversationID != *runtime.lastConversationID {
		t.Fatalf("automatic route = %#v, %v; runtime conversation = %v", route, err, runtime.lastConversationID)
	}
	firstConversationID := *runtime.lastConversationID
	adapter.message.ProviderEventID = "event-private-follow-up"
	adapter.message.ProviderMessageID = "message-private-follow-up"
	adapter.message.Text = "follow up"
	result, err = service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"follow-up"}`)})
	if err != nil {
		t.Fatalf("HandleWebhook(follow-up) error = %v", err)
	}
	if !result.Accepted || runtime.calls != 2 || runtime.lastConversationID == nil || *runtime.lastConversationID != firstConversationID {
		t.Fatalf("follow-up result = %#v, runtime = %#v", result, runtime)
	}
}

func TestChannelDirectMessageWithoutDefaultSubjectIsIgnored(t *testing.T) {
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

	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"legacy"}`)})
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	if !result.Ignored || result.Accepted || runtime.calls != 0 {
		t.Fatalf("legacy result = %#v, runtime calls = %d", result, runtime.calls)
	}
	if len(store.bindings) != 0 {
		t.Fatalf("automatic routes = %#v, want none", store.bindings)
	}
}

func TestChannelGroupConversationAutoBindsOnFirstBotMention(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu group default",
	})
	if err != nil {
		t.Fatal(err)
	}
	verification, err := service.VerifyInstallation(t.Context(), scope, digitalHumanID, installation.ID)
	if err != nil || verification.Status != "verified" {
		t.Fatalf("VerifyInstallation() = %#v, %v", verification, err)
	}
	installation, err = store.ScopedChannelInstallationByID(t.Context(), scope, digitalHumanID, installation.ID)
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

	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"ordinary"}`)})
	if err != nil {
		t.Fatalf("HandleWebhook(ordinary) error = %v", err)
	}
	if !result.Ignored || runtime.calls != 0 {
		t.Fatalf("ordinary group result = %#v, runtime calls = %d", result, runtime.calls)
	}
	if len(store.bindings) != 0 {
		t.Fatalf("ordinary group routes = %#v, want none", store.bindings)
	}

	adapter.message.ProviderEventID = "event-group-mention"
	adapter.message.ProviderMessageID = "message-group-mention"
	adapter.message.Text = "please help"
	adapter.message.Mentions = []channelruntime.Mention{{ExternalID: installation.ExternalBotID}}
	result, err = service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"safe":"mention"}`)})
	if err != nil {
		t.Fatalf("HandleWebhook(mention) error = %v", err)
	}
	if !result.Accepted || runtime.calls != 1 {
		t.Fatalf("group mention result = %#v, runtime calls = %d", result, runtime.calls)
	}
	route, err := service.store.ResolveChannelRoute(t.Context(), installation.ID, conversationID)
	if err != nil || route.Binding.WakePolicy != channelruntime.WakePolicyMentionOnly {
		t.Fatalf("group mention route = %#v, %v; want one mention-only route", route, err)
	}
}

func TestChannelInstallationDeleteRemovesAutomaticRoutes(t *testing.T) {
	service, store, _, _, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeGroup)

	if err := service.DeleteInstallation(t.Context(), scope, digitalHumanID, installation.ID); err != nil {
		t.Fatalf("DeleteInstallation() error = %v", err)
	}
	if _, exists := store.bindings[binding.Binding.ID]; exists {
		t.Fatalf("binding %s still exists after installation cleanup", binding.Binding.ID)
	}
}

func TestChannelInstallationViewUsesConfiguredAPIBaseURL(t *testing.T) {
	service, _, _, _, scope, digitalHumanID := newChannelTestService(t)
	service.WithAPIBaseURL("https://api.zgi.example.com/")
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu public webhook",
	})
	if err != nil {
		t.Fatal(err)
	}

	views, err := service.ListInstallations(t.Context(), scope, digitalHumanID)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://api.zgi.example.com/console/api/webhooks/channels/feishu/" + installation.ID.String()
	if len(views) != 1 || views[0].WebhookURL != want {
		t.Fatalf("installation views = %#v, want webhook URL %q", views, want)
	}
}

func TestChannelInstallationsAreIsolatedByDigitalHuman(t *testing.T) {
	service, store, _, _, scope, digitalHumanID := newChannelTestService(t)
	ownInstallation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Owned Feishu installation",
	})
	if err != nil {
		t.Fatal(err)
	}

	otherDigitalHumanID := uuid.New()
	defaultSubjectType := runtimeidentity.SubjectTypeDigitalHuman
	now := time.Now().UTC()
	foreignInstallation := &channelruntime.Installation{
		ID:                 uuid.New(),
		OrganizationID:     scope.OrganizationID,
		WorkspaceID:        scope.WorkspaceID,
		Provider:           "feishu",
		ConnectionID:       uuid.New(),
		Name:               "Another digital human's installation",
		ExternalTenantID:   "tenant-2",
		ExternalBotID:      "ou_other_bot",
		DefaultSubjectType: &defaultSubjectType,
		DefaultSubjectID:   &otherDigitalHumanID,
		Status:             channelruntime.InstallationStatusActive,
		Revision:           1,
		CreatedBy:          scope.ActorID,
		UpdatedBy:          scope.ActorID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	store.installations[foreignInstallation.ID] = cloneInstallation(foreignInstallation)

	views, err := service.ListInstallations(t.Context(), scope, digitalHumanID)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].ID != ownInstallation.ID {
		t.Fatalf("ListInstallations() = %#v, want only %s", views, ownInstallation.ID)
	}

	updatedName := "Cross-subject update"
	_, err = service.UpdateInstallation(t.Context(), scope, digitalHumanID, foreignInstallation.ID, UpdateChannelInstallationRequest{
		ExpectedRevision: foreignInstallation.Revision,
		Name:             &updatedName,
	})
	if !errors.Is(err, ErrChannelInstallationNotFound) {
		t.Fatalf("UpdateInstallation(foreign) error = %v, want ErrChannelInstallationNotFound", err)
	}
	_, err = service.UpdateInstallationCredential(t.Context(), scope, digitalHumanID, foreignInstallation.ID, UpdateChannelInstallationCredentialRequest{
		ExpectedRevision: foreignInstallation.Revision,
		Credentials:      map[string]string{"app_secret": "replacement"},
	})
	if !errors.Is(err, ErrChannelInstallationNotFound) {
		t.Fatalf("UpdateInstallationCredential(foreign) error = %v, want ErrChannelInstallationNotFound", err)
	}
	if _, err := service.VerifyInstallation(t.Context(), scope, digitalHumanID, foreignInstallation.ID); !errors.Is(err, ErrChannelInstallationNotFound) {
		t.Fatalf("VerifyInstallation(foreign) error = %v, want ErrChannelInstallationNotFound", err)
	}
	if err := service.DeleteInstallation(t.Context(), scope, digitalHumanID, foreignInstallation.ID); !errors.Is(err, ErrChannelInstallationNotFound) {
		t.Fatalf("DeleteInstallation(foreign) error = %v, want ErrChannelInstallationNotFound", err)
	}
	if store.installations[foreignInstallation.ID] == nil {
		t.Fatal("foreign installation was deleted")
	}
}

func TestChannelInstallationVerificationRechecksCredentialAndBotIdentity(t *testing.T) {
	service, store, _, _, scope, digitalHumanID := newChannelTestService(t)
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu verification",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.VerifyInstallation(t.Context(), scope, digitalHumanID, installation.ID)
	if err != nil {
		t.Fatalf("VerifyInstallation() error = %v", err)
	}
	if result.Status != "verified" || !result.CredentialVerified || !result.BotIdentityVerified {
		t.Fatalf("VerifyInstallation() = %#v", result)
	}
	if result.ChannelContext.Status != ChannelContextReadinessReady || result.CheckedAt.IsZero() {
		t.Fatalf("VerifyInstallation() readiness = %#v, checked_at = %v", result.ChannelContext, result.CheckedAt)
	}
	stored := store.installations[installation.ID]
	if stored.ExternalTenantID != "tenant-1" || stored.ExternalBotID != "ou_bot" || stored.Revision != 2 {
		t.Fatalf("stored verified identity = %#v", stored)
	}
}

func TestChannelInstallationVerificationReturnsActionableProviderFailure(t *testing.T) {
	service, store, _, _, scope, digitalHumanID := newChannelTestService(t)
	service.credentials = failingChannelCredentialStub{
		channelCredentialStub: channelCredentialStub{},
		err: integrations.NewProviderErrorWithReason(
			integrations.ErrorCodeAccessDenied,
			"permission_missing",
			"provider denied identity lookup",
			nil,
			integrations.ProviderDiagnostics{ErrorCode: "230027", RequestID: "req-safe", HTTPStatus: 403},
		),
	}
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu saved before test",
	})
	if err != nil {
		t.Fatalf("CreateInstallation() error = %v", err)
	}
	result, err := service.VerifyInstallation(t.Context(), scope, digitalHumanID, installation.ID)
	if err != nil {
		t.Fatalf("VerifyInstallation() error = %v", err)
	}
	if result.Status != "failed" || result.Failure == nil || result.Failure.Code != "access_denied" ||
		result.Failure.ReasonCode != "permission_missing" || result.Failure.ProviderErrorCode != "230027" ||
		result.Failure.ProviderRequestID != "req-safe" || result.Failure.ProviderHTTPStatus != 403 {
		t.Fatalf("VerifyInstallation() = %#v", result)
	}
	if stored := store.installations[installation.ID]; stored.ExternalTenantID != "" || stored.ExternalBotID != "" || stored.Revision != 1 {
		t.Fatalf("failed verification mutated installation = %#v", stored)
	}
}

func TestChannelConnectionTestRequiresRealInboundAndSuccessfulFinalDelivery(t *testing.T) {
	service, store, _, _, scope, digitalHumanID := newChannelTestService(t)
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu round trip",
	})
	if err != nil {
		t.Fatal(err)
	}
	test, err := service.StartConnectionTest(t.Context(), scope, digitalHumanID, installation.ID, StartChannelConnectionTestRequest{})
	if err != nil {
		t.Fatalf("StartConnectionTest() error = %v", err)
	}
	if test.Status != ChannelConnectionTestAwaitingMessage || test.ID == uuid.Nil || test.TestCode == "" || test.TestMessage == "" {
		t.Fatalf("StartConnectionTest() = %#v", test)
	}

	current, err := service.GetConnectionTest(t.Context(), scope, digitalHumanID, installation.ID, test.ID, test.ConversationType, test.StartedAt)
	if err != nil || current.Status != ChannelConnectionTestAwaitingMessage {
		t.Fatalf("GetConnectionTest(before inbound) = %#v, %v", current, err)
	}
	eventID := uuid.New()
	store.events[installation.ID.String()+":test-event"] = &channelruntime.InboundEvent{
		ID: eventID, InstallationID: installation.ID, ConversationType: channelruntime.ConversationTypeDirect,
		ContentPreview: test.TestMessage, Status: channelruntime.InboundEventAccepted, ReceivedAt: test.StartedAt.Add(time.Second),
	}
	current, err = service.GetConnectionTest(t.Context(), scope, digitalHumanID, installation.ID, test.ID, test.ConversationType, test.StartedAt)
	if err != nil || current.Status != ChannelConnectionTestMessageReceived || !current.MessageReceived || current.ReplyDelivered {
		t.Fatalf("GetConnectionTest(after inbound) = %#v, %v", current, err)
	}
	deliveryID := uuid.New()
	store.deliveries[deliveryID] = &channelruntime.Delivery{
		ID: deliveryID, InboundEventID: uuidPointer(eventID), Kind: channelruntime.DeliveryKindFinal,
		Status: channelruntime.DeliverySucceeded, CreatedAt: test.StartedAt.Add(2 * time.Second),
	}
	current, err = service.GetConnectionTest(t.Context(), scope, digitalHumanID, installation.ID, test.ID, test.ConversationType, test.StartedAt)
	if err != nil || current.Status != ChannelConnectionTestSucceeded || !current.MessageReceived || !current.ReplyDelivered {
		t.Fatalf("GetConnectionTest(after delivery) = %#v, %v", current, err)
	}
}

func TestChannelConnectionTestExposesConfirmedDeliveryFailure(t *testing.T) {
	service, store, _, _, scope, digitalHumanID := newChannelTestService(t)
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu denied reply",
	})
	if err != nil {
		t.Fatal(err)
	}
	test, err := service.StartConnectionTest(t.Context(), scope, digitalHumanID, installation.ID, StartChannelConnectionTestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	eventID := uuid.New()
	store.events[installation.ID.String()+":test-event-denied"] = &channelruntime.InboundEvent{
		ID: eventID, InstallationID: installation.ID, ConversationType: channelruntime.ConversationTypeDirect,
		ContentPreview: test.TestMessage, Status: channelruntime.InboundEventAccepted, ReceivedAt: test.StartedAt.Add(time.Second),
	}
	deliveryID := uuid.New()
	store.deliveries[deliveryID] = &channelruntime.Delivery{
		ID: deliveryID, InboundEventID: uuidPointer(eventID), Kind: channelruntime.DeliveryKindFinal,
		Status: channelruntime.DeliveryFailed, ErrorCode: "channel.provider.permission_or_resource_denied",
		CreatedAt: test.StartedAt.Add(2 * time.Second),
	}

	current, err := service.GetConnectionTest(t.Context(), scope, digitalHumanID, installation.ID, test.ID, test.ConversationType, test.StartedAt)
	if err != nil {
		t.Fatalf("GetConnectionTest() error = %v", err)
	}
	if current.Status != ChannelConnectionTestFailed || current.FailureCode != "permission_or_resource_denied" || !current.MessageReceived {
		t.Fatalf("GetConnectionTest() = %#v", current)
	}
}

func TestChannelConnectionTestExpiresWithoutRealMessage(t *testing.T) {
	service, _, _, _, scope, digitalHumanID := newChannelTestService(t)
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu timeout",
	})
	if err != nil {
		t.Fatal(err)
	}
	test, err := service.StartConnectionTest(t.Context(), scope, digitalHumanID, installation.ID, StartChannelConnectionTestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return test.ExpiresAt.Add(time.Second) }
	current, err := service.GetConnectionTest(t.Context(), scope, digitalHumanID, installation.ID, test.ID, test.ConversationType, test.StartedAt)
	if err != nil || current.Status != ChannelConnectionTestExpired || current.FailureCode != "message_not_received" {
		t.Fatalf("GetConnectionTest(expired) = %#v, %v", current, err)
	}
}

func TestChannelConnectionTestIgnoresEvidenceFromOtherInstallations(t *testing.T) {
	service, store, _, _, scope, digitalHumanID := newChannelTestService(t)
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Current Feishu connection",
	})
	if err != nil {
		t.Fatal(err)
	}
	test, err := service.StartConnectionTest(t.Context(), scope, digitalHumanID, installation.ID, StartChannelConnectionTestRequest{
		ConversationType: channelruntime.ConversationTypeGroup,
	})
	if err != nil {
		t.Fatal(err)
	}
	store.events["other-installation-event"] = &channelruntime.InboundEvent{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		InstallationID: uuid.New(), ConversationType: channelruntime.ConversationTypeGroup,
		EventType: "channel.mention", ContentPreview: test.TestMessage,
		Status: channelruntime.InboundEventAccepted, ReceivedAt: test.StartedAt.Add(time.Second),
	}

	current, err := service.GetConnectionTest(
		t.Context(), scope, digitalHumanID, installation.ID, test.ID, test.ConversationType, test.StartedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != ChannelConnectionTestAwaitingMessage || current.MessageReceived || current.FailureCode != "" {
		t.Fatalf("GetConnectionTest(other installation) = %#v", current)
	}

	service.now = func() time.Time { return test.ExpiresAt.Add(time.Second) }
	current, err = service.GetConnectionTest(
		t.Context(), scope, digitalHumanID, installation.ID, test.ID, test.ConversationType, test.StartedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != ChannelConnectionTestExpired || current.FailureCode != "message_not_received" {
		t.Fatalf("GetConnectionTest(expired other installation) = %#v", current)
	}
}

func TestGroupChannelConnectionTestChecksMentionHistoryMembersAndReply(t *testing.T) {
	service, store, _, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu group test",
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter.capabilityChecks = []channelruntime.ConversationCapabilityCheck{
		{
			Capability:          channelruntime.ConversationCapabilityMessageHistory,
			Status:              channelruntime.CapabilityCheckFailed,
			FailureCode:         "permission_missing",
			RequiredPermissions: []string{"im:message:readonly"},
		},
		{
			Capability:          channelruntime.ConversationCapabilityMemberNames,
			Status:              channelruntime.CapabilityCheckPassed,
			RequiredPermissions: []string{"im:chat.members:read"},
		},
	}
	test, err := service.StartConnectionTest(t.Context(), scope, digitalHumanID, installation.ID, StartChannelConnectionTestRequest{
		ConversationType: channelruntime.ConversationTypeGroup,
	})
	if err != nil {
		t.Fatal(err)
	}
	if test.ConversationType != channelruntime.ConversationTypeGroup || !strings.HasPrefix(test.TestMessage, "群聊连接测试 ") {
		t.Fatalf("StartConnectionTest(group) = %#v", test)
	}
	eventID := uuid.New()
	store.events[installation.ID.String()+":group-test-event"] = &channelruntime.InboundEvent{
		ID: eventID, InstallationID: installation.ID, ConversationType: channelruntime.ConversationTypeGroup,
		ExternalConversationID: "oc_group", ProviderMessageID: "om_group", SenderExternalID: "ou_sender",
		EventType: "channel.mention", ContentPreview: test.TestMessage, Status: channelruntime.InboundEventAccepted,
		ReceivedAt: test.StartedAt.Add(time.Second),
	}
	deliveryID := uuid.New()
	store.deliveries[deliveryID] = &channelruntime.Delivery{
		ID: deliveryID, InboundEventID: uuidPointer(eventID), Kind: channelruntime.DeliveryKindFinal,
		Status: channelruntime.DeliverySucceeded, CreatedAt: test.StartedAt.Add(2 * time.Second),
	}

	current, err := service.GetConnectionTest(
		t.Context(), scope, digitalHumanID, installation.ID, test.ID, test.ConversationType, test.StartedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != ChannelConnectionTestSucceeded || current.Readiness != ChannelConnectionTestReadinessPartial || adapter.capabilityCalls != 1 {
		t.Fatalf("GetConnectionTest(group) = %#v, capability calls = %d", current, adapter.capabilityCalls)
	}
	capabilities := make(map[string]ChannelConnectionCapability, len(current.Capabilities))
	for _, capability := range current.Capabilities {
		capabilities[capability.ID] = capability
	}
	if capabilities[ChannelConnectionCapabilityGroupMention].Status != ChannelConnectionCapabilityPassed ||
		capabilities[ChannelConnectionCapabilityReplyDelivery].Status != ChannelConnectionCapabilityPassed ||
		capabilities[ChannelConnectionCapabilityMessageHistory].FailureCode != "permission_missing" ||
		capabilities[ChannelConnectionCapabilityMemberNames].Status != ChannelConnectionCapabilityPassed {
		t.Fatalf("group capabilities = %#v", capabilities)
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
	if _, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, request); err == nil {
		t.Fatal("first HandleWebhook() error = nil")
	}
	result, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, request)
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
	if _, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"payload":"original"}`)}); err != nil {
		t.Fatalf("first HandleWebhook() error = %v", err)
	}
	adapter.message = &channelruntime.InboundMessage{
		Provider: "feishu", ProviderEventID: "event-collision", ProviderMessageID: "message-forged",
		ExternalTenantID: "tenant-1", ExternalConversationID: binding.Mapping.ExternalConversationID,
		ConversationType: channelruntime.ConversationTypeDirect, SenderExternalID: "ou_sender", Text: "替换内容",
		OccurredAt: time.Now().UTC(),
	}
	_, err := service.HandleWebhook(t.Context(), "feishu", installation.ID, channelruntime.WebhookRequest{Body: []byte(`{"payload":"changed"}`)})
	if !errors.Is(err, channelruntime.ErrInvalidWebhook) {
		t.Fatalf("reused event ID error = %v, want ErrInvalidWebhook", err)
	}
	if runtime.calls != 1 {
		t.Fatalf("SubmitInvocation() calls = %d, want 1", runtime.calls)
	}
}

func TestDeliveryConfirmedProviderFailureIsNotRetried(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeDirect)
	payload, digest, err := channelDeliveryPayload("reply")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	delivery := &channelruntime.Delivery{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		InstallationID: installation.ID, BindingID: binding.Binding.ID, InboundEventID: uuidPointer(uuid.New()), RunID: uuid.New(),
		Provider: "feishu", ConnectionID: installation.ConnectionID, ActorID: scope.ActorID,
		SubjectType: runtimeidentity.SubjectTypeDigitalHuman, SubjectID: digitalHumanID,
		ExternalConversationID: binding.Mapping.ExternalConversationID, Payload: payload, PayloadDigest: digest,
		Status: channelruntime.DeliveryPending, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	store.deliveries[delivery.ID] = delivery
	adapter.deliveryErr = integrations.NewErrorWithReason(
		integrations.ErrorCodeAccessDenied,
		"send_not_allowed",
		"temporary provider failure with sensitive upstream detail",
		nil,
	)
	service.now = func() time.Time { return now }
	if err := service.DispatchDeliveries(t.Context(), 10); !errors.Is(err, errChannelDeliveryAttemptFailed) || strings.Contains(err.Error(), "sensitive upstream detail") {
		t.Fatalf("DispatchDeliveries() error = %v, want safe delivery failure", err)
	}
	if stored := store.deliveries[delivery.ID]; stored.ErrorCode != "channel.provider.send_not_allowed" {
		t.Fatalf("delivery error code = %q, want channel.provider.send_not_allowed", stored.ErrorCode)
	}
	if stored := store.deliveries[delivery.ID]; stored.Status != channelruntime.DeliveryFailed || stored.AttemptCount != 1 {
		t.Fatalf("delivery = %#v, want failed after one attempt", stored)
	}
	adapter.deliveryErr = nil
	adapter.receipt = &channelruntime.DeliveryReceipt{ProviderMessageID: "om_1"}
	now = now.Add(time.Minute)
	if err := service.DispatchDeliveries(t.Context(), 10); err != nil {
		t.Fatalf("DispatchDeliveries(after terminal failure) error = %v", err)
	}
	if runtime.calls != 0 || adapter.deliveryCalls != 1 {
		t.Fatalf("SubmitInvocation() calls = %d, Deliver() calls = %d", runtime.calls, adapter.deliveryCalls)
	}
	if stored := store.deliveries[delivery.ID]; stored.Status != channelruntime.DeliveryFailed {
		t.Fatalf("delivery = %#v, want terminal failure", stored)
	}
}

func TestDeliveryTransientFailureRetriesWithoutSubmittingInvocationAgain(t *testing.T) {
	service, store, runtime, adapter, scope, digitalHumanID := newChannelTestService(t)
	installation, binding := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeDirect)
	now := time.Now().UTC()
	delivery := newChannelTestDelivery(t, scope, digitalHumanID, installation, binding, now)
	store.deliveries[delivery.ID] = delivery
	adapter.deliveryErr = integrations.NewError(
		integrations.ErrorCodeUpstream,
		"temporary provider failure with sensitive upstream detail",
		nil,
	)
	service.now = func() time.Time { return now }
	if err := service.DispatchDeliveries(t.Context(), 10); !errors.Is(err, errChannelDeliveryAttemptFailed) || strings.Contains(err.Error(), "sensitive upstream detail") {
		t.Fatalf("DispatchDeliveries() error = %v, want safe retryable failure", err)
	}
	if stored := store.deliveries[delivery.ID]; stored.Status != channelruntime.DeliveryPending || stored.AttemptCount != 1 {
		t.Fatalf("delivery = %#v, want pending retry", stored)
	}

	adapter.deliveryErr = nil
	adapter.receipt = &channelruntime.DeliveryReceipt{ProviderMessageID: "om_retry"}
	now = now.Add(time.Minute)
	if err := service.DispatchDeliveries(t.Context(), 10); err != nil {
		t.Fatalf("DispatchDeliveries(retry) error = %v", err)
	}
	stored := store.deliveries[delivery.ID]
	if runtime.calls != 0 || adapter.deliveryCalls != 2 || stored.Status != channelruntime.DeliverySucceeded || stored.AttemptCount != 2 {
		t.Fatalf("runtime calls = %d, delivery calls = %d, delivery = %#v", runtime.calls, adapter.deliveryCalls, stored)
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
	binding *ChannelRoute,
	now time.Time,
) *channelruntime.Delivery {
	t.Helper()
	payload, digest, err := channelDeliveryPayload("reply")
	if err != nil {
		t.Fatal(err)
	}
	return &channelruntime.Delivery{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		InstallationID: installation.ID, BindingID: binding.Binding.ID, InboundEventID: uuidPointer(uuid.New()), RunID: uuid.New(),
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
	registry, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: adapter, Provisioner: channelProvisionerStub{}})
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

func TestChannelWebhookProbeAcceptsOnlyActiveMatchingInstallation(t *testing.T) {
	service, store, _, _, scope, digitalHumanID := newChannelTestService(t)
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu probe",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProbeWebhook(t.Context(), "FEISHU", installation.ID); err != nil {
		t.Fatalf("ProbeWebhook(active) error = %v", err)
	}
	if err := service.ProbeWebhook(t.Context(), "dingtalk", installation.ID); !errors.Is(err, ErrChannelInstallationNotFound) {
		t.Fatalf("ProbeWebhook(provider mismatch) error = %v", err)
	}
	store.installations[installation.ID].Status = channelruntime.InstallationStatusDisabled
	if err := service.ProbeWebhook(t.Context(), "feishu", installation.ID); !errors.Is(err, ErrChannelInstallationNotFound) {
		t.Fatalf("ProbeWebhook(disabled) error = %v", err)
	}
}

func createChannelTestRoute(t *testing.T, service *ChannelService, scope Scope, digitalHumanID uuid.UUID, conversationType channelruntime.ConversationType) (*channelruntime.Installation, *ChannelRoute) {
	t.Helper()
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "feishu", ConnectionID: uuid.New(), Name: "Feishu test",
	})
	if err != nil {
		t.Fatal(err)
	}
	verification, err := service.VerifyInstallation(t.Context(), scope, digitalHumanID, installation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Status != "verified" {
		t.Fatalf("verification = %#v", verification)
	}
	installation, err = service.store.ScopedChannelInstallationByID(t.Context(), scope, digitalHumanID, installation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if installation.ExternalTenantID != "tenant-1" || installation.ExternalBotID != "ou_bot" {
		t.Fatalf("resolved channel identity = (%q, %q)", installation.ExternalTenantID, installation.ExternalBotID)
	}
	now := time.Now().UTC()
	mapping := &channelruntime.ConversationMapping{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		InstallationID: installation.ID, ExternalConversationID: "oc_" + uuid.NewString(),
		ConversationType: conversationType, LocalConversationID: uuidPointer(uuid.New()),
		CreatedAt: now, UpdatedAt: now,
	}
	binding := &channelruntime.Binding{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		InstallationID: installation.ID, ConversationMappingID: mapping.ID,
		SubjectType: runtimeidentity.SubjectTypeDigitalHuman, SubjectID: digitalHumanID,
		WakePolicy: channelruntime.WakePolicyDirectAndMention, Enabled: true, Revision: 1,
		CreatedBy: scope.ActorID, UpdatedBy: scope.ActorID, CreatedAt: now, UpdatedAt: now,
	}
	if err := service.store.CreateChannelBinding(t.Context(), mapping, binding); err != nil {
		t.Fatal(err)
	}
	return installation, &ChannelRoute{Installation: installation, Mapping: mapping, Binding: binding}
}

type channelSubjectStub struct{ definition *Definition }

func (stub channelSubjectStub) Definition(_ context.Context, scope Scope, id uuid.UUID) (*Definition, error) {
	if stub.definition == nil || stub.definition.ID != id || stub.definition.OrganizationID != scope.OrganizationID || stub.definition.WorkspaceID != scope.WorkspaceID {
		return nil, ErrNotFound
	}
	return stub.definition, nil
}

type channelRuntimeStub struct {
	calls                int
	runID                uuid.UUID
	lastEventID          string
	lastEvent            invocation.Event
	lastQuery            string
	lastConversationID   *uuid.UUID
	remainingFailures    int
	channelPolicy        *ChannelResponsePolicy
	runtimeInputAccepted bool
	lastRuntimeInput     string
	lastRuntimeRequest   ChannelRuntimeInputRequest
	runtimeInputCalls    int
}

type channelTransientDispatcherStub struct {
	requests []ChannelTransientDeliveryRequest
	err      error
}

func (dispatcher *channelTransientDispatcherStub) EnqueueTransient(_ context.Context, request ChannelTransientDeliveryRequest) error {
	dispatcher.requests = append(dispatcher.requests, request)
	return dispatcher.err
}

func (runtime *channelRuntimeStub) SubmitInvocation(_ context.Context, _ Scope, event invocation.Event, query string, conversationID *uuid.UUID) (*Run, bool, error) {
	runtime.calls++
	runtime.lastEventID = event.ID
	runtime.lastEvent = event
	runtime.lastQuery = query
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

func (runtime *channelRuntimeStub) SubmitChannelRuntimeInput(_ context.Context, _ Scope, _ uuid.UUID, _ uuid.UUID, request ChannelRuntimeInputRequest) (bool, error) {
	runtime.runtimeInputCalls++
	runtime.lastRuntimeInput = request.Content
	runtime.lastRuntimeRequest = request
	return runtime.runtimeInputAccepted, nil
}

type channelCredentialStub struct{}

func (channelCredentialStub) ResolveIdentity(context.Context, *channelruntime.Installation) (string, string, error) {
	return "tenant-1", "ou_bot", nil
}
func (channelCredentialStub) ResolveCredential(context.Context, *channelruntime.Installation) (*channelruntime.ResolvedChannelCredential, error) {
	return &channelruntime.ResolvedChannelCredential{Values: map[string]string{"safe": "value"}}, nil
}

func (channelCredentialStub) ContextReadiness(context.Context, *channelruntime.Installation) ChannelContextReadiness {
	return ChannelContextReadiness{Status: ChannelContextReadinessReady, MessageHistoryAvailable: true, MemberNamesAvailable: true}
}

type failingChannelCredentialStub struct {
	channelCredentialStub
	err error
}

func (stub failingChannelCredentialStub) ResolveIdentity(context.Context, *channelruntime.Installation) (string, string, error) {
	return "", "", stub.err
}

type channelAdapterStub struct {
	provider         string
	message          *channelruntime.InboundMessage
	receipt          *channelruntime.DeliveryReceipt
	deliveryErr      error
	deliveryCalls    int
	lastDelivery     channelruntime.DeliveryRequest
	capabilityChecks []channelruntime.ConversationCapabilityCheck
	capabilityCalls  int
}

func (adapter *channelAdapterStub) Provider() string {
	if adapter.provider != "" {
		return adapter.provider
	}
	return "feishu"
}
func (*channelAdapterStub) Capabilities() channelruntime.ProviderCapabilities {
	return channelruntime.ProviderCapabilities{
		ContextRead: true, MessageRoundTripTest: true, ConversationCapabilityTest: true,
	}
}

func (adapter *channelAdapterStub) TestConversationCapabilities(
	_ context.Context,
	_ channelruntime.ConversationCapabilityTestRequest,
) []channelruntime.ConversationCapabilityCheck {
	adapter.capabilityCalls++
	return append([]channelruntime.ConversationCapabilityCheck(nil), adapter.capabilityChecks...)
}
func (*channelAdapterStub) Verify(_ context.Context, request channelruntime.WebhookRequest, _ channelruntime.ResolvedChannelCredential) (*channelruntime.VerifiedWebhook, error) {
	return &channelruntime.VerifiedWebhook{Body: append([]byte(nil), request.Body...)}, nil
}
func (adapter *channelAdapterStub) Normalize(context.Context, *channelruntime.VerifiedWebhook, channelruntime.NormalizationContext) (*channelruntime.InboundMessage, error) {
	if adapter.message == nil {
		return nil, nil
	}
	message := *adapter.message
	if message.WakeReason == "" {
		if message.ConversationType == channelruntime.ConversationTypeDirect {
			message.WakeReason = channelruntime.WakeReasonDirectMessage
		} else {
			message.WakeReason = channelruntime.WakeReasonNone
			for _, mention := range message.Mentions {
				if mention.ExternalID == "ou_bot" {
					message.WakeReason = channelruntime.WakeReasonMention
					break
				}
			}
		}
	}
	return &message, nil
}
func (*channelAdapterStub) Acknowledgement() channelruntime.WebhookResponse {
	return channelruntime.WebhookResponse{StatusCode: 200}
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

type channelProvisionerStub struct{}

func (channelProvisionerStub) PrepareCredentials(_, updates map[string]string) (map[string]string, error) {
	return cloneChannelCredentials(updates), nil
}
func (channelProvisionerStub) ResolveIdentity(context.Context, channelruntime.ResolvedChannelCredential) (channelruntime.InstallationIdentity, error) {
	return channelruntime.InstallationIdentity{ExternalTenantID: "tenant-1", ExternalSelfID: "ou_bot"}, nil
}
func (channelProvisionerStub) ConnectionSpec() channelruntime.ConnectionSpec {
	return channelruntime.ConnectionSpec{IntegrationID: "feishu", DriverID: "test", AuthMethodID: "test"}
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
func (store *channelStoreStub) ScopedChannelInstallationByID(ctx context.Context, scope Scope, digitalHumanID, id uuid.UUID) (*channelruntime.Installation, error) {
	installation, err := store.ChannelInstallationByID(ctx, id)
	if err != nil || installation.OrganizationID != scope.OrganizationID || installation.WorkspaceID != scope.WorkspaceID ||
		!isDigitalHumanChannelInstallation(installation) || *installation.DefaultSubjectID != digitalHumanID {
		return nil, ErrChannelInstallationNotFound
	}
	return installation, nil
}
func (store *channelStoreStub) ListChannelInstallations(_ context.Context, scope Scope, digitalHumanID uuid.UUID) ([]*channelruntime.Installation, error) {
	items := make([]*channelruntime.Installation, 0, len(store.installations))
	for _, installation := range store.installations {
		if installation.OrganizationID != scope.OrganizationID || installation.WorkspaceID != scope.WorkspaceID ||
			!isDigitalHumanChannelInstallation(installation) || *installation.DefaultSubjectID != digitalHumanID {
			continue
		}
		items = append(items, cloneInstallation(installation))
	}
	return items, nil
}
func (store *channelStoreStub) UpdateChannelInstallation(_ context.Context, _ Scope, installation *channelruntime.Installation, expected int64) error {
	current := store.installations[installation.ID]
	if current == nil || current.Revision != expected {
		return ErrChannelConflict
	}
	installation.Revision = expected + 1
	store.installations[installation.ID] = cloneInstallation(installation)
	return nil
}
func (store *channelStoreStub) DeleteChannelInstallation(_ context.Context, scope Scope, digitalHumanID, id uuid.UUID) error {
	installation := store.installations[id]
	if installation == nil || installation.OrganizationID != scope.OrganizationID || installation.WorkspaceID != scope.WorkspaceID ||
		!isDigitalHumanChannelInstallation(installation) || *installation.DefaultSubjectID != digitalHumanID {
		return ErrChannelInstallationNotFound
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
			InstallationID: installation.ID, BindingID: binding.ID, InboundEventID: uuidPointer(event.ID), RunID: runID,
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
func (store *channelStoreStub) ChannelConnectionTestEvidence(_ context.Context, installationID uuid.UUID, conversationType channelruntime.ConversationType, message string, startedAt time.Time) (*channelruntime.InboundEvent, *channelruntime.Delivery, error) {
	var matched *channelruntime.InboundEvent
	for _, event := range store.events {
		if event.InstallationID == installationID && event.ConversationType == conversationType &&
			event.ContentPreview == message && !event.ReceivedAt.Before(startedAt) {
			if conversationType == channelruntime.ConversationTypeGroup && event.EventType != "channel.mention" {
				continue
			}
			matched = event
		}
	}
	if matched == nil {
		return nil, nil, nil
	}
	for _, delivery := range store.deliveries {
		if delivery.InboundEventID != nil && *delivery.InboundEventID == matched.ID && delivery.Kind == channelruntime.DeliveryKindFinal {
			return matched, delivery, nil
		}
	}
	return matched, nil, nil
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
