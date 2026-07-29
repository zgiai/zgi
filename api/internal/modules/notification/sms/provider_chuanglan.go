package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ChuanglanPayload struct {
	Account           string `json:"account"`
	Password          string `json:"password,omitempty"`
	PhoneNumbers      string `json:"phoneNumbers"`
	TemplateID        string `json:"templateId"`
	TemplateParamJSON string `json:"templateParamJson"`
	Signature         string `json:"signature,omitempty"`
	Report            string `json:"report"`
	Extend            string `json:"extend,omitempty"`
}

type ChuanglanProvider struct {
	config ChuanglanConfig
	client *http.Client
}

func NewChuanglanProvider(config ChuanglanConfig) *ChuanglanProvider {
	return &ChuanglanProvider{config: config, client: http.DefaultClient}
}

func (p *ChuanglanProvider) Provider() string {
	return ProviderChuanglan
}

func (p *ChuanglanProvider) BuildPayload(req Request, template TemplateConfig) (*ChuanglanPayload, error) {
	payload, _, err := p.buildPayload(req, template)
	return payload, err
}

func (p *ChuanglanProvider) buildPayload(req Request, template TemplateConfig) (*ChuanglanPayload, chuanglanCredentialConfig, error) {
	if err := validateRequest(req, template); err != nil {
		return nil, chuanglanCredentialConfig{}, err
	}
	providerTemplate := template.Chuanglan
	if normalizedParamMode(providerTemplate.ParamMode, ParamModeOrderedParam) != ParamModeOrderedParam {
		return nil, chuanglanCredentialConfig{}, fmt.Errorf("unsupported chuanglan param mode: %s", providerTemplate.ParamMode)
	}
	credentials, ok := p.config.credentials(providerTemplate.CredentialProfile)
	if !ok {
		return nil, chuanglanCredentialConfig{}, fmt.Errorf("chuanglan credential profile %q is not configured", providerTemplate.CredentialProfile)
	}

	placeholderCount := strings.Count(providerTemplate.TemplateText, "{s}")
	if placeholderCount != len(providerTemplate.ParamOrder) {
		return nil, chuanglanCredentialConfig{}, fmt.Errorf("chuanglan template placeholder count %d does not match param order count %d", placeholderCount, len(providerTemplate.ParamOrder))
	}

	templateParams := templateParamConfigs(template.Params)
	ordered := make(map[string]string, len(providerTemplate.ParamOrder))
	for index, internalName := range providerTemplate.ParamOrder {
		param, ok := templateParams[internalName]
		if !ok {
			return nil, chuanglanCredentialConfig{}, fmt.Errorf("chuanglan param order key %s is not defined by template", internalName)
		}
		value := strings.TrimSpace(req.TemplateParams[internalName])
		if value == "" {
			if param.IsRequired() {
				return nil, chuanglanCredentialConfig{}, fmt.Errorf("chuanglan param order key %s is empty", internalName)
			}
			continue
		}
		ordered[fmt.Sprintf("param%d", index+1)] = value
	}

	paramJSON, err := json.Marshal([]map[string]string{ordered})
	if err != nil {
		return nil, chuanglanCredentialConfig{}, fmt.Errorf("marshal chuanglan template params: %w", err)
	}
	report := "false"
	if credentials.Report {
		report = "true"
	}

	payload := &ChuanglanPayload{
		Account:           credentials.Account,
		Password:          credentials.Password,
		PhoneNumbers:      normalizeChuanglanDomesticPhoneNumbers(req.Phone),
		TemplateID:        providerTemplate.TemplateID,
		TemplateParamJSON: string(paramJSON),
		Signature:         credentials.Signature,
		Report:            report,
		Extend:            credentials.Extend,
	}
	return payload, credentials, nil
}

func normalizeChuanglanDomesticPhoneNumbers(phone string) string {
	phones := splitPhoneNumbers(phone)
	for index, item := range phones {
		phones[index] = strings.TrimPrefix(item, "+86")
	}
	return strings.Join(phones, ",")
}

func (p *ChuanglanProvider) SendNotification(ctx context.Context, req Request, template TemplateConfig) (*Result, error) {
	payload, credentials, err := p.buildPayload(req, template)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal chuanglan request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, credentials.APIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create chuanglan request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send chuanglan request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read chuanglan response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("chuanglan request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	var upstream struct {
		Code     string `json:"code"`
		Message  string `json:"message"`
		ErrorMsg string `json:"errorMsg"`
		Msg      string `json:"msg"`
		MsgID    string `json:"msgId"`
		MsgId    string `json:"msgid"`
	}
	if err := json.Unmarshal(responseBody, &upstream); err != nil {
		return nil, fmt.Errorf("unmarshal chuanglan response: %w", err)
	}
	if upstream.Code != "000000" {
		return nil, fmt.Errorf("chuanglan returned code %s: %s", upstream.Code, firstNonEmpty(upstream.ErrorMsg, upstream.Message, upstream.Msg))
	}
	messageID := upstream.MsgID
	if messageID == "" {
		messageID = upstream.MsgId
	}
	return &Result{
		Provider:  ProviderChuanglan,
		Accepted:  true,
		MessageID: messageID,
		RawCode:   upstream.Code,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
