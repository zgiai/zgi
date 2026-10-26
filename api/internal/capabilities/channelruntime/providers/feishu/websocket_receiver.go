package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
)

// WebSocketReceiver owns a Feishu SDK connection, not a second message pipeline.
// Messages enter the same durable, idempotent channel service as webhooks.
type WebSocketReceiver struct {
	adapter *ChannelAdapter
	options []larkws.ClientOption
}

var _ channelruntime.Receiver = (*WebSocketReceiver)(nil)

func NewWebSocketReceiver(adapter *ChannelAdapter) *WebSocketReceiver {
	if adapter == nil {
		panic("Feishu WebSocket receiver requires channel adapter")
	}
	return &WebSocketReceiver{adapter: adapter}
}

func feishuConnectionMode(credential channelruntime.ResolvedChannelCredential) string {
	if mode := strings.TrimSpace(credential.Values["connection_mode"]); mode != "" {
		return mode
	}
	// Connector-backed Feishu credentials do not store channel transport
	// metadata. They are the new long-connection path; legacy embedded
	// credentials without an explicit mode retain the webhook fallback.
	if credential.Source == channelruntime.CredentialSourcePersonalConnection {
		return "websocket"
	}
	return "webhook"
}

func (*WebSocketReceiver) Mode(credential channelruntime.ResolvedChannelCredential) string {
	return feishuConnectionMode(credential)
}

func (*WebSocketReceiver) LeaseScope(credential channelruntime.ResolvedChannelCredential) string {
	return "feishu:websocket:" + strings.TrimSpace(credential.Values["app_id"])
}

func (receiver *WebSocketReceiver) Run(ctx context.Context, request channelruntime.ReceiverRequest, handle channelruntime.InboundMessageHandler) error {
	appID := strings.TrimSpace(request.Credential.Values["app_id"])
	appSecret := strings.TrimSpace(request.Credential.Values["app_secret"])
	if handle == nil || appID == "" || appSecret == "" || receiver.Mode(request.Credential) != "websocket" {
		return &channelruntime.ReceiverError{Code: "credentials_invalid"}
	}
	identity := request.Identity
	if identity.ExternalSelfID == "" || identity.ExternalTenantID == "" {
		identityCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		var err error
		identity, err = receiver.adapter.ResolveIdentity(identityCtx, request.Credential)
		cancel()
		if err != nil {
			return feishuReceiverError(err)
		}
	}
	state := func(next channelruntime.ReceiverConnectionState) {
		if request.OnStateChange != nil {
			request.OnStateChange(next)
		}
	}
	connected := func() {
		state(channelruntime.ReceiverStateConnected)
		if request.OnConnected != nil {
			request.OnConnected()
		}
	}
	events := dispatcher.NewEventDispatcher("", "").OnP2MessageReceiveV1(
		func(eventCtx context.Context, event *larkim.P2MessageReceiveV1) error {
			return receiver.handleEvent(eventCtx, identity, appID, event, handle)
		},
	).OnP2CardActionTrigger(
		func(eventCtx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
			return receiver.handleCardAction(eventCtx, identity, appID, event, handle)
		},
	)
	options := []larkws.ClientOption{
		larkws.WithDomain(feishuWebSocketDomain(request.Credential)),
		larkws.WithEventHandler(events), larkws.WithAutoReconnect(true),
		larkws.WithLogLevel(larkcore.LogLevelError),
		larkws.WithOnReady(connected), larkws.WithOnReconnected(connected),
		larkws.WithOnReconnecting(func() { state(channelruntime.ReceiverStateReconnecting) }),
		larkws.WithOnDisconnected(func() { state(channelruntime.ReceiverStateDisconnected) }),
	}
	options = append(options, receiver.options...)
	client := larkws.NewClient(appID, appSecret, options...)
	// Start blocks until all SDK workers exit on cancellation, including event
	// handlers. The manager retains the credential and lease until it returns.
	err := client.Start(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return feishuReceiverError(err)
	}
	return nil
}

// feishuWebSocketDomain follows the same region values as the HTTP adapter:
// Lark (international) connections must dial the Lark endpoint.
func feishuWebSocketDomain(credential channelruntime.ResolvedChannelCredential) string {
	switch strings.ToLower(strings.TrimSpace(credential.Values["region"])) {
	case "lark", RegionGlobal:
		return lark.LarkBaseUrl
	default:
		return lark.FeishuBaseUrl
	}
}

func (receiver *WebSocketReceiver) handleEvent(ctx context.Context, identity channelruntime.InstallationIdentity, appID string, event *larkim.P2MessageReceiveV1, handle channelruntime.InboundMessageHandler) error {
	if event == nil || event.EventV2Base == nil || event.EventV2Base.Header == nil ||
		event.EventV2Base.Header.AppID != appID || event.TenantKey() != identity.ExternalTenantID {
		return channelruntime.ErrWebhookUnauthorized
	}
	body, err := json.Marshal(struct {
		*larkevent.EventV2Base
		Event *larkim.P2MessageReceiveV1Data `json:"event"`
	}{EventV2Base: event.EventV2Base, Event: event.Event})
	if err != nil {
		return channelruntime.ErrInvalidWebhook
	}
	message, err := receiver.adapter.Normalize(ctx, &channelruntime.VerifiedWebhook{Body: body}, channelruntime.NormalizationContext{Identity: identity})
	if errors.Is(err, channelruntime.ErrUnsupportedEvent) {
		return nil // Unsupported message types must not create a retry storm.
	}
	if err != nil {
		return err
	}
	// Feishu expects acknowledgement within 3 seconds. This boundary only
	// persists/queues the request; it never waits for a model or final delivery.
	messageCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	if err := handle(messageCtx, message); err != nil {
		// Do not let the SDK log arbitrary database/provider errors with secrets.
		return errors.New("channel message was not durably accepted")
	}
	return nil
}

func feishuReceiverError(err error) *channelruntime.ReceiverError {
	result := &channelruntime.ReceiverError{Code: "connection_failed", Retryable: true, Cause: err}
	var clientError *larkws.ClientError
	if errors.As(err, &clientError) {
		result.Code, result.Retryable = "configuration_invalid", false
		if clientError.Code == 19001 {
			result.Code = "credentials_invalid"
		}
		if clientError.Code == larkws.AuthFailed || clientError.Code == larkws.ExceedConnLimit {
			result.Code = "connection_limit"
		}
		return result
	}
	switch providertransport.ErrorCode(err) {
	case providertransport.ErrorCodeAuthInvalid, providertransport.ErrorCodeConnectionExpired, providertransport.ErrorCodeReconnectRequired:
		result.Code, result.Retryable = "credentials_invalid", false
	case providertransport.ErrorCodeAccessDenied, providertransport.ErrorCodeInsufficientScope:
		result.Code, result.Retryable = "permission_missing", false
	case providertransport.ErrorCodeInvalidInput, providertransport.ErrorCodeConnectionInvalid:
		result.Code, result.Retryable = "configuration_invalid", false
	}
	return result
}
