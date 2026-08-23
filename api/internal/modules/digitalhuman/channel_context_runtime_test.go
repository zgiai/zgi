package digitalhuman

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
)

func TestChannelContextRuntimePreparesAttributedPoemContext(t *testing.T) {
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ConnectionID: uuid.New(),
		Provider: "feishu", CreatedBy: uuid.New(),
	}
	reader := &channelContextReaderStub{pack: &channelruntime.ChannelContextPack{
		Status:        channelruntime.ContextStatusComplete,
		CurrentAuthor: channelruntime.ChannelAuthor{ExternalID: "ou_c", Label: "小李"},
		Messages: []channelruntime.ChannelContextMessage{
			{MessageID: "m2", Author: channelruntime.ChannelAuthor{ExternalID: "ou_b", Label: "小王"}, Text: "这首诗真棒。", MessageType: "text", OccurredAt: now.Add(-time.Minute)},
			{MessageID: "m3", Author: channelruntime.ChannelAuthor{ExternalID: "ou_c", Label: "小李"}, Text: "赏析一下这首诗。", MessageType: "text", OccurredAt: now},
			{MessageID: "m1", Author: channelruntime.ChannelAuthor{ExternalID: "ou_a", Label: "康源"}, Text: "我写了一首诗：春风又绿江南岸。", MessageType: "text", OccurredAt: now.Add(-2 * time.Minute)},
		},
	}}
	runtime := NewChannelContextRuntime(
		&channelContextStoreStub{installation: installation},
		&channelContextRegistryStub{adapter: reader},
		channelContextModelResolverStub{contextWindow: 32_000},
	)
	run := &Run{ID: uuid.New(), OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID}
	event := invocation.Event{
		ID: "event-1", Type: invocation.EventTypeChannelMention,
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: "feishu", InstallationID: installation.ID.String()},
		Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New(), OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_c"}, OccurredAt: now,
		Context: map[string]any{
			"conversation_type": "group", "external_conversation_id": "oc_poem", "provider_message_id": "m3",
		},
	}

	preparation, err := runtime.Prepare(t.Context(), run, &ResolvedExecution{}, event, "openai", "gpt-test")
	if err != nil {
		t.Fatal(err)
	}
	if preparation == nil {
		t.Fatal("context preparation is nil")
	}
	addendum := preparation.SystemPromptAddendum
	for _, expected := range []string{"康源", "我写了一首诗", "小王", "这首诗真棒", "Current requester: 小李"} {
		if !strings.Contains(addendum, expected) {
			t.Fatalf("system context missing %q: %s", expected, addendum)
		}
	}
	if strings.Contains(addendum, "赏析一下这首诗") {
		t.Fatalf("current request was duplicated into historical context: %s", addendum)
	}
	if strings.Index(addendum, "康源") > strings.Index(addendum, "小王") {
		t.Fatalf("messages are not chronological: %s", addendum)
	}
	if preparation.Diagnostics.MessageCount != 2 || preparation.Diagnostics.AuthorCount != 2 {
		t.Fatalf("diagnostics = %+v", preparation.Diagnostics)
	}
	if reader.request.ExternalConversationID != "oc_poem" || reader.request.CurrentMessageID != "m3" {
		t.Fatalf("reader request escaped server-bound conversation: %+v", reader.request)
	}
}

