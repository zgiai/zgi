// Package dingtalk implements native, explicitly invoked robot REST messages.
package dingtalk

import (
	"context"
	"encoding/json"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"net/http"
	"strings"
)

type Client struct {
	http       *providers.HTTP
	definition catalog.Definition
	schemas    *providers.Schemas
	enabled    bool
}

func New(client *http.Client, enabled bool) (*Client, error) {
	d, _ := catalog.MessagingDefinition("dingtalk")
	s, err := providers.NewSchemasForDefinition(d)
	if err != nil {
		return nil, err
	}
	return &Client{providers.NewHTTP(client, "https://api.dingtalk.com"), d, s, enabled}, nil
}
func (*Client) ConnectorID() string { return "dingtalk" }
func (c *Client) token(ctx context.Context, input providers.CredentialInput) (string, error) {
	if !c.enabled {
		return "", &providers.Failure{Code: core.ErrorCodeConnectorUnavailable}
	}
	if catalog.ValidateCredentialValues(c.definition, input.Values) != nil {
		return "", providers.Invalid()
	}
	var result struct {
		Token   string `json:"accessToken"`
		Expires int    `json:"expireIn"`
	}
	_, err := c.http.JSON(ctx, "POST", "/v1.0/oauth2/accessToken", nil, http.Header{}, map[string]string{"appKey": input.Values["client_id"], "appSecret": input.Values["client_secret"]}, &result)
	if err != nil {
		f := *providers.FailureOf(err)
		f.Unknown = false
		return "", &f
	}
	if result.Token == "" || len(result.Token) > 16384 || strings.ContainsAny(result.Token, "\r\n") || result.Expires <= 0 {
		return "", &providers.Failure{Code: core.ErrorCodeInvalidAppCredentials}
	}
	return result.Token, nil
}
func (c *Client) VerifyCredentials(ctx context.Context, input providers.CredentialInput) (providers.Verification, error) {
	_, err := c.token(ctx, input)
	if err != nil {
		return providers.Verification{}, err
	}
	id := catalog.CredentialIdentity(c.definition, input.Values)
	e := &core.MessagingEvidence{Version: 1, Provider: "dingtalk", Subject: "app", Identity: id, CredentialVerified: true, Operations: []core.OperationEvidence{}}
	ops := []string{}
	for _, op := range c.definition.Operations {
		ops = append(ops, op.ID)
		e.Operations = append(e.Operations, core.OperationEvidence{ID: op.ID, Availability: "conditional", Permission: "unknown", Resource: "evaluated_at_execution"})
	}
	return providers.Verification{AccountID: id, DisplayName: input.Values["client_id"], Operations: ops, Evidence: &core.ProviderEvidence{Messaging: e}}, nil
}
func (c *Client) InvokeCredentials(ctx context.Context, op string, input providers.CredentialInput, args map[string]any) (providers.Response, error) {
	values, err := c.schemas.Input(op, args)
	if err != nil {
		return providers.Response{}, err
	}
	token, err := c.token(ctx, input)
	if err != nil {
		return providers.Response{}, err
	}
	msgKey := "sampleText"
	param := map[string]string{"content": providers.Text(values, "text")}
	if providers.Text(values, "format") == "markdown" {
		msgKey = "sampleMarkdown"
		param = map[string]string{"title": "Message", "text": providers.Text(values, "text")}
	}
	encoded, _ := json.Marshal(param)
	defer clear(encoded)
	body := map[string]any{"robotCode": input.Values["client_id"], "msgKey": msgKey, "msgParam": string(encoded)}
	path := "/v1.0/robot/oToMessages/batchSend"
	switch op {
	case "dingtalk.messages.send_direct":
		body["userIds"] = values["user_ids"]
	case "dingtalk.messages.send_group":
		path = "/v1.0/robot/groupMessages/send"
		body["openConversationId"] = values["conversation_id"]
	default:
		return providers.Response{}, providers.Invalid()
	}
	var result struct {
		Receipt    string   `json:"processQueryKey"`
		Invalid    []string `json:"invalidStaffIdList"`
		Controlled []string `json:"flowControlledStaffIdList"`
		Filtered   []string `json:"filteredStaffIdList"`
	}
	result.Invalid, result.Controlled, result.Filtered = []string{}, []string{}, []string{}
	_, err = c.http.JSON(ctx, "POST", path, nil, http.Header{"X-Acs-Dingtalk-Access-Token": {token}}, body, &result)
	if err != nil {
		f := *providers.FailureOf(err)
		f.RequestID = providers.SafeRequestID(f.RequestID, token)
		return providers.Response{}, &f
	}
	receipt := providers.SafeRequestID(providers.SafeRequestID(result.Receipt, token), input.Values["client_secret"])
	if receipt == "" || len(result.Invalid)+len(result.Controlled)+len(result.Filtered) > 60 {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true}
	}
	status := "accepted"
	if len(result.Invalid)+len(result.Controlled)+len(result.Filtered) > 0 {
		status = "partial"
	}
	output := map[string]any{"provider": "dingtalk", "operation": op, "status": status, "receipt": receipt, "invalid_user_ids": result.Invalid, "rate_limited_user_ids": result.Controlled, "filtered_user_ids": result.Filtered}
	providers.Redact(output, token)
	providers.Redact(output, input.Values["client_secret"])
	if err = c.schemas.Output(op, output); err != nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true}
	}
	return providers.Response{Value: output, RequestID: receipt}, nil
}
