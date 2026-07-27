package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"net/mail"
	"net/url"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type Adapter struct {
	client *client
}

func New(httpClient *http.Client) (*Adapter, error) {
	apiClient, err := newClient(httpClient)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: apiClient}, nil
}

func newForBaseURLs(httpClient *http.Client, apiBaseURL, identityBaseURL string) (*Adapter, error) {
	apiClient, err := newClientForBaseURLs(httpClient, apiBaseURL, identityBaseURL)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: apiClient}, nil
}

func (adapter *Adapter) DriverID() string { return DriverID }

func (adapter *Adapter) Execute(ctx context.Context, request integrations.ActionRequest) (*integrations.ActionResult, error) {
	accessToken, err := gmailAccessToken(request.Connection)
	if err != nil {
		return nil, err
	}
	switch request.ActionID {
	case ActionGetAccount:
		output, meta, err := adapter.getAccount(ctx, accessToken)
		return gmailActionResult(output, meta, 1), err
	case ActionSendMail:
		output, meta, err := adapter.sendMail(ctx, accessToken, request.Input)
		return gmailActionResult(output, meta, 1), err
	default:
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail action is not supported", nil)
	}
}

func (adapter *Adapter) ValidateConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.ConnectionProfile, error) {
	accessToken, err := gmailAccessToken(connection)
	if err != nil {
		return nil, err
	}
	var identity googleIdentity
	meta, err := adapter.client.getIdentity(ctx, accessToken, &identity)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(identity.Subject) == "" || strings.TrimSpace(identity.Email) == "" {
		return nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Google identity response is incomplete", nil)
	}
	return &integrations.ConnectionProfile{
		AccountID:         bounded(identity.Subject, 255),
		DisplayName:       bounded(firstNonEmpty(identity.Name, identity.Email), 255),
		GrantedScopes:     append([]string(nil), connection.GrantedScopes...),
		ProviderRequestID: bounded(meta.RequestID, 128),
	}, nil
}

func (adapter *Adapter) ProbeConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.HealthProbeReport, error) {
	profile, err := adapter.ValidateConnection(ctx, connection)
	if err != nil {
		status := integrations.HealthProbeStatusUnhealthy
		switch integrations.ErrorCode(err) {
		case integrations.ErrorCodeTimeout, integrations.ErrorCodeUpstream, integrations.ErrorCodeRateLimited:
			status = integrations.HealthProbeStatusDegraded
		}
		return &integrations.HealthProbeReport{
			Status: status,
			Checks: []integrations.HealthProbeCheck{{
				Code: integrations.ErrorCode(err), Status: status,
				Message: "Google account connection check failed",
			}},
		}, err
	}
	return &integrations.HealthProbeReport{
		Status:  integrations.HealthProbeStatusHealthy,
		Profile: profile,
		Checks: []integrations.HealthProbeCheck{{
			Code: "gmail_authenticated_account", Status: integrations.HealthProbeStatusHealthy,
		}},
	}, nil
}

func (adapter *Adapter) getAccount(ctx context.Context, accessToken string) (map[string]interface{}, responseMeta, error) {
	var identity googleIdentity
	meta, err := adapter.client.getIdentity(ctx, accessToken, &identity)
	if err != nil {
		return nil, meta, err
	}
	if strings.TrimSpace(identity.Subject) == "" || strings.TrimSpace(identity.Email) == "" {
		return nil, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Google identity response is incomplete", nil)
	}
	return map[string]interface{}{
		"provider":   IntegrationID,
		"request_id": bounded(meta.RequestID, 128),
		"account": map[string]interface{}{
			"id":             bounded(identity.Subject, 255),
			"email":          bounded(identity.Email, 320),
			"name":           bounded(identity.Name, 255),
			"picture":        safeHTTPSURL(identity.Picture, 2048),
			"email_verified": identity.EmailVerified,
		},
	}, meta, nil
}

