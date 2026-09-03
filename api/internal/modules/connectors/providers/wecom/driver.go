// Package wecom uses the official bot CLI HTTP protocol, without opening a
// WebSocket that could displace another bot client.
package wecom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/messaging"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	http       *providers.HTTP
	definition catalog.Definition
	schemas    *providers.Schemas
	store      messaging.Store
	enabled    bool
}

func New(client *http.Client, enabled bool, store messaging.Store) (*Client, error) {
	d, _ := catalog.MessagingDefinition("wecom")
	s, err := providers.NewSchemasForDefinition(d)
	if err != nil {
		return nil, err
	}
	return &Client{providers.NewHTTP(client, "https://qyapi.weixin.qq.com"), d, s, store, enabled}, nil
}
func (*Client) ConnectorID() string     { return "wecom" }
func failure(code core.ErrorCode) error { return &providers.Failure{Code: code} }
func (c *Client) token(ctx context.Context, in providers.CredentialInput) (string, error) {
	if !c.enabled {
		return "", failure(core.ErrorCodeConnectorUnavailable)
	}
	if catalog.ValidateCredentialValues(c.definition, in.Values) != nil {
		return "", providers.Invalid()
	}
	ts := time.Now().Unix()
	nonce := "cli_" + strconv.FormatInt(time.Now().UnixMilli(), 10) + "_" + uuid.NewString()
	sum := sha256.Sum256([]byte(in.Values["secret"] + in.Values["bot_id"] + strconv.FormatInt(ts, 10) + nonce))
	bindSource := 1
	if _, ok := core.VerifiedRegistration(ctx, in.ResourceScope.OrganizationID, in.ResourceScope.OwnerAccountID, "wecom", catalog.CredentialIdentity(c.definition, in.Values), in.Values); ok {
		bindSource = 2
	}
	var result struct {
		Code  *int   `json:"errcode"`
		Token string `json:"token"`
	}
	_, err := c.http.JSON(ctx, "POST", "/cgi-bin/aibot/cli/get_cli_config", nil, http.Header{}, map[string]any{"bot_id": in.Values["bot_id"], "time": ts, "nonce": nonce, "signature": hex.EncodeToString(sum[:]), "bind_source": bindSource}, &result)
	if err != nil {
		f := *providers.FailureOf(err)
		f.Unknown = false
		return "", &f
	}
	if result.Code == nil || *result.Code != 0 || result.Token == "" || len(result.Token) > 16384 || strings.ContainsAny(result.Token, "\r\n") {
		return "", failure(core.ErrorCodeInvalidAppCredentials)
	}
	return result.Token, nil
}
func (c *Client) call(ctx context.Context, path, token string, body any, write bool) (map[string]any, error) {
	raw, _ := json.Marshal(body)
	defer clear(raw)
	var outer struct {
		Code    *int   `json:"errcode"`
		Results string `json:"results_json"`
	}
	_, err := c.http.JSON(ctx, "POST", "/cli"+path, nil, http.Header{"Authorization": {"Bearer " + token}}, map[string]string{"payload": string(raw)}, &outer)
	if err != nil {
		f := *providers.FailureOf(err)
		f.Unknown = f.Unknown && write
		f.RequestID = providers.SafeRequestID(f.RequestID, token)
		return nil, &f
	}
	if outer.Code == nil {
		return nil, &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: write}
	}
	if *outer.Code != 0 {
		code := core.ErrorCodePermissionMissing
		if *outer.Code == 853004 {
			code = core.ErrorCodeInvalidAppCredentials
		}
		return nil, failure(code)
	}
	var inner struct {
		Result *string `json:"result"`
		Error  *struct {
			Code json.RawMessage `json:"code"`
		} `json:"error"`
		TaskID string `json:"taskid"`
	}
	if json.Unmarshal([]byte(outer.Results), &inner) != nil {
		return nil, &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: write}
	}
	if inner.Error != nil && string(inner.Error.Code) != "0" && string(inner.Error.Code) != "null" {
		return nil, failure(core.ErrorCodePermissionMissing)
	}
	var result map[string]any
	if inner.Result == nil || json.Unmarshal([]byte(*inner.Result), &result) != nil || result == nil {
		return nil, &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: write}
	}
	return result, nil
}