func TestChannelContextRuntimeUsesDirectChatAsPrimaryWindow(t *testing.T) {
	now := time.Date(2026, 8, 22, 11, 0, 0, 0, time.UTC)
	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ConnectionID: uuid.New(),
		Provider: "feishu", CreatedBy: uuid.New(),
	}
	reader := &channelContextReaderStub{pack: &channelruntime.ChannelContextPack{
		Status:        channelruntime.ContextStatusComplete,
		CurrentAuthor: channelruntime.ChannelAuthor{ExternalID: "ou_private", Label: "康源"},
		Messages: []channelruntime.ChannelContextMessage{
			{MessageID: "m-before", Author: channelruntime.ChannelAuthor{ExternalID: "ou_private", Label: "康源"}, Text: "接着刚才的方案继续", MessageType: "text", OccurredAt: now.Add(-time.Minute)},
			{MessageID: "m-current", Author: channelruntime.ChannelAuthor{ExternalID: "ou_private", Label: "康源"}, Text: "给出下一步", MessageType: "text", OccurredAt: now},
		},
	}}
	runtime := NewChannelContextRuntime(
		&channelContextStoreStub{installation: installation},
		&channelContextRegistryStub{adapter: reader}, nil,
	)
	run := &Run{ID: uuid.New(), OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID}
	event := invocation.Event{
		ID: "event-direct", Type: invocation.EventTypeChannelMessage,
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: "feishu", InstallationID: installation.ID.String()},
		Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New()},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_private"}, OccurredAt: now,
		Context: map[string]any{
			"conversation_type": "direct", "external_conversation_id": "oc_private", "provider_message_id": "m-current",
		},
	}

	preparation, err := runtime.Prepare(t.Context(), run, &ResolvedExecution{}, event, "openai", "gpt-test")
	if err != nil {
		t.Fatal(err)
	}
	if preparation == nil || !strings.Contains(preparation.SystemPromptAddendum, "Primary channel conversation context") ||
		!strings.Contains(preparation.SystemPromptAddendum, "接着刚才的方案继续") {
		t.Fatalf("direct-chat preparation = %#v", preparation)
	}
	if len(preparation.ContextTexts) != 1 || preparation.ContextTexts[0] != "接着刚才的方案继续" {
		t.Fatalf("direct-chat de-duplication texts = %#v", preparation.ContextTexts)
	}
}

func TestChannelContextRuntimeAnswersCurrentMessageWhenProviderHasNoHistoryReader(t *testing.T) {
	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ConnectionID: uuid.New(),
		Provider: "dingtalk", CreatedBy: uuid.New(),
	}
	runtime := NewChannelContextRuntime(
		&channelContextStoreStub{installation: installation},
		&channelContextRegistryStub{adapter: channelContextNoReaderAdapterStub{}}, nil,
	)
	run := &Run{ID: uuid.New(), OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID}
	event := invocation.Event{
		ID: "event-dingtalk", Type: invocation.EventTypeChannelMessage,
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: "dingtalk", InstallationID: installation.ID.String()},
		Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New()},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "user-1"},
		Context: map[string]any{
			"conversation_type": "direct", "external_conversation_id": "direct:user-1",
			"provider_message_id": "message-1", "sender_display_name": "康源",
		},
	}

	preparation, err := runtime.Prepare(t.Context(), run, &ResolvedExecution{}, event, "openai", "gpt-test")
	if err != nil {
		t.Fatal(err)
	}
	if preparation == nil || preparation.Diagnostics.ErrorCode != "channel_context_not_supported" {
		t.Fatalf("preparation = %#v", preparation)
	}
	for _, expected := range []string{"sent by 康源", "Answer the current request normally"} {
		if !strings.Contains(preparation.SystemPromptAddendum, expected) {
			t.Fatalf("fallback context missing %q: %s", expected, preparation.SystemPromptAddendum)
		}
	}
	if strings.Contains(preparation.SystemPromptAddendum, "ask the user to quote") {
		t.Fatalf("fallback incorrectly forces a clarification: %s", preparation.SystemPromptAddendum)
	}
}

