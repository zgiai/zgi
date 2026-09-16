package feishu

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

const MaxResourceBytes = 10 << 20

// ResourceStore is supplied by the composition root. Reads must authorize a
// platform file reference; writes must create an owner-scoped platform file.
// Neither provider URLs, storage keys nor file content are returned to models.
type ResourceStore interface {
	Read(context.Context, providers.ResourceScope, string) (ResourceFile, error)
	Save(context.Context, providers.ResourceScope, ResourceFile) (string, error)
}
type ResourceFile struct {
	Name, MIME string
	Content    []byte
}

func ResourceMIME(content []byte) string {
	if len(content) == 0 || len(content) > MaxResourceBytes {
		return ""
	}
	media, _, _ := mime.ParseMediaType(http.DetectContentType(content))
	switch media {
	case "image/jpeg", "image/png", "image/webp", "application/pdf", "text/plain":
		return media
	}
	return ""
}

func (c *Client) resource(ctx context.Context, operation string, input providers.CredentialInput, args map[string]any) (providers.Response, error) {
	scope := input.ResourceScope
	if ctx == nil || c.files == nil || scope.OrganizationID == uuid.Nil || scope.OwnerAccountID == uuid.Nil || scope.Target.Validate() != nil {
		return providers.Response{}, failure(core.ErrorCodeAccessDenied)
	}
	var file ResourceFile
	var err error
	if operation == "feishu.resource.upload" {
		file, err = c.files.Read(ctx, scope, providers.Text(args, "file_id"))
		if err != nil {
			return providers.Response{}, failure(core.ErrorCodeAccessDenied)
		}
		defer clear(file.Content)
		file.MIME = ResourceMIME(file.Content)
		if file.MIME == "" {
			return providers.Response{}, providers.Invalid()
		}
	}
	token, err := c.tenantToken(ctx, input)
	if err != nil {
		return providers.Response{}, err
	}
	origin := "https://open.feishu.cn"
	if input.Values["region"] == "lark" {
		origin = "https://open.larksuite.com"
	}
	var req *http.Request
	write := operation == "feishu.resource.upload"
	if write {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		path, field := "/open-apis/im/v1/files", "file"
		if strings.HasPrefix(file.MIME, "image/") {
			path, field = "/open-apis/im/v1/images", "image"
			_ = writer.WriteField("image_type", "message")
		} else {
			kind := "stream"
			if file.MIME == "application/pdf" {
				kind = "pdf"
			}
			_ = writer.WriteField("file_type", kind)
			_ = writer.WriteField("file_name", resourceName(file.MIME))
		}
		part, partErr := writer.CreateFormFile(field, resourceName(file.MIME))
		if partErr != nil {
			return providers.Response{}, providers.Invalid()
		}
		_, _ = part.Write(file.Content)
		_ = writer.Close()
		defer clear(body.Bytes())
		req, err = http.NewRequestWithContext(ctx, "POST", origin+path, &body)
		if err == nil {
			req.Header.Set("Content-Type", writer.FormDataContentType())
		}
	} else {
		path := "/open-apis/im/v1/messages/" + url.PathEscape(providers.Text(args, "message_id")) + "/resources/" + url.PathEscape(providers.Text(args, "file_key"))
		req, err = http.NewRequestWithContext(ctx, "GET", origin+path+"?type="+url.QueryEscape(providers.Text(args, "type")), nil)
	}
	if err != nil {
		return providers.Response{}, providers.Invalid()
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := c.http.Do(req)
	if err != nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: write}
	}
	if write || res.StatusCode < 200 || res.StatusCode >= 300 || strings.Contains(res.Header.Get("Content-Type"), "json") {
		response, requestID, err := c.decodeResponse(res, input, token, write)
		if err != nil {
			return providers.Response{}, err
		}
		if !write {
			// Resource reads must return an actual supported file, never a
			// successful JSON upload-style receipt disguised as a download.
			return providers.Response{}, failure(core.ErrorCodeProviderUnavailable)
		}
		data, _ := response["data"].(map[string]any)
		keyName := "file_key"
		if strings.HasPrefix(file.MIME, "image/") {
			keyName = "image_key"
		}
		key := providers.Text(data, keyName)
		if len(key) == 0 || len(key) > 256 || strings.ContainsAny(key, "\r\n") || strings.Contains(key, token) || strings.Contains(key, input.Values["app_secret"]) {
			return providers.Response{}, &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: write}
		}
		return resourceResponse(operation, map[string]any{keyName: key, "size": len(file.Content), "mime_type": file.MIME}, requestID), nil
	}
	defer res.Body.Close()
	if res.ContentLength > MaxResourceBytes {
		return providers.Response{}, providers.Invalid()
	}
	content, err := io.ReadAll(io.LimitReader(res.Body, MaxResourceBytes+1))
	defer clear(content)
	media := ResourceMIME(content)
	if err != nil || media == "" {
		return providers.Response{}, providers.Invalid()
	}
	if providers.Text(args, "type") == "image" && !strings.HasPrefix(media, "image/") {
		return providers.Response{}, providers.Invalid()
	}
	fileID, err := c.files.Save(ctx, scope, ResourceFile{Name: resourceName(media), MIME: media, Content: content})
	if err != nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true}
	}
	if id, err := uuid.Parse(fileID); err != nil || id == uuid.Nil {
		return providers.Response{}, failure(core.ErrorCodeProviderUnavailable)
	}
	requestID := providers.SafeRequestID(providers.SafeRequestID(res.Header.Get("X-Tt-Logid"), token), input.Values["app_secret"])
	return resourceResponse(operation, map[string]any{"file_id": fileID, "name": resourceName(media), "size": len(content), "mime_type": media}, requestID), nil
}
func resourceName(media string) string {
	ext := map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp", "application/pdf": "pdf", "text/plain": "txt"}[media]
	return "feishu-resource." + ext
}
func resourceResponse(operation string, data map[string]any, requestID string) providers.Response {
	return providers.Response{Value: map[string]any{"provider": "feishu", "operation": operation, "data": data, "truncated": false}, RequestID: requestID}
}
