package dingtalk

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	streamclient "github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

// StreamReceiver owns DingTalk's outbound WebSocket connection while reusing
// ChannelAdapter for protocol normalization.
type StreamReceiver struct {
	adapter *ChannelAdapter
}

func NewStreamReceiver(adapter *ChannelAdapter) *StreamReceiver {
	if adapter == nil {
		panic("DingTalk stream receiver requires channel adapter")
	}
	return &StreamReceiver{adapter: adapter}
}

func (*StreamReceiver) Mode(channelruntime.ResolvedChannelCredential) string { return "stream" }

func (receiver *StreamReceiver) Run(
	ctx context.Context,
	request channelruntime.ReceiverRequest,
	handle channelruntime.InboundMessageHandler,
) error {
	if receiver == nil || receiver.adapter == nil || handle == nil || !validDingTalkChannelCredentials(request.Credential.Values) {
		return integrations.NewError(integrations.ErrorCodeConnectionInvalid, "DingTalk Stream credentials are unavailable", nil)
	}
	appKey := strings.TrimSpace(request.Credential.Values["app_key"])
	appSecret := strings.TrimSpace(request.Credential.Values["app_secret"])
	client := streamclient.NewStreamClient(
		streamclient.WithAppCredential(streamclient.NewAppCredentialConfig(appKey, appSecret)),
		streamclient.WithAutoReconnect(true),
	)
	client.RegisterChatBotCallbackRouter(func(callbackCtx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
		body, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		message, err := receiver.adapter.Normalize(callbackCtx, &channelruntime.VerifiedWebhook{Body: body}, channelruntime.NormalizationContext{
			Identity: request.Identity,
		})
		if err != nil {
			return nil, err
		}
		if err := handle(callbackCtx, message); err != nil {
			return nil, err
		}
		return []byte("{}"), nil
	})
	if err := client.Start(ctx); err != nil {
		return err
	}
	if request.OnConnected != nil {
		request.OnConnected()
	}
	<-ctx.Done()
	// Disable the SDK's automatic reconnect before closing so shutdown cannot
	// create an orphan connection after the installation is disabled.
	client.AutoReconnect = false
	client.Close()
	return ctx.Err()
}