func TestChannelContextRuntimeLetsModelInterpretMentionOnlyHistoryAcrossSenders(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 30, 0, 0, time.UTC)
	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ConnectionID: uuid.New(),
		Provider: "feishu", CreatedBy: uuid.New(),
	}
	reader := &channelContextReaderStub{pack: &channelruntime.ChannelContextPack{
		Status:        channelruntime.ContextStatusComplete,
		CurrentAuthor: channelruntime.ChannelAuthor{ExternalID: "ou_b", Label: "小王"},
		Messages: []channelruntime.ChannelContextMessage{
			{MessageID: "m-question", Author: channelruntime.ChannelAuthor{ExternalID: "ou_a", Label: "康源"}, Text: "这个问题怎么处理：xxxxx", MessageType: "text", OccurredAt: now.Add(-time.Minute)},
			{MessageID: "m-wake", Author: channelruntime.ChannelAuthor{ExternalID: "ou_b", Label: "小王"}, Text: "@数字人", MessageType: "text", OccurredAt: now},
		},
	}}
	runtime := NewChannelContextRuntime(
		&channelContextStoreStub{installation: installation},
		&channelContextRegistryStub{adapter: reader}, nil,
	)
	run := &Run{ID: uuid.New(), OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID}
	event := invocation.Event{
		ID: "event-wake", Type: invocation.EventTypeChannelMention,
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: "feishu", InstallationID: installation.ID.String()},
		Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New()},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_b"}, OccurredAt: now,
		Context: map[string]any{
			"conversation_type": "group", "external_conversation_id": "oc_team", "provider_message_id": "m-wake", "mention_only": true,
		},
	}

	preparation, err := runtime.Prepare(t.Context(), run, &ResolvedExecution{}, event, "openai", "gpt-test")
	if err != nil {
		t.Fatal(err)
	}
	addendum := preparation.SystemPromptAddendum
	for _, expected := range []string{
		"Current requester: 小王",
		"康源: 这个问题怎么处理：xxxxx",
		"only a mention of you",
		"decide for yourself",
		"different sender mentioned you",
		"none have been selected for you",
	} {
		if !strings.Contains(addendum, expected) {
			t.Fatalf("mention-only context missing %q: %s", expected, addendum)
		}
	}
	if strings.Contains(addendum, "current incoming message is the only request") || strings.Contains(addendum, "小王: @数字人") {
		t.Fatalf("mention-only wake was misrepresented or duplicated: %s", addendum)
	}
	if !reader.request.MentionOnly {
		t.Fatalf("context reader request = %#v, want MentionOnly=true", reader.request)
	}
}

func TestChannelContextRuntimeMarksOnlyOwnedFinalRepliesAsSelf(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	digitalHumanID := uuid.New()
	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ConnectionID: uuid.New(),
		Provider: "feishu", CreatedBy: uuid.New(),
	}
	reader := &channelContextReaderStub{pack: &channelruntime.ChannelContextPack{
		Status:        channelruntime.ContextStatusComplete,
		CurrentAuthor: channelruntime.ChannelAuthor{ExternalID: "ou_user", Label: "康源"},
		Messages: []channelruntime.ChannelContextMessage{
			{MessageID: "m-user", Author: channelruntime.ChannelAuthor{ExternalID: "ou_user", Label: "康源"}, Text: "帮我总结", MessageType: "text", OccurredAt: now.Add(-3 * time.Minute)},
			{MessageID: "m-ack", Author: channelruntime.ChannelAuthor{ExternalID: "cli_app"}, Text: "内部确认消息", MessageType: "text", OccurredAt: now.Add(-2 * time.Minute)},
			{MessageID: "m-provider-ack", Author: channelruntime.ChannelAuthor{ExternalID: "cli_app", IsSelf: true}, Text: channelAcknowledgementText, MessageType: "text", OccurredAt: now.Add(-105 * time.Second)},
			{MessageID: "m-provider-self", Author: channelruntime.ChannelAuthor{ExternalID: "cli_app", IsSelf: true}, Text: "我发过一条临时进度", MessageType: "text", OccurredAt: now.Add(-90 * time.Second)},
			{MessageID: "m-self", Author: channelruntime.ChannelAuthor{ExternalID: "cli_app"}, Text: "我此前完成了总结", MessageType: "text", OccurredAt: now.Add(-time.Minute)},
			{MessageID: "m-other", Author: channelruntime.ChannelAuthor{ExternalID: "cli_other", Label: "其他助手"}, Text: "另一位参与者的回复", MessageType: "text", OccurredAt: now.Add(-30 * time.Second)},
		},
	}}
	store := &channelContextStoreStub{
		installation: installation,
		deliveryKinds: map[string]channelruntime.DeliveryKind{
			"m-ack":  channelruntime.DeliveryKindAcknowledgement,
			"m-self": channelruntime.DeliveryKindFinal,
		},
	}
	runtime := NewChannelContextRuntime(store, &channelContextRegistryStub{adapter: reader}, nil)
	event := invocation.Event{
		ID: "event-self", Type: invocation.EventTypeChannelMention,
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: "feishu", InstallationID: installation.ID.String()},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID,
			OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID,
		},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_user"}, OccurredAt: now,
		Context: map[string]any{
			"conversation_type": "group", "external_conversation_id": "oc_self", "provider_message_id": "m-current",
		},
	}
	resolved := &ResolvedExecution{Snapshot: ExecutionSnapshot{IdentityName: "运营增长专员"}}
	run := &Run{ID: uuid.New(), DigitalHumanID: digitalHumanID, OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID}

	preparation, err := runtime.Prepare(t.Context(), run, resolved, event, "openai", "gpt-test")
	if err != nil {
		t.Fatal(err)
	}
	addendum := preparation.SystemPromptAddendum
	for _, expected := range []string{
		"Active DigitalHuman identity: 运营增长专员.",
		"SELF (运营增长专员): 我发过一条临时进度",
		"SELF (运营增长专员): 我此前完成了总结",
		"其他助手: 另一位参与者的回复",
		"Refer to SELF in the first person",
	} {
		if !strings.Contains(addendum, expected) {
			t.Fatalf("self-aware channel context missing %q: %s", expected, addendum)
		}
	}
	if strings.Contains(addendum, "内部确认消息") || strings.Contains(addendum, channelAcknowledgementText) {
		t.Fatalf("transient acknowledgement leaked into semantic context: %s", addendum)
	}
	if store.subjectID != digitalHumanID {
		t.Fatalf("ownership subject = %s, want %s", store.subjectID, digitalHumanID)
	}
}

