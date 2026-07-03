package approval

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appconfig "github.com/zgiai/zgi/api/config"
	shortlinkcap "github.com/zgiai/zgi/api/internal/capabilities/shortlink"
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

type recordingEmailService struct {
	body string
}

func (s *recordingEmailService) SendEmail(to []string, subject, body string) error {
	_ = to
	_ = subject
	s.body = body
	return nil
}

func TestCreateRuntimeFormSendsApprovalEmailWithShortURL(t *testing.T) {
	db := newApprovalTestDB(t)
	restore := stubApprovalTokens(t, "emailTok")
	defer restore()
	restoreConfig := stubApprovalWebURL(t, "https://zgi.example.com")
	defer restoreConfig()

	emailService := &recordingEmailService{}
	service := NewServiceWithDependencies(db, emailService, nil, nil)
	webappDisabled := false

	_, err := service.CreateOrGetRuntimeForm(context.Background(), CreateRuntimeFormParams{
		TenantID:      "11111111-1111-1111-1111-111111111111",
		AppID:         "22222222-2222-2222-2222-222222222222",
		WorkflowRunID: "run-email-1",
		NodeID:        "approval-email-1",
		NodeTitle:     "人工审批",
		Rendered:      "请审批",
		Config: NodeConfig{
			Content: "请审批",
			Actions: []Action{{ID: "approve", Label: "通过"}},
			SubmitMethods: SubmitMethods{
				WebApp: WebAppSubmitMethod{Enabled: &webappDisabled},
				Email: EmailSubmitMethod{
					Enabled: true,
					Subject: "审批",
					Body:    "打开 {{#url#}}",
					Recipients: []EmailRecipient{{
						Type:  "external",
						Email: "approver@example.com",
					}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrGetRuntimeForm returned error: %v", err)
	}
	if !strings.Contains(emailService.body, "https://zgi.example.com/") {
		t.Fatalf("email body = %q, want short URL", emailService.body)
	}
	if strings.Contains(emailService.body, "/a/emailTok") {
		t.Fatalf("email body = %q, should not expose approval token URL", emailService.body)
	}
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
	if got := req.TemplateParams[notificationsms.TemplateParamLinkCode]; len(got) != 8 || got == "smsTok01" {
		t.Fatalf("link code param = %q, want 8-char short token", got)
	}

	var delivery Delivery
	if err := db.Where("delivery_method_type = ?", DeliveryTypeSMS).First(&delivery).Error; err != nil {
		t.Fatalf("load sms delivery: %v", err)
	}
	if delivery.SentAt == nil {
		t.Fatal("sms delivery sent_at is nil")
	}
}

func TestCreateRuntimeFormReusesOneShortLinkAcrossSubmitMethods(t *testing.T) {
	db := newApprovalTestDB(t)
	restore := stubApprovalTokens(t, "formTok1", "emailTok1", "smsTok01")
	defer restore()
	restoreConfig := stubApprovalWebURL(t, "https://zgi.example.com")
	defer restoreConfig()

	emailService := &recordingEmailService{}
	smsService := &recordingSMSService{enabled: true}
	service := NewServiceWithSenders(db, emailService, smsService)

	runtimeForm, err := service.CreateOrGetRuntimeForm(context.Background(), CreateRuntimeFormParams{
		TenantID:      "11111111-1111-1111-1111-111111111111",
		AppID:         "22222222-2222-2222-2222-222222222222",
		WorkflowRunID: "run-multi-channel-1",
		NodeID:        "approval-multi-channel-1",
		NodeTitle:     "人工审批",
		Rendered:      "请审批",
		Config: NodeConfig{
			Content: "请审批",
			Actions: []Action{{ID: "approve", Label: "通过"}},
			SubmitMethods: SubmitMethods{
				WebApp: WebAppSubmitMethod{Enabled: nil},
				Email: EmailSubmitMethod{
					Enabled: true,
					Subject: "审批",
					Body:    "打开 {{#url#}}",
					Recipients: []EmailRecipient{{
						Type:  "external",
						Email: "approver@example.com",
					}},
				},
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

	var links []shortlinkcap.ShortLink
	if err := db.Where("target_kind = ?", shortlinkcap.TargetKindApprovalForm).Find(&links).Error; err != nil {
		t.Fatalf("load approval short links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("approval short links = %d, want 1", len(links))
	}
	if links[0].ExpiresAt == nil || !links[0].ExpiresAt.Equal(runtimeForm.Form.ExpirationTime) {
		t.Fatalf("short link expires_at = %v, want form expiration %v", links[0].ExpiresAt, runtimeForm.Form.ExpirationTime)
	}

	shortToken := links[0].ShortToken
	if !strings.Contains(emailService.body, "https://zgi.example.com/"+shortToken) {
		t.Fatalf("email body = %q, want shared short token %q", emailService.body, shortToken)
	}
	if got := smsService.requests[0].TemplateParams[notificationsms.TemplateParamLinkCode]; got != shortToken {
		t.Fatalf("sms link code = %q, want shared short token %q", got, shortToken)
	}
}

func TestFormAccessTokenSubmitsWithoutRecipientAttributionAndRejectsRepeat(t *testing.T) {
	db := newApprovalTestDB(t)
	restore := stubApprovalTokens(t, "formTok1")
	defer restore()

	service := NewServiceWithDependencies(db, nil, nil, nil)
	runtimeForm, err := service.CreateOrGetRuntimeForm(context.Background(), CreateRuntimeFormParams{
		TenantID:      "11111111-1111-1111-1111-111111111111",
		AppID:         "22222222-2222-2222-2222-222222222222",
		WorkflowRunID: "run-form-token-1",
		NodeID:        "approval-form-token-1",
		NodeTitle:     "人工审批",
		Rendered:      "请审批",
		Config: NodeConfig{
			Content: "请审批",
			Actions: []Action{{ID: "approve", Label: "通过"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrGetRuntimeForm returned error: %v", err)
	}
	if runtimeForm.Payload.Token != runtimeForm.Form.AccessToken {
		t.Fatalf("payload token = %q, want form token %q", runtimeForm.Payload.Token, runtimeForm.Form.AccessToken)
	}
	var beforeSubmitLink shortlinkcap.ShortLink
	if err := db.Where("target_kind = ? AND target_token = ?", shortlinkcap.TargetKindApprovalForm, runtimeForm.Form.AccessToken).First(&beforeSubmitLink).Error; err != nil {
		t.Fatalf("load approval short link before submit: %v", err)
	}

	submitted, err := service.SubmitByToken(context.Background(), runtimeForm.Form.AccessToken, SubmitRequest{
		Action: "approve",
		Inputs: map[string]interface{}{},
	}, nil, nil)
	if err != nil {
		t.Fatalf("SubmitByToken returned error: %v", err)
	}
	if submitted.CompletedByRecipientID != nil {
		t.Fatalf("completed_by_recipient_id = %v, want nil for form token submit", *submitted.CompletedByRecipientID)
	}
	var afterSubmitLink shortlinkcap.ShortLink
	if err := db.Where("target_kind = ? AND target_token = ?", shortlinkcap.TargetKindApprovalForm, runtimeForm.Form.AccessToken).First(&afterSubmitLink).Error; err != nil {
		t.Fatalf("load approval short link after submit: %v", err)
	}
	if beforeSubmitLink.ExpiresAt == nil || afterSubmitLink.ExpiresAt == nil || !afterSubmitLink.ExpiresAt.Equal(*beforeSubmitLink.ExpiresAt) {
		t.Fatalf("short link expires_at after submit = %v, want unchanged %v", afterSubmitLink.ExpiresAt, beforeSubmitLink.ExpiresAt)
	}

	_, err = service.SubmitByToken(context.Background(), runtimeForm.Form.AccessToken, SubmitRequest{
		Action: "approve",
		Inputs: map[string]interface{}{},
	}, nil, nil)
	if !errors.Is(err, ErrFormAlreadySubmitted) {
		t.Fatalf("repeat submit error = %v, want ErrFormAlreadySubmitted", err)
	}
}

func TestCreateRuntimeFormRetriesWhenFormTokenCollidesWithLegacyRecipientToken(t *testing.T) {
	db := newApprovalTestDB(t)
	restore := stubApprovalTokens(t, "formTok1", "formTok2")
	defer restore()

	oldForm := Form{
		ID:              "old-form-1",
		TenantID:        "11111111-1111-1111-1111-111111111111",
		AppID:           "22222222-2222-2222-2222-222222222222",
		WorkflowRunID:   "run-old-token-collision",
		NodeID:          "approval-old-token-collision",
		NodeTitle:       "旧审批",
		AccessToken:     "oldForm1",
		FormDefinition:  `{"content":"请审批","actions":[{"id":"approve","label":"通过"}],"rendered_content":"请审批","default_values":{},"expiration_at":"2099-01-01T00:00:00Z"}`,
		RenderedContent: "请审批",
		Status:          FormStatusWaiting,
		ExpirationTime:  time.Now().Add(time.Hour),
	}
	oldDelivery := Delivery{
		ID:                 "old-delivery-1",
		FormID:             oldForm.ID,
		DeliveryMethodType: DeliveryTypeEmail,
		ChannelPayload:     "{}",
	}
	legacyToken := "formTok1"
	oldRecipient := Recipient{
		ID:               "old-recipient-1",
		FormID:           oldForm.ID,
		DeliveryID:       oldDelivery.ID,
		RecipientType:    RecipientTypeEmailExternal,
		RecipientPayload: `{"email":"old@example.com"}`,
		AccessToken:      &legacyToken,
	}
	if err := db.Create(&oldForm).Error; err != nil {
		t.Fatalf("create old form: %v", err)
	}
	if err := db.Create(&oldDelivery).Error; err != nil {
		t.Fatalf("create old delivery: %v", err)
	}
	if err := db.Create(&oldRecipient).Error; err != nil {
		t.Fatalf("create old recipient: %v", err)
	}

	service := NewServiceWithDependencies(db, nil, nil, nil)
	runtimeForm, err := service.CreateOrGetRuntimeForm(context.Background(), CreateRuntimeFormParams{
		TenantID:      "11111111-1111-1111-1111-111111111111",
		AppID:         "22222222-2222-2222-2222-222222222222",
		WorkflowRunID: "run-new-token-collision",
		NodeID:        "approval-new-token-collision",
		NodeTitle:     "新审批",
		Rendered:      "请审批",
		Config: NodeConfig{
			Content: "请审批",
			Actions: []Action{{ID: "approve", Label: "通过"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrGetRuntimeForm returned error: %v", err)
	}
	if runtimeForm.Form.AccessToken != "formTok2" {
		t.Fatalf("form access token = %q, want retry token formTok2", runtimeForm.Form.AccessToken)
	}

	var links []shortlinkcap.ShortLink
	if err := db.Where("target_kind = ?", shortlinkcap.TargetKindApprovalForm).Find(&links).Error; err != nil {
		t.Fatalf("load approval short links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("approval short links = %d, want 1 for new form", len(links))
	}
	if links[0].TargetToken != "formTok2" {
		t.Fatalf("short link target token = %q, want formTok2", links[0].TargetToken)
	}
}

func TestLegacyRecipientAccessTokenStillSubmitsAndAttributesRecipient(t *testing.T) {
	db := newApprovalTestDB(t)
	restore := stubApprovalTokens(t, "formTok1")
	defer restore()

	service := NewServiceWithDependencies(db, nil, nil, nil)
	runtimeForm, err := service.CreateOrGetRuntimeForm(context.Background(), CreateRuntimeFormParams{
		TenantID:      "11111111-1111-1111-1111-111111111111",
		AppID:         "22222222-2222-2222-2222-222222222222",
		WorkflowRunID: "run-legacy-token-1",
		NodeID:        "approval-legacy-token-1",
		NodeTitle:     "人工审批",
		Rendered:      "请审批",
		Config: NodeConfig{
			Content: "请审批",
			Actions: []Action{{ID: "approve", Label: "通过"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrGetRuntimeForm returned error: %v", err)
	}

	var recipient Recipient
	if err := db.Where("form_id = ?", runtimeForm.Form.ID).First(&recipient).Error; err != nil {
		t.Fatalf("load recipient: %v", err)
	}
	legacyToken := "legacy1"
	if err := db.Model(&Recipient{}).Where("id = ?", recipient.ID).Update("access_token", legacyToken).Error; err != nil {
		t.Fatalf("set legacy recipient token: %v", err)
	}

	payload, err := service.GetFormByToken(context.Background(), legacyToken)
	if err != nil {
		t.Fatalf("GetFormByToken legacy token returned error: %v", err)
	}
	if payload.Token != runtimeForm.Form.AccessToken {
		t.Fatalf("payload token = %q, want form token %q", payload.Token, runtimeForm.Form.AccessToken)
	}

	submitted, err := service.SubmitByToken(context.Background(), legacyToken, SubmitRequest{
		Action: "approve",
		Inputs: map[string]interface{}{},
	}, nil, nil)
	if err != nil {
		t.Fatalf("SubmitByToken legacy token returned error: %v", err)
	}
	if submitted.CompletedByRecipientID == nil || *submitted.CompletedByRecipientID != recipient.ID {
		t.Fatalf("completed_by_recipient_id = %v, want %s", submitted.CompletedByRecipientID, recipient.ID)
	}
}

func TestLegacyRecipientTokenOnMigratedFormKeepsRecipientAttributionAfterOpen(t *testing.T) {
	db := newApprovalTestDB(t)
	restore := stubApprovalTokens(t, "legacy1", "newForm1")
	defer restore()

	service := NewServiceWithDependencies(db, nil, nil, nil)
	form := Form{
		ID:              "old-form-1",
		TenantID:        "11111111-1111-1111-1111-111111111111",
		AppID:           "22222222-2222-2222-2222-222222222222",
		WorkflowRunID:   "run-migrated-token-1",
		NodeID:          "approval-migrated-token-1",
		NodeTitle:       "人工审批",
		AccessToken:     "",
		FormDefinition:  `{"content":"请审批","actions":[{"id":"approve","label":"通过"}],"rendered_content":"请审批","default_values":{},"expiration_at":"2099-01-01T00:00:00Z"}`,
		RenderedContent: "请审批",
		Status:          FormStatusWaiting,
		ExpirationTime:  time.Now().Add(time.Hour),
	}
	delivery := Delivery{
		ID:                 "old-delivery-1",
		FormID:             form.ID,
		DeliveryMethodType: DeliveryTypeEmail,
		ChannelPayload:     "{}",
	}
	legacyToken := "legacy1"
	recipient := Recipient{
		ID:               "old-recipient-1",
		FormID:           form.ID,
		DeliveryID:       delivery.ID,
		RecipientType:    RecipientTypeEmailExternal,
		RecipientPayload: `{"email":"approver@example.com"}`,
		AccessToken:      &legacyToken,
	}
	if err := db.Create(&form).Error; err != nil {
		t.Fatalf("create migrated form: %v", err)
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create migrated delivery: %v", err)
	}
	if err := db.Create(&recipient).Error; err != nil {
		t.Fatalf("create migrated recipient: %v", err)
	}

	payload, err := service.GetFormByToken(context.Background(), legacyToken)
	if err != nil {
		t.Fatalf("GetFormByToken legacy token returned error: %v", err)
	}
	if payload.Token == legacyToken {
		t.Fatalf("payload token reused legacy recipient token %q; want a distinct form token", legacyToken)
	}
	if payload.Token != "newForm1" {
		t.Fatalf("payload token = %q, want retry token newForm1", payload.Token)
	}

	submitted, err := service.SubmitByToken(context.Background(), legacyToken, SubmitRequest{
		Action: "approve",
		Inputs: map[string]interface{}{},
	}, nil, nil)
	if err != nil {
		t.Fatalf("SubmitByToken legacy token returned error: %v", err)
	}
	if submitted.CompletedByRecipientID == nil || *submitted.CompletedByRecipientID != recipient.ID {
		t.Fatalf("completed_by_recipient_id = %v, want %s", submitted.CompletedByRecipientID, recipient.ID)
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
	if got := smsService.requests[0].TemplateParams[notificationsms.TemplateParamLinkCode]; len(got) != 8 || got == "smsTok02" {
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
	if err := db.AutoMigrate(&Form{}, &Delivery{}, &Recipient{}, &shortlinkcap.ShortLink{}); err != nil {
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

func stubApprovalWebURL(t *testing.T, webURL string) func() {
	t.Helper()
	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		Console: appconfig.ConsoleConfig{WebURL: webURL},
	}
	return func() {
		appconfig.GlobalConfig = previous
	}
}
