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
