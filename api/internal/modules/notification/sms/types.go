package sms

import (
	"context"
	"strings"
)

const (
	FeatureNotificationSMS            = "notification_sms"
	TemplatePendingActionNotification = "pending_action_notification"
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
	SendNotification(ctx context.Context, req Request, template TemplateConfig) (*Result, error)
}

type Request struct {
	Provider       string
	Phone          string
	Template       string
	TemplateParams map[string]string
	Source         string
	SourceID       string
}

type Result struct {
	Provider  string
	Accepted  bool
	MessageID string
	RawCode   string
}

type CapabilityStatus struct {
	Enabled         bool             `json:"enabled"`
	Providers       []string         `json:"providers"`
	DefaultProvider string           `json:"default_provider"`
	Template        string           `json:"template"`
	PreviewTemplate string           `json:"preview_template,omitempty"`
	Templates       []TemplateStatus `json:"templates,omitempty"`
}

type TemplateStatus struct {
	Key             string                `json:"key"`
	Name            string                `json:"name,omitempty"`
	PreviewTemplate string                `json:"preview_template,omitempty"`
	Params          []TemplateParamConfig `json:"params,omitempty"`
}

type Config struct {
	Enabled         bool
	Providers       []string
	DefaultProvider string
	Template        string
	Templates       []TemplateConfig
	ConfigError     string
	Aliyun          AliyunConfig
	Chuanglan       ChuanglanConfig
}

type TemplateConfig struct {
	Key             string                  `json:"key"`
	Name            string                  `json:"name,omitempty"`
	PreviewTemplate string                  `json:"preview_template,omitempty"`
	Params          []TemplateParamConfig   `json:"params,omitempty"`
	Aliyun          AliyunTemplateConfig    `json:"aliyun,omitempty"`
	Chuanglan       ChuanglanTemplateConfig `json:"chuanglan,omitempty"`
}

type TemplateParamConfig struct {
	Key       string `json:"key"`
	Label     string `json:"label,omitempty"`
	Required  *bool  `json:"required,omitempty"`
	MaxLength int    `json:"max_length,omitempty"`
	Pattern   string `json:"pattern,omitempty"`
}

func (p TemplateParamConfig) IsRequired() bool {
	return p.Required == nil || *p.Required
}

func boolPtr(value bool) *bool {
	return &value
}

type AliyunConfig struct {
	AccessKeyID     string
	AccessKeySecret string
	SignName        string
	APIURL          string
}

type AliyunTemplateConfig struct {
	TemplateCode string            `json:"template_code"`
	ParamMode    string            `json:"param_mode,omitempty"`
	ParamMap     map[string]string `json:"param_map,omitempty"`
}

type ChuanglanConfig struct {
	Account   string
	Password  string
	APIURL    string
	Signature string
	Extend    string
	Report    bool
}

type ChuanglanTemplateConfig struct {
	TemplateID   string   `json:"template_id"`
	TemplateText string   `json:"template_text,omitempty"`
	ParamMode    string   `json:"param_mode,omitempty"`
	ParamOrder   []string `json:"param_order,omitempty"`
}

func (c Config) Capability() CapabilityStatus {
	defaultTemplate := c.defaultTemplateKey()
	status := CapabilityStatus{
		DefaultProvider: strings.TrimSpace(c.DefaultProvider),
		Template:        defaultTemplate,
	}
	if tmpl, ok := c.TemplateByKey(defaultTemplate); ok {
		status.PreviewTemplate = strings.TrimSpace(tmpl.PreviewTemplate)
	}
	if !c.Enabled || c.ConfigError != "" {
		return status
	}

	allowed := make(map[string]struct{}, len(c.Providers))
	for _, provider := range c.Providers {
		provider = strings.ToLower(strings.TrimSpace(provider))
		if provider != "" {
			allowed[provider] = struct{}{}
		}
	}

	if _, ok := allowed[ProviderAliyun]; ok && c.Aliyun.credentialsValid() && c.hasProviderTemplate(ProviderAliyun) {
		status.Providers = append(status.Providers, ProviderAliyun)
	}
	if _, ok := allowed[ProviderChuanglan]; ok && c.Chuanglan.credentialsValid() && c.hasProviderTemplate(ProviderChuanglan) {
		status.Providers = append(status.Providers, ProviderChuanglan)
	}
	for _, provider := range status.Providers {
		if provider == status.DefaultProvider {
			status.Templates = c.capabilityTemplates(provider)
			status.Enabled = len(status.Templates) > 0
			return status
		}
	}
	return status
}

func (c Config) TemplateByKey(key string) (TemplateConfig, bool) {
	key = strings.TrimSpace(key)
	for _, template := range c.Templates {
		if template.Key == key {
			return template, true
		}
	}
	return TemplateConfig{}, false
}

func (c Config) defaultTemplateKey() string {
	if strings.TrimSpace(c.Template) != "" {
		return strings.TrimSpace(c.Template)
	}
	if len(c.Templates) > 0 {
		return c.Templates[0].Key
	}
	return ""
}

func (c Config) hasProviderTemplate(provider string) bool {
	for _, template := range c.Templates {
		if template.supportsProvider(provider) {
			return true
		}
	}
	return false
}

func (c Config) capabilityTemplates(provider string) []TemplateStatus {
	templates := make([]TemplateStatus, 0, len(c.Templates))
	for _, template := range c.Templates {
		if !template.supportsProvider(provider) {
			continue
		}
		templates = append(templates, TemplateStatus{
			Key:             template.Key,
			Name:            template.Name,
			PreviewTemplate: template.PreviewTemplate,
			Params:          template.Params,
		})
	}
	return templates
}

func (t TemplateConfig) supportsProvider(provider string) bool {
	switch provider {
	case ProviderAliyun:
		return t.Aliyun.valid(t.Params)
	case ProviderChuanglan:
		return t.Chuanglan.valid(t.Params)
	default:
		return false
	}
}

func (c AliyunConfig) credentialsValid() bool {
	return strings.TrimSpace(c.AccessKeyID) != "" &&
		strings.TrimSpace(c.AccessKeySecret) != "" &&
		strings.TrimSpace(c.SignName) != ""
}

func (c AliyunTemplateConfig) valid(params []TemplateParamConfig) bool {
	if strings.TrimSpace(c.TemplateCode) == "" || normalizedParamMode(c.ParamMode, ParamModeMap) != ParamModeMap {
		return false
	}
	for _, param := range params {
		if param.IsRequired() && strings.TrimSpace(c.ParamMap[param.Key]) == "" {
			return false
		}
	}
	return true
}

func (c ChuanglanConfig) credentialsValid() bool {
	return strings.TrimSpace(c.Account) != "" &&
		strings.TrimSpace(c.Password) != "" &&
		strings.TrimSpace(c.APIURL) != ""
}

func (c ChuanglanTemplateConfig) valid(params []TemplateParamConfig) bool {
	if strings.TrimSpace(c.TemplateID) == "" ||
		strings.TrimSpace(c.TemplateText) == "" ||
		normalizedParamMode(c.ParamMode, ParamModeOrderedParam) != ParamModeOrderedParam ||
		len(c.ParamOrder) == 0 {
		return false
	}
	allowed := make(map[string]struct{}, len(c.ParamOrder))
	for _, key := range c.ParamOrder {
		allowed[strings.TrimSpace(key)] = struct{}{}
	}
	for _, param := range params {
		if !param.IsRequired() {
			continue
		}
		if _, ok := allowed[param.Key]; !ok {
			return false
		}
	}
	return true
}

func normalizedParamMode(value string, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	return value
}
