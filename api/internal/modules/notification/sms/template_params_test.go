package sms

import (
	"encoding/json"
	"testing"
)

func TestAliyunPayloadUsesTemplateParams(t *testing.T) {
	provider := NewAliyunProvider(AliyunConfig{
		SignName:     "ZGI",
		TemplateCode: "SMS_123",
		ParamMode:    ParamModeMap,
		ParamMap: map[string]string{
			TemplateParamNotificationTitle: "title",
			TemplateParamLinkSuffix:        "link",
		},
	})

	payload, err := provider.BuildPayload(Request{
		Phone:    "13800138000",
		Template: TemplatePendingActionNotification,
		TemplateParams: map[string]string{
			TemplateParamNotificationTitle: "材料待人工智能",
			TemplateParamLinkSuffix:        "/a/AbC12345",
		},
	})
	if err != nil {
		t.Fatalf("BuildPayload returned error: %v", err)
	}

	var params map[string]string
	if err := json.Unmarshal([]byte(payload.TemplateParam), &params); err != nil {
		t.Fatalf("decode template params: %v", err)
	}
	if params["title"] != "材料待人工智能" {
		t.Fatalf("title param = %q", params["title"])
	}
	if params["link"] != "/a/AbC12345" {
		t.Fatalf("link param = %q", params["link"])
	}
}

func TestValidateNotificationTemplateParamsRejectsFullURL(t *testing.T) {
	err := ValidateNotificationTemplateParams(TemplatePendingActionNotification, map[string]string{
		TemplateParamNotificationTitle: "待处理任务",
		TemplateParamLinkSuffix:        "https://notice.example.com/a/AbC12345",
	})
	if err == nil {
		t.Fatal("expected full URL link_suffix to be rejected")
	}
}

func TestValidateNotificationTemplateParamsAllowsPathSuffix(t *testing.T) {
	err := ValidateNotificationTemplateParams(TemplatePendingActionNotification, map[string]string{
		TemplateParamNotificationTitle: "待处理任务",
		TemplateParamLinkSuffix:        "/a/AbC12345",
	})
	if err != nil {
		t.Fatalf("expected path suffix to be valid: %v", err)
	}
}