func (c *Client) business(ctx context.Context, path string, in providers.CredentialInput, token *string, body any, write bool) (map[string]any, error) {
	result, err := c.call(ctx, path, *token, body, write)
	if err == nil {
		return result, nil
	}
	f := providers.FailureOf(err)
	// Only the explicit 853004 refusal maps to invalid credentials here. It
	// proves the operation was not accepted; transport ambiguity never retries.
	if f.Code != core.ErrorCodeInvalidAppCredentials || f.Unknown {
		return nil, err
	}
	next, err := c.token(ctx, in)
	if err != nil {
		return nil, err
	}
	*token = next
	return c.call(ctx, path, next, body, write)
}
func (c *Client) VerifyCredentials(ctx context.Context, in providers.CredentialInput) (providers.Verification, error) {
	token, err := c.token(ctx, in)
	if err != nil {
		return providers.Verification{}, err
	}
	identity, err := c.business(ctx, "/identity/whoami", in, &token, map[string]any{}, false)
	if err != nil {
		return providers.Verification{}, err
	}
	if providers.Text(identity, "extra_identity_context") == "" {
		return providers.Verification{}, failure(core.ErrorCodeApplicationNotReady)
	}
	sessions, err := c.business(ctx, "/message/aibot/sessions/list", in, &token, map[string]any{}, false)
	if err != nil {
		return providers.Verification{}, err
	}
	if _, err = parseSessions(sessions); err != nil {
		return providers.Verification{}, err
	}
	id := catalog.CredentialIdentity(c.definition, in.Values)
	e := &core.MessagingEvidence{Version: 1, Provider: "wecom", Subject: "bot", Identity: id, CredentialVerified: true, Operations: []core.OperationEvidence{}}
	ops := []string{}
	for _, op := range c.definition.Operations {
		availability := "verified"
		if op.Effect == catalog.OperationEffectExternalSend {
			availability = "context_required"
		}
		ops = append(ops, op.ID)
		e.Operations = append(e.Operations, core.OperationEvidence{ID: op.ID, Availability: availability, Permission: "unknown", Resource: "evaluated_at_execution"})
	}
	return providers.Verification{AccountID: id, DisplayName: in.Values["bot_id"], Operations: ops, Evidence: &core.ProviderEvidence{Messaging: e}}, nil
}
func parseSessions(data map[string]any) ([]map[string]any, error) {
	items, ok := data["sessions"].([]any)
	if !ok || len(items) > 20 {
		return nil, failure(core.ErrorCodeProviderUnavailable)
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		id := providers.Text(row, "chat_id")
		kind := providers.Text(row, "chat_type")
		if !ok || id == "" || len(id) > 8192 || (kind != "single" && kind != "group") {
			return nil, failure(core.ErrorCodeProviderUnavailable)
		}
		result = append(result, row)
	}
	return result, nil
}
func (c *Client) InvokeCredentials(ctx context.Context, op string, in providers.CredentialInput, args map[string]any) (providers.Response, error) {
	values, err := c.schemas.Input(op, args)
	if err != nil {
		return providers.Response{}, err
	}
	var conversation messaging.Conversation
	if op == "wecom.messages.send" {
		if c.store == nil {
			return providers.Response{}, failure(core.ErrorCodeConnectorUnavailable)
		}
		conversation, err = c.store.Resolve(ctx, in.ResourceScope, "wecom", providers.Text(values, "context_handle"))
		if err != nil {
			return providers.Response{}, err
		}
	}
	token, err := c.token(ctx, in)
	if err != nil {
		return providers.Response{}, err
	}
	var data map[string]any
	switch op {
	case "wecom.identity.get":
		raw, e := c.business(ctx, "/identity/whoami", in, &token, map[string]any{}, false)
		err = e
		data = map[string]any{"authorization_context": providers.SafeText(providers.Text(raw, "extra_identity_context"), 4096)}
	case "wecom.sessions.list":
		if c.store == nil {
			return providers.Response{}, failure(core.ErrorCodeConnectorUnavailable)
		}
		raw, e := c.business(ctx, "/message/aibot/sessions/list", in, &token, map[string]any{}, false)
		if e != nil {
			return providers.Response{}, e
		}
		items, e := parseSessions(raw)
		if e != nil {
			return providers.Response{}, e
		}
		rows := []any{}
		for _, item := range items {
			handle, e := c.store.Put(ctx, in.ResourceScope, "wecom", messaging.Conversation{Recipient: providers.Text(item, "chat_id")})
			if e != nil {
				return providers.Response{}, e
			}
			rows = append(rows, map[string]any{"context_handle": handle, "name": providers.SafeText(providers.Text(item, "chat_name"), 128), "type": providers.Text(item, "chat_type"), "last_message_time": providers.SafeText(providers.Text(item, "last_msg_time"), 48)})
		}
		data = map[string]any{"sessions": rows}
	case "wecom.messages.send":
		raw, e := c.business(ctx, "/message/aibot/send", in, &token, map[string]any{"chat_id": conversation.Recipient, "msg_type": "markdown", "markdown": map[string]string{"content": providers.Text(values, "text")}}, true)
		err = e
		if err == nil && raw["success"] != true {
			return providers.Response{}, &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true}
		}
		data = map[string]any{"status": "accepted"}
	default:
		return providers.Response{}, providers.Invalid()
	}
	if err != nil {
		return providers.Response{}, err
	}
	out := map[string]any{"provider": "wecom", "operation": op, "data": data}
	providers.Redact(out, token)
	providers.Redact(out, in.Values["secret"])
	if err = c.schemas.Output(op, out); err != nil {
		return providers.Response{}, err
	}
	return providers.Response{Value: out}, nil
}
