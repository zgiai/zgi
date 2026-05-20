package sms

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type AliyunPayload struct {
	PhoneNumbers  string
	SignName      string
	TemplateCode  string
	TemplateParam string
}

type AliyunProvider struct {
	config AliyunConfig
	client *http.Client
}

func NewAliyunProvider(config AliyunConfig) *AliyunProvider {
	return &AliyunProvider{config: config, client: http.DefaultClient}
}

func (p *AliyunProvider) Provider() string {
	return ProviderAliyun
}

func (p *AliyunProvider) BuildPayload(req Request) (*AliyunPayload, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	if p.config.ParamMode != ParamModeMap {
		return nil, fmt.Errorf("unsupported aliyun param mode: %s", p.config.ParamMode)
	}

	values := map[string]string{
		"notification_title": req.NotificationTitle,
		"link_code":          req.LinkCode,
	}
	params := make(map[string]string, len(p.config.ParamMap))
	for internalName, providerName := range p.config.ParamMap {
		value, ok := values[internalName]
		if !ok {
			return nil, fmt.Errorf("unsupported aliyun param mapping key: %s", internalName)
		}
		params[providerName] = value
	}

	templateParam, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal aliyun template param: %w", err)
	}
	return &AliyunPayload{
		PhoneNumbers:  NormalizePhoneNumbers(req.Phone),
		SignName:      p.config.SignName,
		TemplateCode:  p.config.TemplateCode,
		TemplateParam: string(templateParam),
	}, nil
}

func (p *AliyunProvider) SendNotification(ctx context.Context, req Request) (*Result, error) {
	payload, err := p.BuildPayload(req)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimSpace(p.config.APIURL)
	if endpoint == "" {
		endpoint = "https://dysmsapi.aliyuncs.com/"
	}

	values := aliyunCommonParams(p.config.AccessKeyID)
	values.Set("Action", "SendSms")
	values.Set("PhoneNumbers", payload.PhoneNumbers)
	values.Set("SignName", payload.SignName)
	values.Set("TemplateCode", payload.TemplateCode)
	values.Set("TemplateParam", payload.TemplateParam)
	values.Set("Signature", aliyunSignature(values, p.config.AccessKeySecret))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+values.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("create aliyun request: %w", err)
	}
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send aliyun request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read aliyun response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("aliyun request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	var upstream struct {
		Code      string `json:"Code"`
		Message   string `json:"Message"`
		RequestID string `json:"RequestId"`
		BizID     string `json:"BizId"`
	}
	if err := json.Unmarshal(responseBody, &upstream); err != nil {
		return nil, fmt.Errorf("unmarshal aliyun response: %w", err)
	}
	if upstream.Code != "OK" {
		return nil, fmt.Errorf("aliyun returned code %s: %s", upstream.Code, upstream.Message)
	}
	messageID := upstream.BizID
	if messageID == "" {
		messageID = upstream.RequestID
	}
	return &Result{
		Provider:  ProviderAliyun,
		Accepted:  true,
		MessageID: messageID,
		RawCode:   upstream.Code,
	}, nil
}

func aliyunCommonParams(accessKeyID string) url.Values {
	values := url.Values{}
	values.Set("AccessKeyId", accessKeyID)
	values.Set("Format", "JSON")
	values.Set("SignatureMethod", "HMAC-SHA1")
	values.Set("SignatureNonce", fmt.Sprintf("%d", time.Now().UnixNano()))
	values.Set("SignatureVersion", "1.0")
	values.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	values.Set("Version", "2017-05-25")
	return values
}

func aliyunSignature(values url.Values, accessKeySecret string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key != "Signature" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, percentEncode(key)+"="+percentEncode(values.Get(key)))
	}
	canonicalQuery := strings.Join(pairs, "&")
	stringToSign := "GET&%2F&" + percentEncode(canonicalQuery)

	mac := hmac.New(sha1.New, []byte(accessKeySecret+"&"))
	_, _ = mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func percentEncode(value string) string {
	escaped := url.QueryEscape(value)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "*", "%2A")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}
