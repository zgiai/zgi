package dingtalk

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	streamclient "github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/utils"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// dingTalkHandlerTimeout bounds how long a bot message waits to be stored.
// DingTalk sends each bot message once and does not resend it, and the SDK
// acknowledges only after the handler returns, so the handler only queues
// the message.
const dingTalkHandlerTimeout = 2500 * time.Millisecond

// StreamReceiver owns DingTalk's outbound WebSocket connection while reusing
// ChannelAdapter for protocol normalization.
type StreamReceiver struct {
	adapter *ChannelAdapter
}

var _ channelruntime.Receiver = (*StreamReceiver)(nil)

func NewStreamReceiver(adapter *ChannelAdapter) *StreamReceiver {
	if adapter == nil {
		panic("DingTalk stream receiver requires channel adapter")
	}
	return &StreamReceiver{adapter: adapter}
}

func (*StreamReceiver) Mode(channelruntime.ResolvedChannelCredential) string { return "stream" }

// LeaseScope keeps one Stream connection per DingTalk app: DingTalk pushes
// each bot message to one randomly chosen connection of the app, so a second
// one, for another installation or environment, would take its messages.
func (*StreamReceiver) LeaseScope(credential channelruntime.ResolvedChannelCredential) string {
	return "dingtalk:stream:" + strings.TrimSpace(credential.Values["app_key"])
}

func (receiver *StreamReceiver) Run(
	ctx context.Context,
	request channelruntime.ReceiverRequest,
	handle channelruntime.InboundMessageHandler,
) error {
	if receiver == nil || receiver.adapter == nil || handle == nil || !validDingTalkChannelCredentials(request.Credential.Values) {
		return &channelruntime.ReceiverError{Code: "credentials_invalid"}
	}
	appKey := strings.TrimSpace(request.Credential.Values["app_key"])
	appSecret := strings.TrimSpace(request.Credential.Values["app_secret"])
	state := func(next channelruntime.ReceiverConnectionState) {
		if request.OnStateChange != nil {
			request.OnStateChange(next)
		}
	}
	// The SDK reconnects on its own and reports nothing. A disconnect frame
	// marks the connection reconnecting until DingTalk pings it again.
	var reconnecting atomic.Bool
	var client *streamclient.StreamClient
	client = streamclient.NewStreamClient(
		streamclient.WithAppCredential(streamclient.NewAppCredentialConfig(appKey, appSecret)),
		streamclient.WithAutoReconnect(true),
		streamclient.WithSubscription(utils.SubscriptionTypeKSystem, "disconnect", func(frameCtx context.Context, frame *payload.DataFrame) (*payload.DataFrameResponse, error) {
			reconnecting.Store(true)
			state(channelruntime.ReceiverStateReconnecting)
			return client.OnDisconnect(frameCtx, frame)
		}),
		streamclient.WithSubscription(utils.SubscriptionTypeKSystem, "ping", func(frameCtx context.Context, frame *payload.DataFrame) (*payload.DataFrameResponse, error) {
			if reconnecting.CompareAndSwap(true, false) {
				state(channelruntime.ReceiverStateConnected)
			}
			return client.OnPing(frameCtx, frame)
		}),
	)
	client.RegisterChatBotCallbackRouter(func(callbackCtx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
		return receiver.handleMessage(callbackCtx, dingTalkReceiverIdentity(request.Identity, appKey), data, handle)
	})
	if err := client.Start(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return dingTalkReceiverError(err)
	}
	state(channelruntime.ReceiverStateConnected)
	if request.OnConnected != nil {
		request.OnConnected()
	}
	<-ctx.Done()

	client.AutoReconnect = false
	client.Close()
	return ctx.Err()
}

// dingTalkReceiverIdentity names the bot by its app key when the installation
// has not recorded it, as one set up by QR registration: the app key is the
// bot's robotCode, which tells a message mentioning the bot.
func dingTalkReceiverIdentity(identity channelruntime.InstallationIdentity, appKey string) channelruntime.InstallationIdentity {
	if strings.TrimSpace(identity.ExternalSelfID) == "" {
		identity.ExternalSelfID = appKey
	}
	return identity
}

// handleMessage queues one bot message. Messages the channel does not take,
// such as other message types, are acknowledged so they are not reported as
// failures, and errors never carry provider or database text to the SDK log.
func (receiver *StreamReceiver) handleMessage(ctx context.Context, identity channelruntime.InstallationIdentity, data *chatbot.BotCallbackDataModel, handle channelruntime.InboundMessageHandler) ([]byte, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, channelruntime.ErrInvalidWebhook
	}
	message, err := receiver.adapter.Normalize(ctx, &channelruntime.VerifiedWebhook{Body: body}, channelruntime.NormalizationContext{Identity: identity})
	if errors.Is(err, ErrSenderUnpublishedBot) {
		logger.Warn("DingTalk bot message without a userId was ignored; publish the bot", "app_key", identity.ExternalSelfID)
		return []byte("{}"), nil
	}
	if errors.Is(err, channelruntime.ErrUnsupportedEvent) {
		return []byte("{}"), nil
	}
	if err != nil {
		return nil, channelruntime.ErrInvalidWebhook
	}
	messageCtx, cancel := context.WithTimeout(ctx, dingTalkHandlerTimeout)
	defer cancel()
	if err := handle(messageCtx, message); err != nil {
		return nil, errors.New("channel message was not durably accepted")
	}
	return []byte("{}"), nil
}

// dingTalkReceiverError maps a failed connection to a code the console can
// explain. The open-connection call answers bad credentials with a client
// error body.
func dingTalkReceiverError(err error) *channelruntime.ReceiverError {
	result := &channelruntime.ReceiverError{Code: "connection_failed", Retryable: true, Cause: err}
	switch providertransport.ErrorCode(err) {
	case providertransport.ErrorCodeAuthInvalid, providertransport.ErrorCodeConnectionExpired:
		result.Code, result.Retryable = "credentials_invalid", false
		return result
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "clientid") || strings.Contains(text, "clientsecret") || strings.Contains(text, "auth") ||
		strings.Contains(text, "invalid") || strings.Contains(text, "forbidden") || strings.Contains(text, "unauthorized") {
		result.Code, result.Retryable = "credentials_invalid", false
	}
	return result
}
