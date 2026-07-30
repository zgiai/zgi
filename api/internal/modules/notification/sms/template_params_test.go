package sms

import (
	"encoding/json"
	"testing"
)

func TestAliyunPayloadUsesTemplateParams(t *testing.T) {
	provider := NewAliyunProvider(AliyunConfig{
		SignName: "ZGI",
	})
	template := TemplateConfig{
		Key: TemplatePendingActionNotification,
		Params: []TemplateParamConfig{
			{Key: TemplateParamNotificationTitle, Required: boolPtr(true)},
			{Key: TemplateParamLinkCode, Required: boolPtr(true), Pattern: linkCodePattern.String()},
		},
		Aliyun: AliyunTemplateConfig{
			TemplateCode: "SMS_123",
			ParamMode:    ParamModeMap,
			ParamMap: map[string]string{
				TemplateParamNotificationTitle: "title",
				TemplateParamLinkCode:          "link",
			},
		},
	}

	payload, err := provider.BuildPayload(Request{
		Phone:    "13800138000",
		Template: TemplatePendingActionNotification,
		TemplateParams: map[string]string{
			TemplateParamNotificationTitle: "title",
			TemplateParamLinkCode:          "AbC12345",
		},
	}, template)
	if err != nil {
		t.Fatalf("BuildPayload returned error: %v", err)
	}

	var params map[string]string
	if err := json.Unmarshal([]byte(payload.TemplateParam), &params); err != nil {
		t.Fatalf("decode template params: %v", err)
	}
	if params["title"] != "title" {
		t.Fatalf("title param = %q", params["title"])
	}
	if params["link"] != "AbC12345" {
		t.Fatalf("link param = %q", params["link"])
	}
}

func TestChuanglanPayloadUsesDomesticPhoneNumber(t *testing.T) {
	provider := NewChuanglanProvider(ChuanglanConfig{
		Account:  "notification-account",
		Password: "notification-password",
		APIURL:   "https://example.com",
	})
	template := TemplateConfig{
		Key: TemplateAuthPhoneRegisterCode,
		Params: []TemplateParamConfig{
			{Key: TemplateParamVerificationCode, Required: boolPtr(true)},
		},
		Chuanglan: ChuanglanTemplateConfig{
			TemplateID:   "1022464742",
			TemplateText: "您的验证码为：{s}",
			ParamMode:    ParamModeOrderedParam,
			ParamOrder:   []string{TemplateParamVerificationCode},
		},
	}

	payload, err := provider.BuildPayload(Request{
		Phone:    "+8613800138000",
		Template: TemplateAuthPhoneRegisterCode,
		TemplateParams: map[string]string{
			TemplateParamVerificationCode: "123456",
		},
	}, template)
	if err != nil {
		t.Fatalf("BuildPayload returned error: %v", err)
	}
	if payload.PhoneNumbers != "13800138000" {
		t.Fatalf("phoneNumbers = %q, want %q", payload.PhoneNumbers, "13800138000")
	}
}

func TestChuanglanPayloadUsesTemplateCredentialProfile(t *testing.T) {
	provider := NewChuanglanProvider(ChuanglanConfig{
		Account:   "notification-account",
		Password:  "notification-password",
		APIURL:    "https://notification.example.com",
		Signature: "notification-signature",
		profiles: map[string]chuanglanCredentialConfig{
			chuanglanCredentialProfileVerification: {
				Account:   "verification-account",
				Password:  "verification-password",
				APIURL:    "https://verification.example.com",
				Signature: "verification-signature",
				Extend:    "123",
				Report:    true,
			},
		},
	})
	template := TemplateConfig{
		Key:    TemplateAuthPhoneRegisterCode,
		Params: []TemplateParamConfig{{Key: TemplateParamVerificationCode, Required: boolPtr(true)}},
		Chuanglan: ChuanglanTemplateConfig{
			CredentialProfile: chuanglanCredentialProfileVerification,
			TemplateID:        "CL_CODE",
			TemplateText:      "验证码：{s}",
			ParamMode:         ParamModeOrderedParam,
			ParamOrder:        []string{TemplateParamVerificationCode},
		},
	}

	payload, credentials, err := provider.buildPayload(Request{
		Phone:          "+8613800138000",
		Template:       TemplateAuthPhoneRegisterCode,
		TemplateParams: map[string]string{TemplateParamVerificationCode: "123456"},
	}, template)
	if err != nil {
		t.Fatalf("buildPayload returned error: %v", err)
	}
	if payload.Account != "verification-account" {
		t.Fatalf("Account = %q, want %q", payload.Account, "verification-account")
	}
	if payload.Password != "verification-password" {
		t.Fatalf("Password = %q, want verification profile password", payload.Password)
	}
	if payload.Signature != "verification-signature" {
		t.Fatalf("Signature = %q, want %q", payload.Signature, "verification-signature")
	}
	if payload.Extend != "123" || payload.Report != "true" {
		t.Fatalf("unexpected profile options: extend=%q report=%q", payload.Extend, payload.Report)
	}
	if credentials.APIURL != "https://verification.example.com" {
		t.Fatalf("APIURL = %q, want verification profile API URL", credentials.APIURL)
	}
}

