package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

const websocketEventFixture = `{"schema":"2.0","header":{"event_id":"event-1","app_id":"cli_test","event_type":"im.message.receive_v1","tenant_key":"tenant-1","create_time":"1787116200000"},"event":{"sender":{"sender_id":{"open_id":"ou_sender"}},"message":{"message_id":"message-1","create_time":"1787116200000","chat_id":"oc_group","chat_type":"group","message_type":"text","parent_id":"parent-1","content":"{\"text\":\"@_user_1 hello\"}","mentions":[{"key":"@_user_1","id":{"open_id":"ou_bot"},"name":"Bot"}]}}}`

func TestFeishuConnectionModesAndMigration(t *testing.T) {
	adapter := newWebhookTestAdapter()
	receiver := NewWebSocketReceiver(adapter)
	legacy := map[string]string{"app_id": "cli_test", "app_secret": "secret", "verification_token": "verify"}
	if mode := receiver.Mode(channelruntime.ResolvedChannelCredential{Values: legacy}); mode != "webhook" {
		t.Fatalf("legacy mode = %q, want webhook", mode)
	}
	if mode := receiver.Mode(channelruntime.ResolvedChannelCredential{
		Values: map[string]string{"app_id": "cli_test", "app_secret": "secret"},
		Source: channelruntime.CredentialSourcePersonalConnection,
	}); mode != "websocket" {
		t.Fatalf("connector mode = %q, want websocket", mode)
	}
	for _, test := range []struct {
		name             string
		current, updates map[string]string
		mode             string
		invalid          bool
	}{
		{name: "new defaults to websocket", updates: map[string]string{"app_id": "cli_test", "app_secret": "secret"}, mode: "websocket"},
		{name: "old client creating webhook", updates: legacy, mode: "webhook"},
		{name: "legacy update preserves webhook", current: legacy, updates: map[string]string{"app_secret": "rotated"}, mode: "webhook"},
		{name: "explicit switch keeps secrets", current: legacy, updates: map[string]string{"connection_mode": "websocket"}, mode: "websocket"},
		{name: "invalid mode rejected", current: legacy, updates: map[string]string{"connection_mode": "unsupported"}, invalid: true},
		{name: "webhook needs token", updates: map[string]string{"app_id": "cli_test", "app_secret": "secret", "connection_mode": "webhook"}, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			prepared, err := adapter.PrepareCredentials(test.current, test.updates)
			if (err != nil) != test.invalid {
				t.Fatalf("PrepareCredentials() error = %v, want invalid %v", err, test.invalid)
			}
			if !test.invalid && prepared["connection_mode"] != test.mode {
				t.Fatalf("mode = %q, want %q", prepared["connection_mode"], test.mode)
			}
		})
	}
	prepared, err := adapter.PrepareCredentials(legacy, map[string]string{"connection_mode": "websocket"})
	if err != nil {
		t.Fatal(err)
	}
	if prepared["verification_token"] != "verify" || legacy["connection_mode"] != "" {
		t.Fatal("switch lost rollback credential or mutated caller")
	}
	_, err = adapter.Verify(t.Context(), channelruntime.WebhookRequest{Body: []byte(`{"challenge":"x","token":"verify","type":"url_verification"}`)}, channelruntime.ResolvedChannelCredential{Values: prepared})
	if !errors.Is(err, channelruntime.ErrWebhookUnauthorized) {
		t.Fatalf("old webhook accepted in websocket mode: %v", err)
	}
	rollback, err := adapter.PrepareCredentials(prepared, map[string]string{"connection_mode": "webhook"})
	if err != nil || rollback["verification_token"] != "verify" {
		t.Fatalf("rollback failed: %v", err)
	}
}

func TestFeishuWebSocketNormalizesAndChecksIdentity(t *testing.T) {
	receiver := NewWebSocketReceiver(newWebhookTestAdapter())
	identity := channelruntime.InstallationIdentity{ExternalTenantID: "tenant-1", ExternalSelfID: "ou_bot"}
	for _, test := range []struct {
		name, payload     string
		rejected, ignored bool
		wake              channelruntime.WakeReason
	}{
		{name: "group mention", payload: websocketEventFixture, wake: channelruntime.WakeReasonMention},
		{name: "private message", payload: strings.ReplaceAll(websocketEventFixture, `"chat_type":"group"`, `"chat_type":"p2p"`), wake: channelruntime.WakeReasonDirectMessage},
		{name: "wrong app", payload: strings.ReplaceAll(websocketEventFixture, "cli_test", "cli_other"), rejected: true},
		{name: "wrong tenant", payload: strings.ReplaceAll(websocketEventFixture, "tenant-1", "tenant-other"), rejected: true},
		{name: "unsupported image", payload: strings.ReplaceAll(websocketEventFixture, `"message_type":"text"`, `"message_type":"image"`), ignored: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var event larkim.P2MessageReceiveV1
			if err := json.Unmarshal([]byte(test.payload), &event); err != nil {
				t.Fatal(err)
			}
			var received *channelruntime.InboundMessage
			err := receiver.handleEvent(t.Context(), identity, "cli_test", &event, func(ctx context.Context, message *channelruntime.InboundMessage) error {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 2500*time.Millisecond {
					t.Error("inbound handler has no bounded acknowledgement deadline")
				}
				received = message
				return nil
			})
			if (err != nil) != test.rejected {
				t.Fatalf("handle error = %v", err)
			}
			if test.rejected || test.ignored {
				if received != nil {
					t.Fatal("rejected event entered channel runtime")
				}
				return
			}
			if received == nil || received.Text != "hello" || received.SenderExternalID != "ou_sender" || received.ParentMessageID != "parent-1" || received.WakeReason != test.wake {
				t.Fatalf("normalized message = %#v", received)
			}
		})
	}
}

