package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
)

// Feishu's file upload API excludes empty files and caps each file at 30 MB.
const maxChannelFileBytes int64 = 30_000_000

func (adapter *ChannelAdapter) deliverWithAttachments(ctx context.Context, region, token string, request channelruntime.DeliveryRequest) (*channelruntime.DeliveryReceipt, error) {
	if !validFeishuToken(request.ExternalConversationID) || len(request.Attachments) > 8 {
		return nil, channelruntime.ErrInvalidWebhook
	}
	for _, file := range request.Attachments {
		if file.ID == "" || file.Open == nil || strings.TrimSpace(file.Filename) == "" || file.Size <= 0 || file.Size > maxChannelFileBytes {
			return nil, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "Feishu channel file is invalid or exceeds the upload limit", nil)
		}
	}
	// Upload before sending any message. Upload failures are retryable without
	// duplicating user-visible messages; uploaded resources do not send themselves.
	keys := make([]string, 0, len(request.Attachments))
	for _, file := range request.Attachments {
		key, err := adapter.provider.client.uploadChannelFile(ctx, region, token, file)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	var receipt channelruntime.DeliveryReceipt
	sent := false
	if strings.TrimSpace(request.Text) != "" {
		id, meta, err := adapter.sendChannelMessage(ctx, region, token, request)
		if err != nil {
			return nil, channelFileSendError(err, false)
		}
		if id == "" {
			return nil, channelruntime.ErrDeliveryOutcomeUnknown
		}
		sent = true
		receipt = channelruntime.DeliveryReceipt{ProviderMessageID: id, ProviderRequestID: meta.RequestID}
	}
	for i, key := range keys {
		content, err := json.Marshal(map[string]string{"file_key": key})
		if err != nil {
			return nil, channelFileSendError(err, sent)
		}
		body := channelMessageBody{ReceiveID: request.ExternalConversationID, MsgType: "file", Content: string(content),
			UUID: uuid.NewSHA1(request.DeliveryID, []byte("feishu:file:"+request.Attachments[i].ID)).String()}
		var response feishuMessageData
		meta, err := adapter.provider.client.sendMessage(ctx, region, token, "chat_id", body, &response)
		if err != nil {
			return nil, channelFileSendError(err, sent)
		}
		if strings.TrimSpace(response.MessageID) == "" {
			return nil, channelruntime.ErrDeliveryOutcomeUnknown
		}
		sent = true
		receipt = channelruntime.DeliveryReceipt{ProviderMessageID: response.MessageID, ProviderRequestID: meta.RequestID}
	}
	return &receipt, nil
}

func channelFileSendError(err error, alreadySent bool) error {
	code := providertransport.ErrorCode(err)
	if alreadySent || code == providertransport.ErrorCodeTimeout || code == providertransport.ErrorCodeUpstream || code == providertransport.ErrorCodeResponseInvalid {
		// A partial delivery cannot be retried as a whole or changed to a link.
		return errors.Join(channelruntime.ErrDeliveryOutcomeUnknown, err)
	}
	return err
}

func (c *client) uploadChannelFile(ctx context.Context, region, token string, file channelruntime.DeliveryAttachment) (string, error) {
	reader, err := file.Open(ctx)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	// Encode only the multipart headers in memory. File bodies are sent one at
	// a time, without retaining a batch or starting an unowned writer goroutine.
	var headers bytes.Buffer
	w := multipart.NewWriter(&headers)
	if err := w.WriteField("file_type", "stream"); err != nil {
		return "", err
	}
	if err := w.WriteField("file_name", file.Filename); err != nil {
		return "", err
	}
	if _, err := w.CreateFormFile("file", file.Filename); err != nil {
		return "", err
	}
	prefixLength := headers.Len()
	if err := w.Close(); err != nil {
		return "", err
	}
	base, err := c.baseURL(region)
	if err != nil {
		return "", err
	}
	endpoint := *base
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/open-apis/im/v1/files"
	encoded := headers.Bytes()
	body := io.MultiReader(bytes.NewReader(encoded[:prefixLength]), io.LimitReader(reader, file.Size), bytes.NewReader(encoded[prefixLength:]))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
	if err != nil {
		return "", err
	}
	req.ContentLength = int64(len(encoded)) + file.Size
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", providertransport.NewError(providertransport.ErrorCodeUpstream, "Feishu file upload failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", mapFeishuStatus(resp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(payload) > maxResponseBytes {
		return "", providertransport.NewError(providertransport.ErrorCodeResponseInvalid, "Feishu file upload response is invalid", err)
	}
	var response struct {
		Code int `json:"code"`
		Data struct {
			FileKey string `json:"file_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return "", providertransport.NewError(providertransport.ErrorCodeResponseInvalid, "Feishu file upload response is invalid", err)
	}
	if response.Code != 0 {
		return "", mapFeishuBusinessCode(response.Code)
	}
	if !validFeishuToken(response.Data.FileKey) {
		return "", providertransport.NewError(providertransport.ErrorCodeResponseInvalid, "Feishu file upload response has no file key", nil)
	}
	return response.Data.FileKey, nil
}
