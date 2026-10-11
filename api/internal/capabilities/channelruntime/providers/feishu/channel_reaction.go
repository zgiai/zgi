package feishu

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
)

const feishuProcessingEmoji = "Typing"

type feishuMessageReaction struct {
	ReactionID string `json:"reaction_id"`
	Operator   struct {
		OperatorID   string `json:"operator_id"`
		OperatorType string `json:"operator_type"`
	} `json:"operator"`
	ReactionType struct {
		EmojiType string `json:"emoji_type"`
	} `json:"reaction_type"`
}

// AddProcessingMarker shows Feishu's native Typing reaction on the inbound
// message. The message ID is copied from the verified provider event and is
// never selected by the model.
func (adapter *ChannelAdapter) AddProcessingMarker(ctx context.Context, request channelruntime.ProcessingMarkerRequest) (*channelruntime.ProcessingMarkerReceipt, error) {
	if !adapter.directProviderAvailable() || request.OrganizationID == uuid.Nil || request.WorkspaceID == uuid.Nil ||
		request.InstallationID == uuid.Nil || request.ConnectionID == uuid.Nil || request.RunID == uuid.Nil ||
		strings.TrimSpace(request.InboundMessageID) == "" {
		return nil, channelruntime.ErrInvalidWebhook
	}
	connection, cleanup, err := adapter.resolveDirectConnection(ctx, request.OrganizationID, request.WorkspaceID, request.InstallationID)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	region, err := feishuRegion(connection)
	if err != nil {
		return nil, err
	}
	token, tokenMeta, err := adapter.provider.tenantAccessToken(ctx, connection, region)
	if err != nil {
		return nil, err
	}
	var reaction feishuMessageReaction
	meta, err := adapter.provider.client.addMessageReaction(ctx, region, token, strings.TrimSpace(request.InboundMessageID), map[string]any{
		"reaction_type": map[string]string{"emoji_type": feishuProcessingEmoji},
	}, &reaction)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(reaction.ReactionID) == "" {
		return nil, providertransport.NewError(providertransport.ErrorCodeResponseInvalid, "Feishu reaction response is incomplete", nil)
	}
	return &channelruntime.ProcessingMarkerReceipt{
		ProviderMarkerID:  strings.TrimSpace(reaction.ReactionID),
		ProviderRequestID: firstNonEmpty(meta.RequestID, tokenMeta.RequestID),
	}, nil
}

// RemoveProcessingMarker removes only the app-owned Typing reactions. Listing
// before deletion makes cleanup safe after an ambiguous add or worker retry.
func (adapter *ChannelAdapter) RemoveProcessingMarker(ctx context.Context, request channelruntime.ProcessingMarkerRequest, providerMarkerID string) error {
	if !adapter.directProviderAvailable() || request.OrganizationID == uuid.Nil || request.WorkspaceID == uuid.Nil ||
		request.InstallationID == uuid.Nil || request.ConnectionID == uuid.Nil || request.RunID == uuid.Nil ||
		strings.TrimSpace(request.InboundMessageID) == "" {
		return channelruntime.ErrInvalidWebhook
	}
	connection, cleanup, err := adapter.resolveDirectConnection(ctx, request.OrganizationID, request.WorkspaceID, request.InstallationID)
	if err != nil {
		return err
	}
	defer cleanup()
	region, err := feishuRegion(connection)
	if err != nil {
		return err
	}
	token, _, err := adapter.provider.tenantAccessToken(ctx, connection, region)
	if err != nil {
		return err
	}
	appID := feishuConnectionAppID(connection)
	pageToken := ""
	for page := 0; page < 10; page++ {
		query := url.Values{"reaction_type": []string{feishuProcessingEmoji}, "page_size": []string{"50"}}
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}
		var response struct {
			Items     []feishuMessageReaction `json:"items"`
			HasMore   bool                    `json:"has_more"`
			PageToken string                  `json:"page_token"`
		}
		if _, err := adapter.provider.client.listMessageReactions(ctx, region, token, strings.TrimSpace(request.InboundMessageID), query, &response); err != nil {
			return err
		}
		for _, reaction := range response.Items {
			ownedByApp := strings.EqualFold(strings.TrimSpace(reaction.Operator.OperatorType), "app") &&
				strings.TrimSpace(reaction.Operator.OperatorID) == appID
			matchesSavedID := strings.TrimSpace(providerMarkerID) != "" && reaction.ReactionID == strings.TrimSpace(providerMarkerID)
			if !ownedByApp && !matchesSavedID {
				continue
			}
			if _, err := adapter.provider.client.deleteMessageReaction(ctx, region, token, strings.TrimSpace(request.InboundMessageID), strings.TrimSpace(reaction.ReactionID), nil); err != nil {
				return err
			}
		}
		if !response.HasMore || strings.TrimSpace(response.PageToken) == "" {
			return nil
		}
		pageToken = strings.TrimSpace(response.PageToken)
	}
	return providertransport.NewError(providertransport.ErrorCodeResponseInvalid, "Feishu reaction pagination did not terminate", nil)
}

func (c *client) addMessageReaction(ctx context.Context, region, accessToken, messageID string, body, target interface{}) (responseMeta, error) {
	path := "/open-apis/im/v1/messages/" + url.PathEscape(messageID) + "/reactions"
	return c.doEnvelope(ctx, region, http.MethodPost, path, nil, body, accessToken, false, target)
}

func (c *client) listMessageReactions(ctx context.Context, region, accessToken, messageID string, query url.Values, target interface{}) (responseMeta, error) {
	path := "/open-apis/im/v1/messages/" + url.PathEscape(messageID) + "/reactions"
	return c.doEnvelope(ctx, region, http.MethodGet, path, query, nil, accessToken, true, target)
}

func (c *client) deleteMessageReaction(ctx context.Context, region, accessToken, messageID, reactionID string, target interface{}) (responseMeta, error) {
	path := "/open-apis/im/v1/messages/" + url.PathEscape(messageID) + "/reactions/" + url.PathEscape(reactionID)
	return c.doEnvelope(ctx, region, http.MethodDelete, path, nil, nil, accessToken, false, target)
}