func TestChannelContextStableFallbackLabelsKeepSendersDistinct(t *testing.T) {
	installationID := uuid.New()
	first := stableChannelAuthorAlias(installationID, "oc_1", "ou_1")
	if first != stableChannelAuthorAlias(installationID, "oc_1", "ou_1") {
		t.Fatal("fallback label is unstable")
	}
	if first == stableChannelAuthorAlias(installationID, "oc_1", "ou_2") {
		t.Fatal("different senders were merged")
	}
}

func TestChannelContextSelectionPrioritizesMessagesAfterPreviousReply(t *testing.T) {
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	after := now.Add(-90 * time.Second)
	messages := []channelruntime.ChannelContextMessage{
		{MessageID: "m-poem", Text: "更早但仍可作为附近上下文读取的原诗", OccurredAt: now.Add(-2 * time.Minute)},
		{MessageID: "m-comment", Text: "上次回复后的评价", OccurredAt: now.Add(-time.Minute)},
		{MessageID: "m-followup", Text: "上次回复后的补充", OccurredAt: now.Add(-30 * time.Second)},
	}
	selected, _, truncated := selectChannelContextMessages(
		tokenestimate.NewEstimator(), messages,
		channelruntime.ChannelContextRequest{CurrentMessageID: "m-current", OccurredAt: now, After: &after},
		"gpt-test", 2_000, 2,
	)
	if !truncated || len(selected) != 2 || selected[0].MessageID != "m-comment" || selected[1].MessageID != "m-followup" {
		t.Fatalf("selected messages = %#v, truncated = %v", selected, truncated)
	}
}

