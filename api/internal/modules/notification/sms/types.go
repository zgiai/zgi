package sms

import (
	"context"
	"strings"
)

const (
	FeatureNotificationSMS            = "notification_sms"
	TemplatePendingActionNotification = "pending_action_notification"
	TemplateParamNotificationTitle    = "notification_title"
	TemplateParamLinkSuffix           = "link_suffix"
	TemplateParamLinkCode             = "link_code"
	ProviderAliyun                    = "aliyun"
	ProviderChuanglan                 = "chuanglan"
	ParamModeMap                      = "map"
	ParamModeOrderedParam             = "ordered_param"
)

type Service interface {
	IsEnabled() bool
	Send(ctx context.Context, req Request) (*Result, error)
}

type Provider interface {
	Provider() string
	SendNotification(ctx context.Context, req Request) (*Result, error)
}

type Request struct {
	Provider          string
	Phone             string
	Template          string
	TemplateParams    map[string]string
	NotificationTitle string
	LinkCode          string
	Source            string
	SourceID          string
}

type Result struct {
	Provider  string
	Accepted  bool
	MessageID string
	RawCode   string
}

type CapabilityStatus struct {
	Enabled         bool     `json:"enabled"`
	Providers       []string `json:"providers"`
	DefaultProvider string   `json:"default_provider"`
	Template        string   `json:"template"`
	PreviewTemplate string   `json:"preview_template,omitempty"`
}

type Config struct {
	Enabled         bool
	Providers       []string
	DefaultProvider string
	Template        string
	PreviewTemplate string
	Aliyun          AliyunConfig
	Chuanglan       ChuanglanConfig
}

type AliyunConfig struct {
	AccessKeyID     string
	AccessKeySecret string
	SignName        string
	TemplateCode    string
	ParamMode       string
	ParamMap        map[string]string
	APIURL          string
}

type ChuanglanConfig struct {
	Account      string
	Password     string
	APIURL       string
	TemplateID   string
	Signature    string
	Extend       string
	Report       bool
	AuthMode     string
	TemplateText string
	ParamMode    string
	ParamOrder   []string
}

func (c Config) Capability() CapabilityStatus {
	status := CapabilityStatus{
		DefaultProvider: strings.TrimSpace(c.DefaultProvider),
		Template:        strings.TrimSpace(c.Template),
		PreviewTemplate: strings.TrimSpace(c.PreviewTemplate),
	}
	if !c.Enabled || status.Template != TemplatePendingActionNotification {
		return status
	}

	allowed := make(map[string]struct{}, len(c.Providers))
	for _, provider := range c.Providers {
		provider = strings.ToLower(strings.TrimSpace(provider))
		if provider != "" {
			allowed[provider] = struct{}{}
		}
	}

	if _, ok := allowed[ProviderAliyun]; ok && c.Aliyun.valid() {
		status.Providers = append(status.Providers, ProviderAliyun)
	}
	if _, ok := allowed[ProviderChuanglan]; ok && c.Chuanglan.valid() {
		status.Providers = append(status.Providers, ProviderChuanglan)
	}
	for _, provider := range status.Providers {
		if provider == status.DefaultProvider {
			status.Enabled = true
			return status
		}
	}
	return status
}

func (c AliyunConfig) valid() bool {
	return strings.TrimSpace(c.AccessKeyID) != "" &&
		strings.TrimSpace(c.AccessKeySecret) != "" &&
		strings.TrimSpace(c.SignName) != "" &&
		strings.TrimSpace(c.TemplateCode) != "" &&
		strings.TrimSpace(c.ParamMode) == ParamModeMap &&
		hasRequiredNotificationParamsFromMap(c.ParamMap)
}

func (c ChuanglanConfig) valid() bool {
	return strings.TrimSpace(c.Account) != "" &&
		strings.TrimSpace(c.Password) != "" &&
		strings.TrimSpace(c.APIURL) != "" &&
		strings.TrimSpace(c.TemplateID) != "" &&
		strings.TrimSpace(c.TemplateText) != "" &&
		strings.TrimSpace(c.ParamMode) == ParamModeOrderedParam &&
		hasRequiredNotificationParamsFromList(c.ParamOrder)
}
