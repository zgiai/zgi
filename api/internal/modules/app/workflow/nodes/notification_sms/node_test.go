package notification_sms

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	notificationsms "github.com/zgiai/zgi/api/internal/modules/notification/sms"
)

type recordingSMSService struct {
	request notificationsms.Request
}

func (s *recordingSMSService) IsEnabled() bool {
	return true
}

func (s *recordingSMSService) ValidateTemplateParams(string, map[string]string) error {
	return nil
}

func (s *recordingSMSService) Send(_ context.Context, req notificationsms.Request) (*notificationsms.Result, error) {
	s.request = req
	return &notificationsms.Result{Provider: req.Provider, Accepted: true, MessageID: "msg-1"}, nil
}

func TestNotificationSMSNodeUsesAnnouncementShortTokenVariableForLinkCode(t *testing.T) {
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"announcement_node", "token"}, "shrt1234")
	state := entities.NewGraphRuntimeState(variablePool)
	smsService := &recordingSMSService{}

	nodeInterface, err := New(
		"sms-instance",
		map[string]any{
			"id": "sms-node",
			"data": map[string]any{
				"type":     "notification-sms",
				"title":    "发送短信",
				"phone":    "18569919612",
				"provider": "aliyun",
				"template": notificationsms.TemplatePendingActionNotification,
				"template_params": map[string]string{
					notificationsms.TemplateParamNotificationTitle: "公告待办",
					notificationsms.TemplateParamLinkCode:          "{{#announcement_node.token#}}",
				},
			},
		},
		entities.GraphInitParams{WorkflowID: "workflow-1"},
		&entities.Graph{},
		state,
		nil,
		smsService,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	node, ok := nodeInterface.(*Node)
	if !ok {
		t.Fatalf("New() returned %T, want *Node", nodeInterface)
	}
	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", result.Status)
	}
	if got := smsService.request.TemplateParams[notificationsms.TemplateParamLinkCode]; got != "shrt1234" {
		t.Fatalf("link_code = %q, want short token", got)
	}
}
