package digitalhuman

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"gorm.io/datatypes"
)

type channelConnectorPreviewStub struct {
	*channelConnectorConfirmerStub
	fields []channelruntime.DeliveryCardField
	reads  int
}

func (stub *channelConnectorPreviewStub) ChannelConnectorConfirmationPreview(_ context.Context, _, _, _ uuid.UUID) ([]channelruntime.DeliveryCardField, error) {
	stub.reads++
	return stub.fields, nil
}

func connectorCardRun(t *testing.T, provider, conversationType string, receipt uuid.UUID) *Run {
	t.Helper()
	encoded, err := json.Marshal(invocation.Event{
		Origin:  invocation.Origin{Kind: invocation.OriginChannel, Provider: provider},
		Context: map[string]any{"conversation_type": conversationType},
	})
	if err != nil {
		t.Fatal(err)
	}
	actor := uuid.New()
	return &Run{ID: uuid.New(), OrganizationID: uuid.New(), ActorID: &actor, WaitingReceipt: &receipt, InvocationEvent: datatypes.JSON(encoded)}
}

func connectorCardMetadata(receipt uuid.UUID) map[string]interface{} {
	return map[string]interface{}{"skill_invocations": []interface{}{map[string]interface{}{
		"result": map[string]interface{}{"state": "requires_confirmation", "confirmation": map[string]interface{}{
			"route_receipt": receipt.String(), "connector_name": "Feishu / Lark",
			"connector_name_i18n": map[string]interface{}{"zh-Hans": "飞书"},
			"action_name":         "Send a text message",
			"action_name_i18n":    map[string]interface{}{"zh-Hans": "发送文本消息"},
			"risk_classification": "high",
			"consequence":         map[string]interface{}{"description_i18n": map[string]interface{}{"zh-Hans": "发送文本消息。"}},
		}},
	}}}
}

func TestChannelConnectorCardShowsTheOperationToItsDecider(t *testing.T) {
	receipt := uuid.New()
	preview := &channelConnectorPreviewStub{
		channelConnectorConfirmerStub: &channelConnectorConfirmerStub{},
		fields:                        []channelruntime.DeliveryCardField{{Label: "内容", Value: "smoke-1"}},
	}
	service := &RuntimeService{channelConnectors: preview}

	card := service.channelConnectorCard(t.Context(), connectorCardRun(t, "feishu", "direct", receipt), "", connectorCardMetadata(receipt))
	if card == nil || card.Title != "飞书 · 发送文本消息" || !card.HighRisk || card.Summary != "发送文本消息。" ||
		!card.Pending() || len(card.Fields) != 1 || card.Fields[0].Value != "smoke-1" || card.Notice != "" {
		t.Fatalf("direct card = %#v", card)
	}

	group := service.channelConnectorCard(t.Context(), connectorCardRun(t, "feishu", "group", receipt), "要发吗？", connectorCardMetadata(receipt))
	if group == nil || len(group.Fields) != 0 || group.Notice != channelConnectorGroupDetailsNotice || group.Summary != "要发吗？" {
		t.Fatalf("group card = %#v", group)
	}
	if preview.reads != 1 {
		t.Fatalf("preview reads = %d, a group must not read the details", preview.reads)
	}

	if card := service.channelConnectorCard(t.Context(), connectorCardRun(t, "dingtalk", "direct", receipt), "", connectorCardMetadata(receipt)); card != nil {
		t.Fatalf("a channel without cards got one: %#v", card)
	}
	if card := service.channelConnectorCard(t.Context(), connectorCardRun(t, "feishu", "direct", uuid.New()), "", connectorCardMetadata(receipt)); card != nil {
		t.Fatalf("another receipt's confirmation was shown: %#v", card)
	}
}

type channelEditAdapterFixture struct {
	*channelAdapterStub
	edit      bool
	editCalls int
	edited    string
	lastCard  *channelruntime.DeliveryCard
}

func (adapter *channelEditAdapterFixture) Capabilities() channelruntime.ProviderCapabilities {
	capabilities := adapter.channelAdapterStub.Capabilities()
	capabilities.Edit = adapter.edit
	return capabilities
}

func (adapter *channelEditAdapterFixture) Edit(_ context.Context, request channelruntime.DeliveryRequest, providerMessageID string) (*channelruntime.DeliveryReceipt, error) {
	adapter.editCalls++
	adapter.edited, adapter.lastCard = providerMessageID, request.Card
	return &channelruntime.DeliveryReceipt{ProviderMessageID: providerMessageID}, nil
}