func TestFeishuWebSocketRejectsNonDurableAcceptance(t *testing.T) {
	receiver := NewWebSocketReceiver(newWebhookTestAdapter())
	var event larkim.P2MessageReceiveV1
	if err := json.Unmarshal([]byte(websocketEventFixture), &event); err != nil {
		t.Fatal(err)
	}
	err := receiver.handleEvent(t.Context(), channelruntime.InstallationIdentity{ExternalTenantID: "tenant-1", ExternalSelfID: "ou_bot"}, "cli_test", &event,
		func(context.Context, *channelruntime.InboundMessage) error {
			return errors.New("database failed with sensitive details")
		})
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("durability failure must reject the event without leaking the cause: %v", err)
	}
}

func TestFeishuWebSocketSDKReconnectAcknowledgementAndShutdown(t *testing.T) {
	var connections atomic.Int32
	acknowledged := make(chan int, 1)
	reconnected := make(chan struct{}, 1)
	var mu sync.Mutex
	var states []channelruntime.ReceiverConnectionState
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/callback/ws/endpoint", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(larkws.EndpointResp{Data: &larkws.Endpoint{
			Url:          "ws" + strings.TrimPrefix(server.URL, "http") + "/socket?device_id=test&service_id=1",
			ClientConfig: &larkws.ClientConfig{ReconnectCount: -1, ReconnectInterval: 1, PingInterval: 3600},
		}})
	})
	mux.HandleFunc("/socket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		index := connections.Add(1)
		if index == 1 {
			frame := larkws.Frame{Method: int32(larkws.FrameTypeData), Payload: []byte(websocketEventFixture), Headers: []larkws.Header{
				{Key: "type", Value: "event"}, {Key: "message_id", Value: "frame-1"}, {Key: "sum", Value: "1"}, {Key: "seq", Value: "0"},
			}}
			body, err := frame.Marshal()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, body); err != nil {
				return
			}
		}
		for {
			_, body, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var frame larkws.Frame
			if frame.Unmarshal(body) != nil || frame.Method != int32(larkws.FrameTypeData) {
				continue
			}
			var response struct {
				Code int `json:"code"`
			}
			if json.Unmarshal(frame.Payload, &response) != nil {
				return
			}
			acknowledged <- response.Code
			return // Drop the first socket only after its durable acknowledgement.
		}
	})
	receiver := NewWebSocketReceiver(newWebhookTestAdapter())
	receiver.options = []larkws.ClientOption{larkws.WithDomain(server.URL)}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	var accepted atomic.Int32
	go func() {
		finished <- receiver.Run(ctx, channelruntime.ReceiverRequest{
			Credential: channelruntime.ResolvedChannelCredential{Values: map[string]string{"app_id": "cli_test", "app_secret": "secret", "connection_mode": "websocket"}},
			Identity:   channelruntime.InstallationIdentity{ExternalTenantID: "tenant-1", ExternalSelfID: "ou_bot"},
			OnStateChange: func(state channelruntime.ReceiverConnectionState) {
				mu.Lock()
				states = append(states, state)
				mu.Unlock()
				if state == channelruntime.ReceiverStateConnected && connections.Load() >= 2 {
					select {
					case reconnected <- struct{}{}:
					default:
					}
				}
			},
		}, func(context.Context, *channelruntime.InboundMessage) error { accepted.Add(1); return nil })
	}()
	select {
	case code := <-acknowledged:
		if code != 200 {
			t.Errorf("acknowledgement = %d", code)
		}
	case <-ctx.Done():
		t.Fatal("no SDK acknowledgement")
	}
	select {
	case <-reconnected:
	case <-ctx.Done():
		t.Fatal("SDK did not reconnect")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("shutdown error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SDK shutdown did not join its workers")
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(states, channelruntime.ReceiverStateReconnecting) || accepted.Load() != 1 || connections.Load() != 2 {
		t.Fatalf("states=%v accepted=%d connections=%d", states, accepted.Load(), connections.Load())
	}
}

func TestFeishuWebSocketSDKPermanentFailureIsNotRetried(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(larkws.EndpointResp{Code: 19001, Msg: "invalid secret must not be exposed"})
	}))
	defer server.Close()
	receiver := NewWebSocketReceiver(newWebhookTestAdapter())
	receiver.options = []larkws.ClientOption{larkws.WithDomain(server.URL)}
	err := receiver.Run(t.Context(), channelruntime.ReceiverRequest{
		Credential: channelruntime.ResolvedChannelCredential{Values: map[string]string{"app_id": "cli_test", "app_secret": "secret", "connection_mode": "websocket"}},
		Identity:   channelruntime.InstallationIdentity{ExternalTenantID: "tenant-1", ExternalSelfID: "ou_bot"},
	}, func(context.Context, *channelruntime.InboundMessage) error { return nil })
	var failure *channelruntime.ReceiverError
	if !errors.As(err, &failure) || failure.Retryable || failure.Code != "credentials_invalid" || requests.Load() != 1 || strings.Contains(err.Error(), "secret") {
		t.Fatalf("failure=%v requests=%d", err, requests.Load())
	}
}
