package action

import (
	"testing"

	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
)

func TestBuildNotificationRequestAllowsSMSWithoutTemplateParams(t *testing.T) {
	req, err := buildNotificationRequest(map[string]interface{}{
		"channel_type": "sms",
		"to":           []interface{}{"13800138000"},
		"template":     "no_param_notice",
	})
	if err != nil {
		t.Fatalf("buildNotificationRequest() error = %v", err)
	}
	if req.ChannelType != automationmodel.NotificationChannelTypeSMS {
		t.Fatalf("expected sms channel, got %s", req.ChannelType)
	}
	if req.Template != "no_param_notice" {
		t.Fatalf("expected selected template, got %q", req.Template)
	}
	if len(req.TemplateParams) != 0 {
		t.Fatalf("expected empty template params, got %#v", req.TemplateParams)
	}
}

func TestBuildNotificationRequestReadsTemplateParams(t *testing.T) {
	req, err := buildNotificationRequest(map[string]interface{}{
		"channel_type": "sms",
		"to":           []interface{}{"13800138000"},
		"template":     "pending_action_notification",
		"template_params": map[string]interface{}{
			"notification_title": "审批待办",
			"link_code":          "abc123",
		},
	})
	if err != nil {
		t.Fatalf("buildNotificationRequest() error = %v", err)
	}
	if req.TemplateParams["notification_title"] != "审批待办" {
		t.Fatalf("expected notification_title to be read, got %#v", req.TemplateParams)
	}
	if req.TemplateParams["link_code"] != "abc123" {
		t.Fatalf("expected link_code to be read, got %#v", req.TemplateParams)
	}
}