type channelDeliveryLookupStore struct{ *channelStoreStub }

func (store channelDeliveryLookupStore) ChannelDeliveryByID(_ context.Context, id uuid.UUID) (*channelruntime.Delivery, error) {
	if delivery := store.deliveries[id]; delivery != nil {
		return delivery, nil
	}
	return nil, ErrChannelNotReady
}

func TestChannelDispatchEditsTheDeliveredPrompt(t *testing.T) {
	setup := func(t *testing.T, edit bool, promptStatus channelruntime.DeliveryStatus) (*ChannelService, *channelEditAdapterFixture, *channelruntime.Delivery, *channelruntime.Delivery) {
		service, store, _, stub, scope, application := newChannelTestService(t)
		installation, route := createChannelTestRoute(t, service, scope, application, channelruntime.ConversationTypeDirect)
		adapter := &channelEditAdapterFixture{channelAdapterStub: stub, edit: edit}
		registry, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: adapter, Provisioner: channelProvisionerStub{}})
		if err != nil {
			t.Fatal(err)
		}
		service.adapters, service.store = registry, channelDeliveryLookupStore{store}
		now := time.Now().UTC()
		prompt := newChannelTestDelivery(t, scope, application, installation, route, now)
		prompt.Status, prompt.ProviderMessageID = promptStatus, "om_card"
		// A prompt still waiting to be sent is not due in this dispatch.
		prompt.AvailableAt = now.Add(time.Hour)
		card := &channelruntime.DeliveryCard{Title: "飞书 · 发送文本消息", State: channelruntime.DeliveryCardCompleted, Outcome: "已确认并执行完成。"}
		promptID := prompt.ID
		update := newChannelTestDelivery(t, scope, application, installation, route, now)
		update.RunID = prompt.RunID
		update.Payload, update.PayloadDigest, err = channelDeliveryPayloadData{
			Kind: channelruntime.DeliveryKindFinal, Text: card.Outcome, Card: card, EditOf: &promptID,
		}.encode()
		if err != nil {
			t.Fatal(err)
		}
		store.deliveries[prompt.ID], store.deliveries[update.ID] = prompt, update
		service.now = func() time.Time { return now }
		return service, adapter, prompt, update
	}

	service, adapter, _, update := setup(t, true, channelruntime.DeliverySucceeded)
	if err := service.DispatchDeliveries(t.Context(), 10); err != nil {
		t.Fatal(err)
	}
	if adapter.editCalls != 1 || adapter.edited != "om_card" || adapter.lastCard == nil || adapter.lastCard.State != channelruntime.DeliveryCardCompleted ||
		adapter.deliveryCalls != 0 || update.Status != channelruntime.DeliverySucceeded {
		t.Fatalf("edit calls=%d edited=%q sends=%d update=%s", adapter.editCalls, adapter.edited, adapter.deliveryCalls, update.Status)
	}

	service, adapter, _, update = setup(t, true, channelruntime.DeliveryPending)
	if err := service.DispatchDeliveries(t.Context(), 10); err != nil {
		t.Fatal(err)
	}
	if adapter.editCalls != 0 || update.Status != channelruntime.DeliveryPending || update.ErrorCode != "channel.delivery.edit_waiting" {
		t.Fatalf("an update ran before its prompt was sent: calls=%d status=%s code=%s", adapter.editCalls, update.Status, update.ErrorCode)
	}

	service, adapter, _, update = setup(t, false, channelruntime.DeliverySucceeded)
	if err := service.DispatchDeliveries(t.Context(), 10); err != nil {
		t.Fatal(err)
	}
	if adapter.editCalls != 0 || adapter.deliveryCalls != 0 || update.Status != channelruntime.DeliveryFailed || update.ErrorCode != "channel.delivery.edit_unsupported" {
		t.Fatalf("a channel without edits: calls=%d sends=%d status=%s code=%s", adapter.editCalls, adapter.deliveryCalls, update.Status, update.ErrorCode)
	}
}

func TestChannelConnectorCardStateNamesTheOutcome(t *testing.T) {
	for state, want := range map[string]channelruntime.DeliveryCardState{
		"completed": channelruntime.DeliveryCardCompleted, "cancelled": channelruntime.DeliveryCardCancelled,
		"failed": channelruntime.DeliveryCardFailed, "unknown": channelruntime.DeliveryCardUnknown,
	} {
		if got, outcome := channelConnectorCardState(state, ""); got != want || outcome == "" {
			t.Fatalf("%s = %s, %q", state, got, outcome)
		}
	}
}