func TestChannelContextSelectionPreservesParticipantCoverageBeforeLongSelfReplies(t *testing.T) {
	now := time.Date(2026, 8, 22, 13, 0, 0, 0, time.UTC)
	longReply := strings.Repeat("历史总结内容", 600)
	messages := []channelruntime.ChannelContextMessage{
		{MessageID: "m-a", Author: channelruntime.ChannelAuthor{ExternalID: "ou_a", Label: "康源"}, Text: "原始问题 A", OccurredAt: now.Add(-5 * time.Minute)},
		{MessageID: "m-b", Author: channelruntime.ChannelAuthor{ExternalID: "ou_b", Label: "小王"}, Text: "原始问题 B", OccurredAt: now.Add(-4 * time.Minute)},
		{MessageID: "m-self-1", Author: channelruntime.ChannelAuthor{ExternalID: "cli_app", IsSelf: true}, Text: longReply, OccurredAt: now.Add(-3 * time.Minute)},
		{MessageID: "m-self-2", Author: channelruntime.ChannelAuthor{ExternalID: "cli_app", IsSelf: true}, Text: longReply, OccurredAt: now.Add(-2 * time.Minute)},
	}

	selected, _, truncated := selectChannelContextMessages(
		tokenestimate.NewEstimator(), messages,
		channelruntime.ChannelContextRequest{CurrentMessageID: "m-current", OccurredAt: now},
		"gpt-test", 1_000, 4,
	)
	selectedIDs := make(map[string]bool, len(selected))
	for _, message := range selected {
		selectedIDs[message.MessageID] = true
	}
	if !selectedIDs["m-a"] || !selectedIDs["m-b"] {
		t.Fatalf("participant messages were crowded out: %#v", selected)
	}
	if !truncated {
		t.Fatalf("truncated = false, want bounded long self replies to be reported")
	}
}

func TestChannelContextRendersReplyRelationshipsWithoutProviderIDs(t *testing.T) {
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	messages := []channelruntime.ChannelContextMessage{
		{MessageID: "provider-root-secret", Author: channelruntime.ChannelAuthor{Label: "康源"}, Text: "原诗", OccurredAt: now.Add(-time.Minute)},
		{
			MessageID: "provider-parent-secret", RootMessageID: "provider-root-secret", ParentMessageID: "provider-root-secret", ThreadID: "provider-thread-secret",
			Author: channelruntime.ChannelAuthor{Label: "小王"}, Text: "评价", OccurredAt: now, Edited: true,
		},
	}
	rendered := renderChannelContextMessagesWithRequest(messages, channelruntime.ChannelContextRequest{
		RootMessageID: "provider-root-secret", ParentMessageID: "provider-parent-secret", ThreadID: "provider-thread-secret",
	})
	for _, marker := range []string{"root of current request", "parent of current request", "reply to #1", "root #1", "thread", "edited"} {
		if !strings.Contains(rendered, marker) {
			t.Fatalf("rendered context missing %q: %s", marker, rendered)
		}
	}
	for _, providerID := range []string{"provider-root-secret", "provider-parent-secret", "provider-thread-secret"} {
		if strings.Contains(rendered, providerID) {
			t.Fatalf("provider ID leaked into model context: %s", rendered)
		}
	}
}

func TestChannelContextDisambiguatesDuplicateDisplayNames(t *testing.T) {
	installationID := uuid.New()
	messages := []channelruntime.ChannelContextMessage{
		{Author: channelruntime.ChannelAuthor{ExternalID: "ou_1", Label: "小王"}},
		{Author: channelruntime.ChannelAuthor{ExternalID: "ou_2", Label: "小王"}},
	}
	assignChannelAuthorLabels(messages, channelruntime.ChannelAuthor{ExternalID: "ou_current", Label: "小王"}, installationID, "oc_1")
	if messages[0].Author.Label == messages[1].Author.Label || messages[0].Author.Label == "小王" || messages[1].Author.Label == "小王" {
		t.Fatalf("duplicate labels were not disambiguated: %#v", messages)
	}
}

func TestExecutionTraceKeepsOnlyChannelContextDiagnostics(t *testing.T) {
	trace := executionTraceFromMetadata(map[string]interface{}{
		"channel_context": []interface{}{map[string]interface{}{
			"status": "complete", "message_count": 3, "author_count": 3,
			"messages": []string{"secret poem"}, "sender_id": "ou_secret",
		}},
	})
	encoded := string(trace)
	if strings.Contains(encoded, "secret poem") || strings.Contains(encoded, "ou_secret") {
		t.Fatalf("channel content leaked into execution trace: %s", encoded)
	}
	if !strings.Contains(encoded, "message_count") {
		t.Fatalf("safe diagnostics missing from trace: %s", encoded)
	}
}

