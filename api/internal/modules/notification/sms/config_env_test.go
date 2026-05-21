package sms

import (
	"context"
	"testing"
)

func TestConfigFromLookupReturnsConfiguredTemplates(t *testing.T) {
	env := map[string]string{
		"NOTIFICATION_SMS_ENABLED":                  "true",
		"NOTIFICATION_SMS_PROVIDERS":                "aliyun",
		"NOTIFICATION_SMS_DEFAULT_PROVIDER":         "aliyun",
		"NOTIFICATION_SMS_TEMPLATE":                 "verify_code",
		"NOTIFICATION_SMS_ALIYUN_ACCESS_KEY_ID":     "ak",
		"NOTIFICATION_SMS_ALIYUN_ACCESS_KEY_SECRET": "sk",
		"NOTIFICATION_SMS_ALIYUN_SIGN_NAME":         "ZGI",
		"NOTIFICATION_SMS_TEMPLATES_JSON": `[
			{
				"key":"pending_action_notification",
				"name":"待办提醒",
				"preview_template":"任务 {{notification_title}}，链接 {{link_code}}",
				"params":[
					{"key":"notification_title","label":"通知标题","required":true},
					{"key":"link_code","label":"链接参数","required":true}
				],
				"aliyun":{"template_code":"SMS_PENDING","param_map":{"notification_title":"name","link_code":"code"}}
			},
			{
				"key":"verify_code",
				"name":"验证码",
				"preview_template":"验证码 {{code}}",
				"params":[{"key":"code","label":"验证码","required":true}],
				"aliyun":{"template_code":"SMS_CODE","param_map":{"code":"code"}}
			}
		]`,
	}

	cfg := ConfigFromLookup(func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	})
	capability := cfg.Capability()

	if !capability.Enabled {
		t.Fatalf("expected sms capability to be enabled")
	}
	if capability.Template != "verify_code" {
		t.Fatalf("expected default template verify_code, got %q", capability.Template)
	}
	if len(capability.Templates) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(capability.Templates))
	}
	if capability.Templates[1].Key != "verify_code" || capability.Templates[1].Params[0].Key != "code" {
		t.Fatalf("unexpected second template: %#v", capability.Templates[1])
	}
}

func TestConfigFromLookupRequiresTemplateCatalogWhenEnabled(t *testing.T) {
	cfg := ConfigFromLookup(func(key string) (string, bool) {
		env := map[string]string{
			"NOTIFICATION_SMS_ENABLED":          "true",
			"NOTIFICATION_SMS_PROVIDERS":        "aliyun",
			"NOTIFICATION_SMS_DEFAULT_PROVIDER": "aliyun",
		}
		value, ok := env[key]
		return value, ok
	})

	if cfg.ConfigError == "" {
		t.Fatalf("expected template catalog config error")
	}
	if cfg.Capability().Enabled {
		t.Fatalf("expected sms capability to be disabled without template catalog")
	}
}

func TestServiceSendUsesSelectedTemplate(t *testing.T) {
	provider := &captureProvider{name: ProviderAliyun}
	service := NewService(testSMSConfig(), provider)

	_, err := service.Send(context.Background(), Request{
		Phone:          "13800138000",
		Template:       "verify_code",
		TemplateParams: map[string]string{"code": "123456"},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if provider.template.Key != "verify_code" {
		t.Fatalf("expected provider to receive verify_code template, got %q", provider.template.Key)
	}
	if provider.request.TemplateParams["code"] != "123456" {
		t.Fatalf("expected template param code to be forwarded, got %#v", provider.request.TemplateParams)
	}
}

func TestServiceSendRejectsMissingTemplateParam(t *testing.T) {
	service := NewService(testSMSConfig(), &captureProvider{name: ProviderAliyun})

	_, err := service.Send(context.Background(), Request{
		Phone:          "13800138000",
		Template:       "verify_code",
		TemplateParams: map[string]string{},
	})
	if err == nil {
		t.Fatalf("expected missing template param error")
	}
}

func TestServiceSendRejectsMissingTemplate(t *testing.T) {
	service := NewService(testSMSConfig(), &captureProvider{name: ProviderAliyun})

	_, err := service.Send(context.Background(), Request{
		Phone:          "13800138000",
		TemplateParams: map[string]string{"code": "123456"},
	})
	if err == nil {
		t.Fatalf("expected missing template error")
	}
}

func TestServiceSendForwardsPendingTemplateParams(t *testing.T) {
	provider := &captureProvider{name: ProviderAliyun}
	service := NewService(testSMSConfig(), provider)

	_, err := service.Send(context.Background(), Request{
		Phone:          "13800138000",
		Template:       TemplatePendingActionNotification,
		TemplateParams: map[string]string{"notification_title": "审批待办", "link_code": "abc123"},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if provider.request.TemplateParams["notification_title"] != "审批待办" {
		t.Fatalf("expected notification_title to be forwarded, got %#v", provider.request.TemplateParams)
	}
	if provider.request.TemplateParams["link_code"] != "abc123" {
		t.Fatalf("expected link_code to be forwarded, got %#v", provider.request.TemplateParams)
	}
}

type captureProvider struct {
	name     string
	request  Request
	template TemplateConfig
}

func (p *captureProvider) Provider() string {
	return p.name
}

func (p *captureProvider) SendNotification(_ context.Context, req Request, template TemplateConfig) (*Result, error) {
	p.request = req
	p.template = template
	return &Result{Provider: p.name, Accepted: true, MessageID: "msg-1", RawCode: "OK"}, nil
}

func testSMSConfig() Config {
	return Config{
		Enabled:         true,
		Providers:       []string{ProviderAliyun},
		DefaultProvider: ProviderAliyun,
		Template:        TemplatePendingActionNotification,
		Aliyun: AliyunConfig{
			AccessKeyID:     "ak",
			AccessKeySecret: "sk",
			SignName:        "ZGI",
		},
		Templates: []TemplateConfig{
			{
				Key:  TemplatePendingActionNotification,
				Name: "待办提醒",
				Params: []TemplateParamConfig{
					{Key: "notification_title", Required: true, MaxLength: maxNotificationTitleRunes},
					{Key: "link_code", Required: true, Pattern: linkCodePattern.String()},
				},
				Aliyun: AliyunTemplateConfig{
					TemplateCode: "SMS_PENDING",
					ParamMode:    ParamModeMap,
					ParamMap:     map[string]string{"notification_title": "name", "link_code": "code"},
				},
			},
			{
				Key:    "verify_code",
				Name:   "验证码",
				Params: []TemplateParamConfig{{Key: "code", Required: true}},
				Aliyun: AliyunTemplateConfig{
					TemplateCode: "SMS_CODE",
					ParamMode:    ParamModeMap,
					ParamMap:     map[string]string{"code": "code"},
				},
			},
		},
	}
}