func (adapter *Adapter) sendMail(ctx context.Context, accessToken string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	message, err := buildRFC2822Message(input)
	if err != nil {
		return nil, responseMeta{}, err
	}
	var sent gmailSendResponse
	meta, err := adapter.client.sendMessage(ctx, accessToken, base64.RawURLEncoding.EncodeToString(message), &sent)
	if err != nil {
		return nil, meta, err
	}
	if strings.TrimSpace(sent.ID) == "" {
		return nil, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail send response is incomplete", nil)
	}
	labels := make([]interface{}, 0, min(len(sent.LabelIDs), 20))
	for _, label := range sent.LabelIDs {
		if value := bounded(label, 100); value != "" && len(labels) < 20 {
			labels = append(labels, value)
		}
	}
	return map[string]interface{}{
		"provider":   IntegrationID,
		"request_id": bounded(meta.RequestID, 128),
		"message": map[string]interface{}{
			"id": bounded(sent.ID, 255), "thread_id": bounded(sent.ThreadID, 255), "label_ids": labels,
		},
	}, meta, nil
}

func buildRFC2822Message(input map[string]interface{}) ([]byte, error) {
	to, err := normalizedAddressList(inputString(input, "to"), 20)
	if err != nil || len(to) == 0 {
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail recipient list is invalid", err)
	}
	cc, err := normalizedAddressList(inputString(input, "cc"), 20)
	if err != nil {
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail CC list is invalid", err)
	}
	subject := strings.TrimSpace(inputString(input, "subject"))
	body := inputString(input, "body_text")
	if subject == "" || len([]rune(subject)) > 998 || containsHeaderBreak(subject) {
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail subject is invalid", nil)
	}
	if body == "" || len([]rune(body)) > 100_000 {
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail message body is invalid", nil)
	}
	var builder strings.Builder
	builder.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	if len(cc) > 0 {
		builder.WriteString("Cc: " + strings.Join(cc, ", ") + "\r\n")
	}
	builder.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\n")
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	builder.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	builder.WriteString(wrapBase64(base64.StdEncoding.EncodeToString([]byte(body)), 76))
	builder.WriteString("\r\n")
	return []byte(builder.String()), nil
}

func normalizedAddressList(value string, limit int) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	addresses, err := mail.ParseAddressList(value)
	if err != nil || len(addresses) > limit {
		return nil, fmt.Errorf("address list is invalid")
	}
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address == nil || containsHeaderBreak(address.Name) || containsHeaderBreak(address.Address) ||
			len(address.Address) > 320 {
			return nil, fmt.Errorf("address is invalid")
		}
		result = append(result, address.String())
	}
	return result, nil
}

func containsHeaderBreak(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

func wrapBase64(value string, width int) string {
	if width <= 0 {
		return value
	}
	var builder strings.Builder
	for len(value) > width {
		builder.WriteString(value[:width])
		builder.WriteString("\r\n")
		value = value[width:]
	}
	builder.WriteString(value)
	return builder.String()
}

func gmailAccessToken(connection *integrations.ResolvedConnection) (string, error) {
	if connection == nil || !strings.EqualFold(connection.IntegrationID, IntegrationID) ||
		!strings.EqualFold(connection.DriverID, DriverID) {
		return "", integrations.NewError(integrations.ErrorCodeConnectionInvalid, "Gmail connection is invalid", nil)
	}
	token := strings.TrimSpace(connection.Credentials["access_token"])
	if token == "" {
		return "", integrations.NewError(integrations.ErrorCodeAuthInvalid, "Gmail credentials are unavailable", nil)
	}
	return token, nil
}

func gmailActionResult(output map[string]interface{}, meta responseMeta, count int) *integrations.ActionResult {
	if output == nil {
		return nil
	}
	return &integrations.ActionResult{
		Output: output, ProviderRequestID: bounded(meta.RequestID, 128),
		ResultCount: count, AttemptCount: max(meta.Attempts, 1),
	}
}

type googleIdentity struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

type gmailSendResponse struct {
	ID       string   `json:"id"`
	ThreadID string   `json:"threadId"`
	LabelIDs []string `json:"labelIds"`
}

func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func safeHTTPSURL(value string, limit int) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return ""
	}
	parsed.Fragment = ""
	return bounded(parsed.String(), limit)
}

func inputString(input map[string]interface{}, key string) string {
	value, _ := input[key].(string)
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

var _ integrations.Adapter = (*Adapter)(nil)
var _ integrations.ConnectionTester = (*Adapter)(nil)
var _ integrations.HealthProbe = (*Adapter)(nil)