func TestChuanglanPayloadWithoutCredentialProfileUsesNotificationAccount(t *testing.T) {
	provider := NewChuanglanProvider(ChuanglanConfig{
		Account:  "notification-account",
		Password: "notification-password",
		APIURL:   "https://notification.example.com",
		profiles: map[string]chuanglanCredentialConfig{
			chuanglanCredentialProfileVerification: {
				Account:  "verification-account",
				Password: "verification-password",
				APIURL:   "https://verification.example.com",
			},
		},
	})
	template := TemplateConfig{
		Key:    TemplatePendingActionNotification,
		Params: []TemplateParamConfig{{Key: TemplateParamNotificationTitle, Required: boolPtr(true)}},
		Chuanglan: ChuanglanTemplateConfig{
			TemplateID:   "CL_NOTICE",
			TemplateText: "待办：{s}",
			ParamMode:    ParamModeOrderedParam,
			ParamOrder:   []string{TemplateParamNotificationTitle},
		},
	}

	payload, err := provider.BuildPayload(Request{
		Phone:          "13800138000",
		Template:       TemplatePendingActionNotification,
		TemplateParams: map[string]string{TemplateParamNotificationTitle: "审批"},
	}, template)
	if err != nil {
		t.Fatalf("BuildPayload returned error: %v", err)
	}
	if payload.Account != "notification-account" {
		t.Fatalf("Account = %q, want %q", payload.Account, "notification-account")
	}
}

func TestNormalizeChuanglanDomesticPhoneNumbers(t *testing.T) {
	tests := []struct {
		name  string
		phone string
		want  string
	}{
		{name: "e164 mainland number", phone: "+8613800138000", want: "13800138000"},
		{name: "local mainland number", phone: "13800138000", want: "13800138000"},
		{
			name:  "multiple mainland numbers",
			phone: "+8613800138000, +8613900139000",
			want:  "13800138000,13900139000",
		},
		{name: "non-mainland number unchanged", phone: "+12025550123", want: "+12025550123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeChuanglanDomesticPhoneNumbers(tt.phone); got != tt.want {
				t.Fatalf("normalizeChuanglanDomesticPhoneNumbers() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateTemplateParamsRejectsInvalidLinkCode(t *testing.T) {
	err := ValidateTemplateParams(pendingActionTemplateForTest(), map[string]string{
		TemplateParamNotificationTitle: "待处理任务",
		TemplateParamLinkCode:          "/a/AbC12345",
	})
	if err == nil {
		t.Fatal("expected non-code link_code to be rejected")
	}
}

func TestValidateTemplateParamsAllowsLinkCode(t *testing.T) {
	err := ValidateTemplateParams(pendingActionTemplateForTest(), map[string]string{
		TemplateParamNotificationTitle: "待处理任务",
		TemplateParamLinkCode:          "AbC12345",
	})
	if err != nil {
		t.Fatalf("expected link code to be valid: %v", err)
	}
}

func TestValidateTemplateParamsRequiresLinkCode(t *testing.T) {
	err := ValidateTemplateParams(pendingActionTemplateForTest(), map[string]string{
		TemplateParamNotificationTitle: "待处理任务",
	})
	if err == nil {
		t.Fatal("expected missing link_code to be rejected")
	}
}

func pendingActionTemplateForTest() TemplateConfig {
	return TemplateConfig{
		Key: TemplatePendingActionNotification,
		Params: []TemplateParamConfig{
			{Key: TemplateParamNotificationTitle, Required: boolPtr(true), MaxLength: maxNotificationTitleRunes},
			{Key: TemplateParamLinkCode, Required: boolPtr(true), Pattern: linkCodePattern.String()},
		},
	}
}
