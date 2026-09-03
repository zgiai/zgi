// Package weixin explicitly pulls direct messages and replies through private
// context handles. It never starts a background messaging gateway.
package weixin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/ilink"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/messaging"
	"net/http"
	"slices"
	"strings"
	"time"
)

type Client struct {
	http       *providers.HTTP
	definition catalog.Definition
	schemas    *providers.Schemas
	store      messaging.Store
	enabled    bool
	appID      string
}

func New(client *http.Client, enabled bool, appID string, store messaging.Store) (*Client, error) {
	d, _ := catalog.MessagingDefinition("weixin")
	s, err := providers.NewSchemasForDefinition(d)
	if err != nil {
		return nil, err
	}
	if enabled && (appID == "" || len(appID) > 80 || strings.ContainsAny(appID, "\r\n ")) {
		return nil, providers.Invalid()
	}
	return &Client{providers.NewBoundedHTTP(client, ilink.Origin, 35*time.Second), d, s, store, enabled, appID}, nil
}
func (*Client) ConnectorID() string     { return "weixin" }
func failure(code core.ErrorCode) error { return &providers.Failure{Code: code} }
func (c *Client) check(in providers.CredentialInput) error {
	if !c.enabled || c.store == nil {
		return failure(core.ErrorCodeConnectorUnavailable)
	}
	if catalog.ValidateCredentialValues(c.definition, in.Values) != nil {
		return providers.Invalid()
	}
	return nil
}

type updateResponse struct {
	Ret      *int    `json:"ret"`
	Code     *int    `json:"errcode"`
	Cursor   *string `json:"get_updates_buf"`
	Messages []struct {
		ID       json.Number `json:"message_id"`
		ClientID string      `json:"client_id"`
		From     string      `json:"from_user_id"`
		To       string      `json:"to_user_id"`
		Group    string      `json:"group_id"`
		Type     int         `json:"message_type"`
		State    int         `json:"message_state"`
		Context  string      `json:"context_token"`
		Items    []struct {
			Type int `json:"type"`
			Text *struct {
				Text string `json:"text"`
			} `json:"text_item"`
		} `json:"item_list"`
	} `json:"msgs"`
}

