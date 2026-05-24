package approval

import (
	"context"
	"errors"
	"strings"
	"testing"

	notificationsms "github.com/zgiai/zgi/api/internal/modules/notification/sms"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type recordingSMSService struct {
	enabled     bool
	validateErr error
	validateFn  func(template string, params map[string]string) error
	requests    []notificationsms.Request
}

func (s *recordingSMSService) IsEnabled() bool {
	return s.enabled
}

func (s *recordingSMSService) ValidateTemplateParams(template string, params map[string]string) error {
	if s.validateFn != nil {
		return s.validateFn(template, params)
	}
	return s.validateErr
}

func (s *recordingSMSService) Send(ctx context.Context, req notificationsms.Request) (*notificationsms.Result, error) {
	_ = ctx
	s.requests = append(s.requests, req)
	return &notificationsms.Result{Provider: req.Provider, Accepted: true, MessageID: "msg-1"}, nil
}

func TestCreateRuntimeFormSendsApprovalSMSWithLinkCode(t *testing.T) {
	db := newApprovalTestDB(t)
	restore := stubApprovalTokens(t, "smsTok01")
	defer restore()

	smsService := &recordingSMSService{enabled: true}
	service := NewServiceWithSenders(db, nil, smsService)
	webappDisabled := false

	_, err := service.CreateOrGetRuntimeForm(context.Background(), CreateRuntimeFormParams{
		TenantID:      "11111111-1111-1111-1111-111111111111",
		AppID:         "22222222-2222-2222-2222-222222222222",
		WorkflowRunID: "run-1",
		NodeID:        "approval-1",
		NodeTitle:     "人工审批",
		Rendered:      "请审批",
		Config: NodeConfig{
			Content: "请审批",
			Actions: []Action{{ID: "approve", Label: "通过"}},
			SubmitMethods: SubmitMethods{
				WebApp: WebAppSubmitMethod{Enabled: &webappDisabled},
				SMS: SMSSubmitMethod{
					Enabled:           true,
					Provider:          notificationsms.ProviderAliyun,
					Template:          notificationsms.TemplatePendingActionNotification,
					NotificationTitle: "材料待人工智能",
					Recipients: []SMSRecipient{{
						Type:  "external",
						Phone: "13800138000",
					}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrGetRuntimeForm returned error: %v", err)
	}
	if len(smsService.requests) != 1 {
		t.Fatalf("sms requests = %d, want 1", len(smsService.requests))
	}

	req := smsService.requests[0]
	if req.TemplateParams[notificationsms.TemplateParamNotificationTitle] != "材料待人工智能" {
		t.Fatalf("notification title param = %q", req.TemplateParams[notificationsms.TemplateParamNotificationTitle])
	}
	if req.TemplateParams[notificationsms.TemplateParamLinkCode] != "smsTok01" {
		t.Fatalf("link code param = %q", req.TemplateParams[notificationsms.TemplateParamLinkCode])
	}

	var delivery Delivery
	if err := db.Where("delivery_method_type = ?", DeliveryTypeSMS).First(&delivery).Error; err != nil {
		t.Fatalf("load sms delivery: %v", err)
	}
	if delivery.SentAt == nil {
		t.Fatal("sms delivery sent_at is nil")
	}
}

func TestCreateRuntimeFormFailsWhenApprovalSMSTemplateParamsAreInvalid(t *testing.T) {
	db := newApprovalTestDB(t)
	smsService := &recordingSMSService{enabled: true, validateErr: errors.New("template param link_code is required")}
	service := NewServiceWithSenders(db, nil, smsService)
	webappDisabled := false

	_, err := service.CreateOrGetRuntimeForm(context.Background(), CreateRuntimeFormParams{
		TenantID:      "11111111-1111-1111-1111-111111111111",
		AppID:         "22222222-2222-2222-2222-222222222222",
		WorkflowRunID: "run-1",
		NodeID:        "approval-1",
		NodeTitle:     "人工审批",
		Rendered:      "请审批",
		Config: NodeConfig{
			Content: "请审批",
			Actions: []Action{{ID: "approve", Label: "通过"}},
			SubmitMethods: SubmitMethods{
				WebApp: WebAppSubmitMethod{Enabled: &webappDisabled},
				SMS: SMSSubmitMethod{
					Enabled:           true,
					NotificationTitle: "材料待人工智能",
					Recipients: []SMSRecipient{{
						Type:  "external",
						Phone: "13800138000",
					}},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid sms template params to fail")
	}
	if !strings.Contains(err.Error(), "approval sms") {
		t.Fatalf("error = %v", err)
	}
	if len(smsService.requests) != 0 {
		t.Fatalf("sms requests = %d, want 0", len(smsService.requests))
	}
}

func TestCreateRuntimeFormValidatesApprovalSMSTemplateParamsWithCustomParam(t *testing.T) {
	db := newApprovalTestDB(t)
	restore := stubApprovalTokens(t, "smsTok02")
	defer restore()

	smsService := &recordingSMSService{
		enabled: true,
		validateFn: func(template string, params map[string]string) error {
			if template != notificationsms.TemplatePendingActionNotification {
				t.Fatalf("template = %q", template)
			}
			if params[notificationsms.TemplateParamNotificationTitle] != "材料待人工智能" {
				t.Fatalf("notification title param = %q", params[notificationsms.TemplateParamNotificationTitle])
			}
			if params[notificationsms.TemplateParamLinkCode] != "token" {
				t.Fatalf("link code param = %q", params[notificationsms.TemplateParamLinkCode])
			}
			if params["remark"] != "请尽快处理" {
				t.Fatalf("remark param = %q", params["remark"])
			}
			if _, ok := params["link_suffix"]; ok {
				t.Fatalf("link_suffix should not be generated by the system")
			}
			return nil
		},
	}
	service := NewServiceWithSenders(db, nil, smsService)
	webappDisabled := false

	_, err := service.CreateOrGetRuntimeForm(context.Background(), CreateRuntimeFormParams{
		TenantID:      "11111111-1111-1111-1111-111111111111",
		AppID:         "22222222-2222-2222-2222-222222222222",
		WorkflowRunID: "run-1",
		NodeID:        "approval-1",
		NodeTitle:     "人工审批",
		Rendered:      "请审批",
		Config: NodeConfig{
			Content: "请审批",
			Actions: []Action{{ID: "approve", Label: "通过"}},
			SubmitMethods: SubmitMethods{
				WebApp: WebAppSubmitMethod{Enabled: &webappDisabled},
				SMS: SMSSubmitMethod{
					Enabled:           true,
					Provider:          notificationsms.ProviderAliyun,
					Template:          notificationsms.TemplatePendingActionNotification,
					NotificationTitle: "材料待人工智能",
					TemplateParams: map[string]string{
						"remark": "请尽快处理",
					},
					Recipients: []SMSRecipient{{
						Type:  "external",
						Phone: "13800138000",
					}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrGetRuntimeForm returned error: %v", err)
	}
	if len(smsService.requests) != 1 {
		t.Fatalf("sms requests = %d, want 1", len(smsService.requests))
	}
	if smsService.requests[0].TemplateParams["remark"] != "请尽快处理" {
		t.Fatalf("request remark param = %q", smsService.requests[0].TemplateParams["remark"])
	}
	if smsService.requests[0].TemplateParams[notificationsms.TemplateParamLinkCode] != "smsTok02" {
		t.Fatalf("request link code param = %q", smsService.requests[0].TemplateParams[notificationsms.TemplateParamLinkCode])
	}
}

func TestCreateRuntimeFormAllowsLinkSuffixAsNormalApprovalSMSTemplateParam(t *testing.T) {
	db := newApprovalTestDB(t)
	restore := stubApprovalTokens(t, "smsTok03")
	defer restore()

	smsService := &recordingSMSService{
		enabled: true,
		validateFn: func(_ string, params map[string]string) error {
			if params["link_suffix"] != "customSuffix" {
				t.Fatalf("link_suffix param = %q", params["link_suffix"])
			}
			return nil
		},
	}
	service := NewServiceWithSenders(db, nil, smsService)
	webappDisabled := false

	_, err := service.CreateOrGetRuntimeForm(context.Background(), CreateRuntimeFormParams{
		TenantID:      "11111111-1111-1111-1111-111111111111",
		AppID:         "22222222-2222-2222-2222-222222222222",
		WorkflowRunID: "run-1",
		NodeID:        "approval-1",
		NodeTitle:     "人工审批",
		Rendered:      "请审批",
		Config: NodeConfig{
			Content: "请审批",
			Actions: []Action{{ID: "approve", Label: "通过"}},
			SubmitMethods: SubmitMethods{
				WebApp: WebAppSubmitMethod{Enabled: &webappDisabled},
				SMS: SMSSubmitMethod{
					Enabled:           true,
					NotificationTitle: "材料待人工智能",
					TemplateParams: map[string]string{
						"link_suffix": "customSuffix",
					},
					Recipients: []SMSRecipient{{
						Type:  "external",
						Phone: "13800138000",
					}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrGetRuntimeForm returned error: %v", err)
	}
}

func TestCreateRuntimeFormFailsWhenSMSMemberHasNoPhone(t *testing.T) {
	db := newApprovalTestDB(t)
	service := NewServiceWithSenders(db, nil, &recordingSMSService{enabled: true})
	webappDisabled := false

	_, err := service.CreateOrGetRuntimeForm(context.Background(), CreateRuntimeFormParams{
		TenantID:      "11111111-1111-1111-1111-111111111111",
		AppID:         "22222222-2222-2222-2222-222222222222",
		WorkflowRunID: "run-1",
		NodeID:        "approval-1",
		NodeTitle:     "人工审批",
		Rendered:      "请审批",
		Config: NodeConfig{
			Content: "请审批",
			Actions: []Action{{ID: "approve", Label: "通过"}},
			SubmitMethods: SubmitMethods{
				WebApp: WebAppSubmitMethod{Enabled: &webappDisabled},
				SMS: SMSSubmitMethod{
					Enabled:           true,
					NotificationTitle: "材料待人工智能",
					Recipients: []SMSRecipient{{
						Type:      "member",
						AccountID: "33333333-3333-3333-3333-333333333333",
					}},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected missing member phone to fail")
	}
	if !strings.Contains(err.Error(), "member phone not found") {
		t.Fatalf("error = %v", err)
	}
}

func newApprovalTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Form{}, &Delivery{}, &Recipient{}); err != nil {
		t.Fatalf("migrate approval tables: %v", err)
	}
	if err := db.Exec("CREATE TABLE accounts (id text primary key, mobile_e164 text)").Error; err != nil {
		t.Fatalf("create accounts table: %v", err)
	}
	return db
}

func stubApprovalTokens(t *testing.T, tokens ...string) func() {
	t.Helper()

	original := newApprovalToken
	index := 0
	newApprovalToken = func() (string, error) {
		if index >= len(tokens) {
			return tokens[len(tokens)-1], nil
		}
		token := tokens[index]
		index++
		return token, nil
	}
	return func() {
		newApprovalToken = original
	}
}