func TestChannelContextExpansionUsesRotatingServerBoundCursorWithoutPersistingContent(t *testing.T) {
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	installationID := uuid.New()
	reader := &channelContextReaderStub{packs: []*channelruntime.ChannelContextPack{
		{
			Status: channelruntime.ContextStatusComplete, NextPageToken: "provider-page-2",
			Messages: []channelruntime.ChannelContextMessage{{
				MessageID: "m-old-1", Author: channelruntime.ChannelAuthor{ExternalID: "ou_a", Label: "康源"},
				Text: "第一段仅供本轮模型使用", MessageType: "text", OccurredAt: now.Add(-2 * time.Hour),
			}},
		},
		{
			Status: channelruntime.ContextStatusPartial,
			Messages: []channelruntime.ChannelContextMessage{{
				MessageID: "m-old-2", Author: channelruntime.ChannelAuthor{ExternalID: "ou_b", Label: "小王"},
				Text: "第二段仅供本轮模型使用", MessageType: "text", OccurredAt: now.Add(-3 * time.Hour),
			}},
		},
	}}
	runtime := NewChannelContextRuntime(
		&channelContextStoreStub{installation: &channelruntime.Installation{ID: installationID}},
		&channelContextRegistryStub{adapter: reader}, nil,
	)
	diagnostics := ChannelContextDiagnostics{Status: string(channelruntime.ContextStatusComplete), AuthorCount: 1}
	request := channelruntime.ChannelContextRequest{
		InstallationID: installationID, ExternalConversationID: "oc_bound", CurrentMessageID: "m-current",
		OccurredAt: now, Limit: initialContextMessageLimit,
	}
	tool := runtime.contextExpansionTool(reader, request, "provider-page-1", nil, "gpt-test", "", 2_000, 10, nil, &diagnostics)
	initialCursor := strings.TrimSpace(strings.TrimPrefix(tool.Definition.Function.Description[strings.LastIndex(tool.Definition.Function.Description, " Current cursor: "):], " Current cursor: "))

	invalid := callChannelContextTool(t, tool, "forged-cursor")
	if invalid.Status != "failed" || len(reader.requests) != 0 {
		t.Fatalf("forged cursor result = %#v, reads = %d", invalid, len(reader.requests))
	}
	first := callChannelContextTool(t, tool, initialCursor)
	if first.Status != "success" || len(reader.requests) != 1 || reader.requests[0].PageToken != "provider-page-1" || reader.requests[0].ExternalConversationID != "oc_bound" {
		t.Fatalf("first expansion = %#v, requests = %#v", first, reader.requests)
	}
	encodedDurable, _ := json.Marshal(map[string]any{"arguments": first.Arguments, "result": first.Result})
	if strings.Contains(string(encodedDurable), "第一段") || strings.Contains(string(encodedDurable), "ou_a") {
		t.Fatalf("durable tool trace leaked context: %s", encodedDurable)
	}
	if !strings.Contains(first.ModelResult["context"].(string), "第一段") {
		t.Fatalf("ephemeral model result missing context: %#v", first.ModelResult)
	}
	nextCursor, _ := first.ModelResult["next_cursor"].(string)
	if nextCursor == "" || nextCursor == initialCursor {
		t.Fatalf("next cursor was not rotated: %#v", first.ModelResult)
	}
	replayed := callChannelContextTool(t, tool, initialCursor)
	if replayed.Status != "failed" || len(reader.requests) != 1 {
		t.Fatalf("old cursor replay = %#v, reads = %d", replayed, len(reader.requests))
	}
	second := callChannelContextTool(t, tool, nextCursor)
	if second.Status != "success" || len(reader.requests) != 2 || reader.requests[1].PageToken != "provider-page-2" || second.ModelResult["next_cursor"] != "" {
		t.Fatalf("second expansion = %#v, requests = %#v", second, reader.requests)
	}
	if diagnostics.Status != string(channelruntime.ContextStatusPartial) || diagnostics.MessageCount != 2 || diagnostics.AuthorCount != 2 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestChannelContextRuntimeOffersExpansionForMessagesOmittedFromFirstPage(t *testing.T) {
	now := time.Date(2026, 8, 22, 14, 0, 0, 0, time.UTC)
	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ConnectionID: uuid.New(),
		Provider: "feishu", CreatedBy: uuid.New(),
	}
	messages := make([]channelruntime.ChannelContextMessage, 0, 10)
	for index := 0; index < 10; index++ {
		messages = append(messages, channelruntime.ChannelContextMessage{
			MessageID:  fmt.Sprintf("m-%d", index),
			Author:     channelruntime.ChannelAuthor{ExternalID: "ou_a", Label: "康源"},
			Text:       strings.Repeat(fmt.Sprintf("第%d段历史", index), 180),
			OccurredAt: now.Add(-time.Duration(10-index) * time.Minute),
		})
	}
	reader := &channelContextReaderStub{pack: &channelruntime.ChannelContextPack{
		Status: channelruntime.ContextStatusComplete, CurrentAuthor: channelruntime.ChannelAuthor{ExternalID: "ou_a", Label: "康源"}, Messages: messages,
	}}
	runtime := NewChannelContextRuntime(
		&channelContextStoreStub{installation: installation},
		&channelContextRegistryStub{adapter: reader},
		channelContextModelResolverStub{contextWindow: 5_000},
	)
	run := &Run{ID: uuid.New(), OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID}
	event := invocation.Event{
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: "feishu", InstallationID: installation.ID.String()},
		Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New()},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_a"}, OccurredAt: now,
		Context: map[string]any{"conversation_type": "group", "external_conversation_id": "oc_team", "provider_message_id": "m-current"},
	}

	preparation, err := runtime.Prepare(t.Context(), run, &ResolvedExecution{}, event, "openai", "gpt-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(preparation.Tools) != 1 || !preparation.Diagnostics.Truncated ||
		!strings.Contains(preparation.SystemPromptAddendum, "Context coverage: partial") ||
		!strings.Contains(preparation.SystemPromptAddendum, channelContextToolName) {
		t.Fatalf("preparation = %#v", preparation)
	}
	tool := preparation.Tools[0]
	cursor := strings.TrimSpace(strings.TrimPrefix(tool.Definition.Function.Description[strings.LastIndex(tool.Definition.Function.Description, " Current cursor: "):], " Current cursor: "))
	result := callChannelContextTool(t, tool, cursor)
	if result.Status != "success" || len(reader.requests) != 1 {
		t.Fatalf("local expansion = %#v, provider reads = %d", result, len(reader.requests))
	}
	if context, _ := result.ModelResult["context"].(string); strings.TrimSpace(context) == "" {
		t.Fatalf("local expansion context = %#v", result.ModelResult)
	}
}