func (c *Client) pull(ctx context.Context, in providers.CredentialInput, state *messaging.State) error {
	var response updateResponse
	_, err := c.http.JSON(ctx, "POST", "/ilink/bot/getupdates", nil, ilink.Headers(c.appID, in.Values["token"], true), map[string]any{"get_updates_buf": state.Cursor, "base_info": ilink.BaseInfo()}, &response)
	if err != nil {
		f := *providers.FailureOf(err)
		f.Unknown = false
		return &f
	}
	if response.Ret != nil && *response.Ret == -14 || response.Code != nil && *response.Code == -14 {
		return failure(core.ErrorCodeInvalidAppCredentials)
	}
	if response.Ret == nil || *response.Ret != 0 || response.Code != nil && *response.Code != 0 || response.Cursor == nil || len(*response.Cursor) > 32768 || len(response.Messages) > 100 {
		return failure(core.ErrorCodeProviderUnavailable)
	}
	for _, msg := range response.Messages {
		if msg.Group != "" || msg.Type != 1 || msg.State != 2 {
			continue
		}
		id := string(msg.ID)
		if id == "" {
			id = msg.ClientID
		}
		if id == "" || len(id) > 256 || msg.From == "" || len(msg.From) > 8192 || len(msg.Context) > 16384 || len(msg.Items) > 100 {
			return failure(core.ErrorCodeProviderUnavailable)
		}
		if slices.Contains(state.Seen, id) {
			continue
		}
		text := ""
		for _, item := range msg.Items {
			if item.Type == 1 && item.Text != nil {
				text += item.Text.Text + "\n"
			}
		}
		text = providers.SafeText(strings.TrimSpace(text), 4000)
		state.Messages = append(state.Messages, messaging.Message{ID: id, Recipient: msg.From, Text: text, ContextToken: msg.Context, ReceivedAt: time.Now().UTC()})
		state.Seen = append(state.Seen, id)
		if len(state.Seen) > 256 {
			state.Seen = state.Seen[len(state.Seen)-256:]
		}
	}
	if len(state.Messages) > 100 {
		return failure(core.ErrorCodeProviderUnavailable)
	}
	state.Cursor = *response.Cursor
	return nil
}
func (c *Client) VerifyCredentials(ctx context.Context, in providers.CredentialInput) (providers.Verification, error) {
	if err := c.check(in); err != nil {
		return providers.Verification{}, err
	}
	id := catalog.CredentialIdentity(c.definition, in.Values)
	flow, proven := core.VerifiedRegistration(ctx, in.ResourceScope.OrganizationID, in.ResourceScope.OwnerAccountID, "weixin", id, in.Values)
	if !proven {
		flow = uuid.Nil
	}
	// Without QR proof, only a previously committed state for this exact
	// connection and credential digest can be rechecked; arbitrary manual
	// account_id + token combinations cannot create an identity assertion.
	err := c.store.Update(ctx, in.ResourceScope, "weixin", messaging.CredentialDigest(in.Values), flow, func(ctx context.Context, state *messaging.State) error { return c.pull(ctx, in, state) })
	if err != nil {
		return providers.Verification{}, err
	}
	e := &core.MessagingEvidence{Version: 1, Provider: "weixin", Subject: "account", Identity: id, CredentialVerified: true, Operations: []core.OperationEvidence{{ID: "weixin.messages.pull", Availability: "verified", Permission: "unknown", Resource: "evaluated_at_execution"}, {ID: "weixin.messages.reply", Availability: "context_required", Permission: "unknown", Resource: "evaluated_at_execution"}}}
	return providers.Verification{AccountID: id, DisplayName: in.Values["account_id"], Operations: []string{"weixin.messages.pull", "weixin.messages.reply"}, Evidence: &core.ProviderEvidence{Messaging: e}}, nil
}
func (c *Client) InvokeCredentials(ctx context.Context, op string, in providers.CredentialInput, args map[string]any) (providers.Response, error) {
	if err := c.check(in); err != nil {
		return providers.Response{}, err
	}
	values, err := c.schemas.Input(op, args)
	if err != nil {
		return providers.Response{}, err
	}
	out := map[string]any{"provider": "weixin", "operation": op}
	switch op {
	case "weixin.messages.pull":
		rows := []any{}
		err = c.store.Update(ctx, in.ResourceScope, "weixin", messaging.CredentialDigest(in.Values), uuid.Nil, func(ctx context.Context, state *messaging.State) error {
			if len(state.Messages) == 0 {
				if err := c.pull(ctx, in, state); err != nil {
					return err
				}
			}
			for _, msg := range state.Messages {
				if time.Since(msg.ReceivedAt) > 6*time.Hour {
					continue
				}
				row := map[string]any{"message_id": msg.ID, "text": msg.Text, "reply_available": false}
				providers.Redact(row, msg.ContextToken)
				if msg.ContextToken != "" {
					handle, err := c.store.Put(ctx, in.ResourceScope, "weixin", messaging.Conversation{Recipient: msg.Recipient, ContextToken: msg.ContextToken})
					if err != nil {
						return err
					}
					row["context_handle"] = handle
					row["reply_available"] = true
				}
				rows = append(rows, row)
			}
			state.Messages = []messaging.Message{}
			return nil
		})
		out["messages"] = rows
	case "weixin.messages.reply":
		conversation, e := c.store.Resolve(ctx, in.ResourceScope, "weixin", providers.Text(values, "context_handle"))
		if e != nil {
			return providers.Response{}, e
		}
		if conversation.ContextToken == "" {
			return providers.Response{}, failure(core.ErrorCodeResourceAccessDenied)
		}
		if in.ResourceScope.LogicalOperationID == "" {
			return providers.Response{}, providers.Invalid()
		}
		sum := sha256.Sum256([]byte(in.ResourceScope.ConnectionID.String() + ":" + in.ResourceScope.LogicalOperationID))
		clientID := "zgi-" + hex.EncodeToString(sum[:16])
		body := map[string]any{"base_info": ilink.BaseInfo(), "msg": map[string]any{"from_user_id": "", "to_user_id": conversation.Recipient, "client_id": clientID, "message_type": 2, "message_state": 2, "context_token": conversation.ContextToken, "item_list": []any{map[string]any{"type": 1, "text_item": map[string]string{"text": providers.Text(values, "text")}}}}}
		var response struct {
			Ret  *int `json:"ret"`
			Code *int `json:"errcode"`
		}
		_, err = c.http.JSON(ctx, "POST", "/ilink/bot/sendmessage", nil, ilink.Headers(c.appID, in.Values["token"], true), body, &response)
		if err == nil {
			if response.Ret == nil {
				err = &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true}
			} else if *response.Ret != 0 || response.Code != nil && *response.Code != 0 {
				if *response.Ret == -14 || response.Code != nil && *response.Code == -14 {
					err = failure(core.ErrorCodeInvalidAppCredentials)
				} else {
					err = failure(core.ErrorCodeResourceAccessDenied)
				}
			}
		}
		out["status"] = "accepted"
		out["receipt"] = clientID
	default:
		return providers.Response{}, providers.Invalid()
	}
	if err != nil {
		return providers.Response{}, err
	}
	providers.Redact(out, in.Values["token"])
	if err = c.schemas.Output(op, out); err != nil {
		return providers.Response{}, err
	}
	return providers.Response{Value: out}, nil
}
