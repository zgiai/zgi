package dingtalk

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

// dingTalkThinkingEmotion is DingTalk's built-in "thinking" text emotion. It
// stands in for Feishu's Typing reaction on a message being handled; the
// name is the label DingTalk clients show.
var dingTalkThinkingEmotion = map[string]any{
	"emotionType": 2,
	"emotionName": "🤔思考中",
	"textEmotion": map[string]string{"emotionId": "2659900", "emotionName": "🤔思考中", "text": "🤔思考中", "backgroundId": "im_bg_1"},
}

var _ channelruntime.ProcessingMarkerAdapter = (*ChannelAdapter)(nil)

// AddProcessingMarker puts the thinking emotion on the inbound message. The
// message and conversation IDs are copied from the verified bot callback.
func (adapter *ChannelAdapter) AddProcessingMarker(ctx context.Context, request channelruntime.ProcessingMarkerRequest) (*channelruntime.ProcessingMarkerReceipt, error) {
	response, err := adapter.sendEmotion(ctx, request, "/v1.0/robot/emotion/reply")
	if err != nil {
		return nil, err
	}
	return &channelruntime.ProcessingMarkerReceipt{ProviderRequestID: response.RequestID}, nil
}

// RemoveProcessingMarker takes the thinking emotion off again. DingTalk keeps
// one such emotion per bot and message, so it needs no marker ID.
func (adapter *ChannelAdapter) RemoveProcessingMarker(ctx context.Context, request channelruntime.ProcessingMarkerRequest, _ string) error {
	_, err := adapter.sendEmotion(ctx, request, "/v1.0/robot/emotion/recall")
	return err
}

func (adapter *ChannelAdapter) sendEmotion(ctx context.Context, request channelruntime.ProcessingMarkerRequest, path string) (dingTalkRobotSendResponse, error) {
	if adapter == nil || adapter.provider == nil || adapter.connections == nil || request.OrganizationID == uuid.Nil ||
		request.InstallationID == uuid.Nil || strings.TrimSpace(request.InboundMessageID) == "" ||
		strings.TrimSpace(request.InboundConversationID) == "" {
		return dingTalkRobotSendResponse{}, channelruntime.ErrInvalidWebhook
	}
	creds, destroy, err := adapter.channelCredentials(ctx, request.OrganizationID, request.WorkspaceID, request.InstallationID)
	if err != nil {
		return dingTalkRobotSendResponse{}, err
	}
	defer destroy()
	body := map[string]any{
		"robotCode": creds.AppKey, "openMsgId": strings.TrimSpace(request.InboundMessageID),
		"openConversationId": strings.TrimSpace(request.InboundConversationID),
	}
	for key, value := range dingTalkThinkingEmotion {
		body[key] = value
	}
	return adapter.sendRobotMessage(ctx, creds, path, body)
}