func callChannelContextTool(t *testing.T, tool skillloop.RuntimeTool, cursor string) skillloop.RuntimeToolResult {
	t.Helper()
	arguments, err := json.Marshal(map[string]string{"cursor": cursor, "reason": "需要更早的原文"})
	if err != nil {
		t.Fatal(err)
	}
	return tool.Handler(t.Context(), adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{
		Name: channelContextToolName, Arguments: string(arguments),
	}})
}

type channelContextStoreStub struct {
	installation  *channelruntime.Installation
	deliveryKinds map[string]channelruntime.DeliveryKind
	subjectID     uuid.UUID
}

func (stub *channelContextStoreStub) ChannelInstallationByID(context.Context, uuid.UUID) (*channelruntime.Installation, error) {
	return stub.installation, nil
}

func (*channelContextStoreStub) LastSuccessfulChannelDeliveryAt(context.Context, uuid.UUID, string, time.Time) (*time.Time, error) {
	return nil, nil
}

func (stub *channelContextStoreStub) SuccessfulChannelDeliveryKinds(
	_ context.Context,
	_ uuid.UUID,
	_ string,
	subjectID uuid.UUID,
	_ []string,
) (map[string]channelruntime.DeliveryKind, error) {
	stub.subjectID = subjectID
	kinds := make(map[string]channelruntime.DeliveryKind, len(stub.deliveryKinds))
	for messageID, kind := range stub.deliveryKinds {
		kinds[messageID] = kind
	}
	return kinds, nil
}

type channelContextRegistryStub struct{ adapter channelruntime.Adapter }

func (stub *channelContextRegistryStub) Resolve(string) (channelruntime.Adapter, error) {
	return stub.adapter, nil
}

type channelContextReaderStub struct {
	pack     *channelruntime.ChannelContextPack
	request  channelruntime.ChannelContextRequest
	packs    []*channelruntime.ChannelContextPack
	requests []channelruntime.ChannelContextRequest
}

type channelContextNoReaderAdapterStub struct{}

func (channelContextNoReaderAdapterStub) Provider() string { return "dingtalk" }
func (channelContextNoReaderAdapterStub) Capabilities() channelruntime.ProviderCapabilities {
	return channelruntime.ProviderCapabilities{}
}
func (channelContextNoReaderAdapterStub) Verify(context.Context, channelruntime.WebhookRequest, channelruntime.ResolvedChannelCredential) (*channelruntime.VerifiedWebhook, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (channelContextNoReaderAdapterStub) Normalize(context.Context, *channelruntime.VerifiedWebhook, channelruntime.NormalizationContext) (*channelruntime.InboundMessage, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (channelContextNoReaderAdapterStub) Acknowledgement() channelruntime.WebhookResponse {
	return channelruntime.WebhookResponse{}
}
func (channelContextNoReaderAdapterStub) Deliver(context.Context, channelruntime.DeliveryRequest) (*channelruntime.DeliveryReceipt, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (channelContextNoReaderAdapterStub) Edit(context.Context, channelruntime.DeliveryRequest, string) (*channelruntime.DeliveryReceipt, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (channelContextNoReaderAdapterStub) Retract(context.Context, channelruntime.DeliveryRequest, string) error {
	return channelruntime.ErrUnsupportedEvent
}

func (*channelContextReaderStub) Provider() string { return "feishu" }
func (*channelContextReaderStub) Capabilities() channelruntime.ProviderCapabilities {
	return channelruntime.ProviderCapabilities{ContextRead: true}
}
func (*channelContextReaderStub) Verify(context.Context, channelruntime.WebhookRequest, channelruntime.ResolvedChannelCredential) (*channelruntime.VerifiedWebhook, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (*channelContextReaderStub) Normalize(context.Context, *channelruntime.VerifiedWebhook, channelruntime.NormalizationContext) (*channelruntime.InboundMessage, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (*channelContextReaderStub) Acknowledgement() channelruntime.WebhookResponse {
	return channelruntime.WebhookResponse{}
}
func (*channelContextReaderStub) Deliver(context.Context, channelruntime.DeliveryRequest) (*channelruntime.DeliveryReceipt, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (*channelContextReaderStub) Edit(context.Context, channelruntime.DeliveryRequest, string) (*channelruntime.DeliveryReceipt, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (*channelContextReaderStub) Retract(context.Context, channelruntime.DeliveryRequest, string) error {
	return channelruntime.ErrUnsupportedEvent
}
func (stub *channelContextReaderStub) ReadContext(_ context.Context, request channelruntime.ChannelContextRequest) (*channelruntime.ChannelContextPack, error) {
	stub.request = request
	stub.requests = append(stub.requests, request)
	if len(stub.packs) > 0 {
		pack := stub.packs[0]
		stub.packs = stub.packs[1:]
		return pack, nil
	}
	return stub.pack, nil
}

type channelContextModelResolverStub struct{ contextWindow int }

func (stub channelContextModelResolverStub) Resolve(context.Context, uuid.UUID, string, string) (chatruntime.ModelSpec, bool, error) {
	return chatruntime.ModelSpec{ContextWindow: stub.contextWindow}, true, nil
}
