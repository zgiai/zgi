package approval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	appconfig "github.com/zgiai/zgi/api/config"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	notificationsms "github.com/zgiai/zgi/api/internal/modules/notification/sms"
	"github.com/zgiai/zgi/api/pkg/email"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

const (
	defaultTimeoutDuration = 36
	defaultTimeoutUnit     = "hour"
	approvalPublicURLPath  = "/a/"
)

var actionIDPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var rawTemplatePlaceholderPattern = regexp.MustCompile(`\{\{#[^{}]+#\}\}`)

// EmailSender sends approval notification emails.
type EmailSender interface {
	SendEmail(to []string, subject, body string) error
}

type SMSSender interface {
	IsEnabled() bool
	ValidateTemplateParams(template string, params map[string]string) error
	Send(ctx context.Context, req notificationsms.Request) (*notificationsms.Result, error)
}

type emailSenderFunc func(to []string, subject, body string) error

func (f emailSenderFunc) SendEmail(to []string, subject, body string) error {
	return f(to, subject, body)
}

type Service struct {
	db          *gorm.DB
	emailSender EmailSender
	smsSender   SMSSender
}

func NewService(db *gorm.DB) *Service {
	return NewServiceWithEmailSender(db, emailSenderFunc(email.SendEmail))
}

// NewServiceWithEmailSender creates an approval service with an explicit email sender.
func NewServiceWithEmailSender(db *gorm.DB, sender EmailSender) *Service {
	return NewServiceWithSenders(db, sender, nil)
}

func NewServiceWithSenders(db *gorm.DB, emailSender EmailSender, smsSender SMSSender) *Service {
	sender := emailSender
	if sender == nil {
		sender = emailSenderFunc(email.SendEmail)
	}
	return &Service{db: db, emailSender: sender, smsSender: smsSender}
}

func (s *Service) CreateOrGetRuntimeForm(ctx context.Context, params CreateRuntimeFormParams) (*RuntimeForm, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("approval service is not initialized")
	}
	if err := validateRuntimeParams(params); err != nil {
		return nil, err
	}

	var existing Form
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND workflow_run_id = ? AND node_id = ?", params.TenantID, params.WorkflowRunID, params.NodeID).
		First(&existing).Error
	if err == nil {
		return s.runtimeFormPayload(ctx, &existing)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load approval form: %w", err)
	}

	form, recipients, deliveries, err := s.buildRuntimeForm(ctx, params)
	if err != nil {
		return nil, err
	}

	if err := s.createRuntimeFormWithTokenRetry(ctx, form, deliveries, recipients); err != nil {
		return nil, err
	}

	s.deliverApprovals(ctx, form, deliveries, recipients)

	return s.runtimeFormPayload(ctx, form)
}

func (s *Service) GetFormByToken(ctx context.Context, token string) (*FormPayload, error) {
	form, _, err := s.getFormAndRecipientByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if err := ensureFormReadable(form); err != nil {
		return nil, err
	}
	payload, err := s.formPayload(ctx, form)
	if err != nil {
		return nil, err
	}
	payload.Token = token
	return &payload, nil
}

func (s *Service) DebugAccessTokenByFormID(ctx context.Context, formID string) (string, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("approval service is not initialized")
	}
	var recipients []Recipient
	if err := s.db.WithContext(ctx).
		Where("form_id = ?", formID).
		Order("created_at ASC").
		Find(&recipients).Error; err != nil {
		return "", fmt.Errorf("load approval recipients: %w", err)
	}
	for _, recipient := range recipients {
		if recipient.AccessToken == "" {
			continue
		}
		if recipient.RecipientType == RecipientTypeWebApp || recipient.RecipientType == RecipientTypeConsole {
			return recipient.AccessToken, nil
		}
	}
	return s.createConsoleDebugRecipient(ctx, formID)
}

func (s *Service) createConsoleDebugRecipient(ctx context.Context, formID string) (string, error) {
	var form Form
	if err := s.db.WithContext(ctx).First(&form, "id = ?", formID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("load approval form: %w", err)
	}
	deliveryID := uuid.NewString()
	deliveryPayload, _ := json.Marshal(map[string]interface{}{"type": RecipientTypeConsole})
	recipientPayload, _ := json.Marshal(map[string]interface{}{"type": RecipientTypeConsole})
	var token string
	var createErr error
	for attempt := 0; attempt < tokenCreateMaxAttempts; attempt++ {
		generated, err := newApprovalToken()
		if err != nil {
			return "", err
		}
		token = generated
		createErr = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&Delivery{
				ID:                 deliveryID,
				FormID:             formID,
				DeliveryMethodType: DeliveryTypeWebApp,
				ChannelPayload:     string(deliveryPayload),
			}).Error; err != nil {
				return fmt.Errorf("create approval debug delivery: %w", err)
			}
			if err := tx.Create(&Recipient{
				ID:               uuid.NewString(),
				FormID:           formID,
				DeliveryID:       deliveryID,
				RecipientType:    RecipientTypeConsole,
				RecipientPayload: string(recipientPayload),
				AccessToken:      token,
			}).Error; err != nil {
				return fmt.Errorf("create approval debug recipient: %w", err)
			}
			return nil
		})
		if createErr == nil {
			return token, nil
		}
		if !isApprovalTokenConflict(createErr) {
			return "", createErr
		}
	}
	return "", fmt.Errorf("create approval debug recipient after token retries: %w", createErr)
}

func (s *Service) SubmitByToken(ctx context.Context, token string, req SubmitRequest, submissionUserID, submissionEndUserID *string) (*Form, error) {
	form, recipient, err := s.getFormAndRecipientByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if err := ensureFormSubmittable(form); err != nil {
		return nil, err
	}
	definition, err := decodeDefinition(form.FormDefinition)
	if err != nil {
		return nil, err
	}
	if err := validateSubmission(definition, req); err != nil {
		return nil, err
	}

	data, err := json.Marshal(req.Inputs)
	if err != nil {
		return nil, fmt.Errorf("marshal approval inputs: %w", err)
	}
	now := time.Now()
	updates := map[string]interface{}{
		"status":                    FormStatusSubmitted,
		"selected_action_id":        req.Action,
		"submitted_data":            string(data),
		"submitted_at":              now,
		"completed_by_recipient_id": recipient.ID,
	}
	if submissionUserID != nil {
		updates["submission_user_id"] = *submissionUserID
	}
	if submissionEndUserID != nil {
		updates["submission_end_user_id"] = *submissionEndUserID
	}

	result := s.db.WithContext(ctx).Model(&Form{}).
		Where("id = ? AND status = ?", form.ID, FormStatusWaiting).
		Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("submit approval form: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, ErrFormAlreadySubmitted
	}

	if err := s.db.WithContext(ctx).First(form, "id = ?", form.ID).Error; err != nil {
		return nil, fmt.Errorf("reload approval form: %w", err)
	}
	return form, nil
}

func (s *Service) ActivePauseApprovalFormsSubmitted(ctx context.Context, workflowRunID string) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("approval service is not initialized")
	}
	pauseService := workflowpause.NewService(s.db)
	_, reasons, _, err := pauseService.GetActiveByWorkflowRunID(ctx, workflowRunID)
	if err != nil {
		if errors.Is(err, workflowpause.ErrPauseNotFound) {
			return true, nil
		}
		return false, err
	}

	formIDs := make([]string, 0, len(reasons))
	seen := make(map[string]struct{}, len(reasons))
	for _, reason := range reasons {
		if reason.Type != workflowpause.ReasonTypeApprovalRequired || reason.FormID == "" {
			continue
		}
		if _, exists := seen[reason.FormID]; exists {
			continue
		}
		seen[reason.FormID] = struct{}{}
		formIDs = append(formIDs, reason.FormID)
	}
	if len(formIDs) == 0 {
		return true, nil
	}

	var submittedCount int64
	if err := s.db.WithContext(ctx).Model(&Form{}).
		Where("id IN ? AND status = ?", formIDs, FormStatusSubmitted).
		Count(&submittedCount).Error; err != nil {
		return false, fmt.Errorf("count submitted approval forms: %w", err)
	}
	return submittedCount == int64(len(formIDs)), nil
}

func (s *Service) AppendApprovalResultFilledEvent(ctx context.Context, form *Form) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("approval service is not initialized")
	}
	if form == nil || form.Status != FormStatusSubmitted {
		return nil
	}
	pauseService := workflowpause.NewService(s.db)
	pauseRecord, reasons, _, err := pauseService.GetActiveByWorkflowRunID(ctx, form.WorkflowRunID)
	if err != nil {
		if errors.Is(err, workflowpause.ErrPauseNotFound) {
			return nil
		}
		return err
	}
	if !activePauseHasFormID(reasons, form.ID) {
		return nil
	}
	eventData, err := approvalResultFilledEventData(form)
	if err != nil {
		return err
	}
	return pauseService.AppendEvent(ctx, workflowpause.AppendEventParams{
		TenantID:      pauseRecord.TenantID,
		AppID:         pauseRecord.AppID,
		WorkflowRunID: form.WorkflowRunID,
		EventType:     workflowpause.EventApprovalResultFilled,
		EventData:     eventData,
	})
}

func activePauseHasFormID(reasons []workflowpause.RunPauseReason, formID string) bool {
	for _, reason := range reasons {
		if reason.Type == workflowpause.ReasonTypeApprovalRequired && reason.FormID == formID {
			return true
		}
	}
	return false
}

func approvalResultFilledEventData(form *Form) (map[string]interface{}, error) {
	definition, err := decodeDefinition(form.FormDefinition)
	if err != nil {
		return nil, err
	}
	inputs := map[string]interface{}{}
	if form.SubmittedData != nil && *form.SubmittedData != "" {
		if err := json.Unmarshal([]byte(*form.SubmittedData), &inputs); err != nil {
			return nil, fmt.Errorf("decode approval submitted data: %w", err)
		}
	}
	actionID := ""
	if form.SelectedActionID != nil {
		actionID = *form.SelectedActionID
	}
	actionLabel := ""
	for _, action := range definition.Actions {
		if action.ID == actionID {
			actionLabel = action.Label
			break
		}
	}
	return map[string]interface{}{
		"form_id":          form.ID,
		"workflow_run_id":  form.WorkflowRunID,
		"node_id":          form.NodeID,
		"node_title":       form.NodeTitle,
		"action_id":        actionID,
		"action_label":     actionLabel,
		"inputs":           inputs,
		"rendered_content": form.RenderedContent,
	}, nil
}

func (s *Service) GetFormByID(ctx context.Context, formID string) (*Form, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("approval service is not initialized")
	}
	var form Form
	if err := s.db.WithContext(ctx).First(&form, "id = ?", formID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFormNotFound
		}
		return nil, fmt.Errorf("load approval form: %w", err)
	}
	return &form, nil
}

func (s *Service) MarkTimedOut(ctx context.Context, formID string) (*Form, error) {
	var form Form
	if err := s.db.WithContext(ctx).First(&form, "id = ?", formID).Error; err != nil {
		return nil, fmt.Errorf("load approval form: %w", err)
	}
	if form.Status != FormStatusWaiting {
		return &form, nil
	}
	if err := s.db.WithContext(ctx).Model(&form).Update("status", FormStatusTimeout).Error; err != nil {
		return nil, fmt.Errorf("mark approval timeout: %w", err)
	}
	form.Status = FormStatusTimeout
	return &form, nil
}

func (s *Service) TimeoutExpiredForms(ctx context.Context, limit int) ([]*Form, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("approval service is not initialized")
	}
	if limit <= 0 {
		limit = 100
	}

	var forms []Form
	if err := s.db.WithContext(ctx).
		Where("status = ? AND expiration_time <= ?", FormStatusWaiting, time.Now()).
		Order("expiration_time ASC").
		Limit(limit).
		Find(&forms).Error; err != nil {
		return nil, fmt.Errorf("load expired approval forms: %w", err)
	}

	timedOut := make([]*Form, 0, len(forms))
	for i := range forms {
		form, err := s.MarkTimedOut(ctx, forms[i].ID)
		if err != nil {
			return nil, err
		}
		if form.Status == FormStatusTimeout {
			timedOut = append(timedOut, form)
		}
	}
	return timedOut, nil
}

func (s *Service) ListRunEventsByToken(ctx context.Context, token string, afterSequence, limit int) (*RunEventsPayload, error) {
	form, _, err := s.getFormAndRecipientByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if afterSequence < 0 {
		afterSequence = 0
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	var pauseSequence int
	if err := s.db.WithContext(ctx).Model(&RunEvent{}).
		Where("workflow_run_id = ? AND event_type = ?", form.WorkflowRunID, "workflow_paused").
		Select("COALESCE(MAX(sequence), 0)").
		Scan(&pauseSequence).Error; err != nil {
		return nil, fmt.Errorf("load approval pause event sequence: %w", err)
	}
	minSequence := pauseSequence
	if afterSequence > minSequence {
		minSequence = afterSequence
	}

	var events []RunEvent
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND workflow_run_id = ? AND sequence > ?", form.TenantID, form.WorkflowRunID, minSequence).
		Order("sequence ASC").
		Limit(limit).
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load workflow run events: %w", err)
	}

	payload := &RunEventsPayload{
		WorkflowRunID: form.WorkflowRunID,
		Events:        make([]RunEventPayload, 0, len(events)),
	}
	for _, event := range events {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(event.EventData), &data); err != nil {
			return nil, fmt.Errorf("decode workflow run event %s: %w", event.ID, err)
		}
		data = sanitizeRunEventData(data)
		payload.Events = append(payload.Events, RunEventPayload{
			Sequence:  event.Sequence,
			Event:     event.EventType,
			Data:      data,
			CreatedAt: event.CreatedAt.Unix(),
		})
	}
	return payload, nil
}

func sanitizeRunEventData(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	output := make(map[string]interface{}, len(input))
	for key, value := range input {
		if isInternalRunEventKey(key) {
			continue
		}
		output[key] = sanitizeRunEventValue(value)
	}
	return output
}

func sanitizeRunEventValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return sanitizeRunEventData(typed)
	case []interface{}:
		output := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			output = append(output, sanitizeRunEventValue(item))
		}
		return output
	default:
		return value
	}
}

func isInternalRunEventKey(key string) bool {
	switch key {
	case "sys.workflow_resume_state",
		"sys.workflow_resume_pause_id",
		"workflow_resume_state",
		"workflow_resume_pause_id",
		"__approval_form",
		"__approval_form_id",
		"__approval_token":
		return true
	default:
		return false
	}
}

func (s *Service) buildRuntimeForm(ctx context.Context, params CreateRuntimeFormParams) (*Form, []Recipient, []Delivery, error) {
	formID := uuid.NewString()
	expiration := expirationTime(params.Config.Timeout)
	definition := FormDefinition{
		Content:       params.Config.Content,
		Fields:        params.Config.Fields,
		Actions:       params.Config.Actions,
		SubmitMethods: params.Config.SubmitMethods,
		Rendered:      params.Rendered,
		DefaultValues: params.DefaultValues,
		Title:         params.NodeTitle,
		ExpirationAt:  expiration,
	}
	definitionJSON, err := json.Marshal(definition)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal approval definition: %w", err)
	}

	form := &Form{
		ID:              formID,
		TenantID:        params.TenantID,
		AppID:           params.AppID,
		WorkflowRunID:   params.WorkflowRunID,
		NodeID:          params.NodeID,
		NodeTitle:       params.NodeTitle,
		FormDefinition:  string(definitionJSON),
		RenderedContent: params.Rendered,
		Status:          FormStatusWaiting,
		ExpirationTime:  expiration,
	}

	var recipients []Recipient
	var deliveries []Delivery
	if isWebAppEnabled(params.Config.SubmitMethods.WebApp) {
		delivery, deliveryRecipients, err := s.webAppDelivery(formID)
		if err != nil {
			return nil, nil, nil, err
		}
		deliveries = append(deliveries, delivery)
		recipients = append(recipients, deliveryRecipients...)
	}
	if params.Config.SubmitMethods.Email.Enabled {
		delivery, deliveryRecipients, err := s.emailDelivery(ctx, formID, params.Config.SubmitMethods.Email)
		if err != nil {
			return nil, nil, nil, err
		}
		deliveries = append(deliveries, delivery)
		recipients = append(recipients, deliveryRecipients...)
	}
	if params.Config.SubmitMethods.SMS.Enabled {
		delivery, deliveryRecipients, err := s.smsDelivery(ctx, formID, params.Config.SubmitMethods.SMS)
		if err != nil {
			return nil, nil, nil, err
		}
		deliveries = append(deliveries, delivery)
		recipients = append(recipients, deliveryRecipients...)
	}

	if len(recipients) == 0 {
		delivery, deliveryRecipients, err := s.webAppDelivery(formID)
		if err != nil {
			return nil, nil, nil, err
		}
		deliveries = append(deliveries, delivery)
		recipients = append(recipients, deliveryRecipients...)
	}

	return form, recipients, deliveries, nil
}

func (s *Service) webAppDelivery(formID string) (Delivery, []Recipient, error) {
	deliveryID := uuid.NewString()
	token, err := newApprovalToken()
	if err != nil {
		return Delivery{}, nil, err
	}
	deliveryPayload, _ := json.Marshal(map[string]interface{}{"type": DeliveryTypeWebApp})
	recipientPayload, _ := json.Marshal(map[string]interface{}{"type": RecipientTypeWebApp})
	return Delivery{
			ID:                 deliveryID,
			FormID:             formID,
			DeliveryMethodType: DeliveryTypeWebApp,
			ChannelPayload:     string(deliveryPayload),
		}, []Recipient{{
			ID:               uuid.NewString(),
			FormID:           formID,
			DeliveryID:       deliveryID,
			RecipientType:    RecipientTypeWebApp,
			RecipientPayload: string(recipientPayload),
			AccessToken:      token,
		}}, nil
}

func (s *Service) emailDelivery(ctx context.Context, formID string, cfg EmailSubmitMethod) (Delivery, []Recipient, error) {
	deliveryID := uuid.NewString()
	payload, err := json.Marshal(cfg)
	if err != nil {
		return Delivery{}, nil, fmt.Errorf("marshal email delivery config: %w", err)
	}
	recipients := make([]Recipient, 0, len(cfg.Recipients))
	for _, configured := range cfg.Recipients {
		resolved, err := s.resolveEmailRecipient(ctx, configured)
		if err != nil {
			logger.WarnContext(ctx, "failed to resolve approval email recipient", "type", configured.Type, err)
			continue
		}
		token, err := newApprovalToken()
		if err != nil {
			return Delivery{}, nil, err
		}
		recipientPayload, _ := json.Marshal(resolved.Payload)
		recipients = append(recipients, Recipient{
			ID:               uuid.NewString(),
			FormID:           formID,
			DeliveryID:       deliveryID,
			RecipientType:    resolved.Type,
			RecipientPayload: string(recipientPayload),
			AccessToken:      token,
		})
	}
	return Delivery{
		ID:                 deliveryID,
		FormID:             formID,
		DeliveryMethodType: DeliveryTypeEmail,
		ChannelPayload:     string(payload),
	}, recipients, nil
}

type resolvedRecipient struct {
	Type    string
	Email   string
	Phone   string
	Payload map[string]interface{}
}

func (s *Service) resolveEmailRecipient(ctx context.Context, recipient EmailRecipient) (*resolvedRecipient, error) {
	switch strings.TrimSpace(recipient.Type) {
	case "external":
		emailAddress := strings.TrimSpace(recipient.Email)
		if emailAddress == "" {
			return nil, fmt.Errorf("external email is required")
		}
		return &resolvedRecipient{
			Type:  RecipientTypeEmailExternal,
			Email: emailAddress,
			Payload: map[string]interface{}{
				"type":  RecipientTypeEmailExternal,
				"email": emailAddress,
			},
		}, nil
	case "member":
		accountID := strings.TrimSpace(recipient.AccountID)
		if accountID == "" {
			return nil, fmt.Errorf("member account_id is required")
		}
		var row struct {
			Email string
		}
		if err := s.db.WithContext(ctx).
			Table("accounts").
			Select("accounts.email").
			Where("accounts.id = ?", accountID).
			Scan(&row).Error; err != nil {
			return nil, fmt.Errorf("load member email: %w", err)
		}
		if strings.TrimSpace(row.Email) == "" {
			return nil, fmt.Errorf("member email not found")
		}
		return &resolvedRecipient{
			Type:  RecipientTypeEmailMember,
			Email: row.Email,
			Payload: map[string]interface{}{
				"type":       RecipientTypeEmailMember,
				"account_id": accountID,
				"email":      row.Email,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported recipient type: %s", recipient.Type)
	}
}

func (s *Service) smsDelivery(ctx context.Context, formID string, cfg SMSSubmitMethod) (Delivery, []Recipient, error) {
	if err := s.ensureSMSEnabled(); err != nil {
		return Delivery{}, nil, err
	}
	if err := s.validateApprovalSMSTemplateParams(cfg); err != nil {
		return Delivery{}, nil, err
	}
	deliveryID := uuid.NewString()
	payload, err := json.Marshal(cfg)
	if err != nil {
		return Delivery{}, nil, fmt.Errorf("marshal sms delivery config: %w", err)
	}
	recipients := make([]Recipient, 0, len(cfg.Recipients))
	for _, configured := range cfg.Recipients {
		resolved, err := s.resolveSMSRecipient(ctx, configured)
		if err != nil {
			return Delivery{}, nil, fmt.Errorf("resolve sms recipient: %w", err)
		}
		token, err := newApprovalToken()
		if err != nil {
			return Delivery{}, nil, err
		}
		recipientPayload, _ := json.Marshal(resolved.Payload)
		recipients = append(recipients, Recipient{
			ID:               uuid.NewString(),
			FormID:           formID,
			DeliveryID:       deliveryID,
			RecipientType:    resolved.Type,
			RecipientPayload: string(recipientPayload),
			AccessToken:      token,
		})
	}
	if len(recipients) == 0 {
		return Delivery{}, nil, fmt.Errorf("sms recipients are required")
	}
	return Delivery{
		ID:                 deliveryID,
		FormID:             formID,
		DeliveryMethodType: DeliveryTypeSMS,
		ChannelPayload:     string(payload),
	}, recipients, nil
}

func (s *Service) ensureSMSEnabled() error {
	if s == nil || s.smsSender == nil {
		return fmt.Errorf("approval sms service is not configured")
	}
	if !s.smsSender.IsEnabled() {
		return fmt.Errorf("notification sms is not enabled")
	}
	return nil
}

func (s *Service) resolveSMSRecipient(ctx context.Context, recipient SMSRecipient) (*resolvedRecipient, error) {
	switch strings.TrimSpace(recipient.Type) {
	case "external":
		phone := notificationsms.NormalizePhoneNumbers(recipient.Phone)
		if phone == "" {
			return nil, fmt.Errorf("external phone is required")
		}
		return &resolvedRecipient{
			Type:  RecipientTypeSMSExternal,
			Phone: phone,
			Payload: map[string]interface{}{
				"type":  RecipientTypeSMSExternal,
				"phone": phone,
			},
		}, nil
	case "member":
		accountID := strings.TrimSpace(recipient.AccountID)
		if accountID == "" {
			return nil, fmt.Errorf("member account_id is required")
		}
		var row struct {
			MobileE164 string `gorm:"column:mobile_e164"`
		}
		if err := s.db.WithContext(ctx).
			Table("accounts").
			Select("accounts.mobile_e164").
			Where("accounts.id = ?", accountID).
			Scan(&row).Error; err != nil {
			return nil, fmt.Errorf("load member phone: %w", err)
		}
		phone := notificationsms.NormalizePhoneNumbers(row.MobileE164)
		if phone == "" {
			return nil, fmt.Errorf("member phone not found")
		}
		return &resolvedRecipient{
			Type:  RecipientTypeSMSMember,
			Phone: phone,
			Payload: map[string]interface{}{
				"type":       RecipientTypeSMSMember,
				"account_id": accountID,
				"phone":      phone,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported recipient type: %s", recipient.Type)
	}
}

func (s *Service) deliverApprovals(ctx context.Context, form *Form, deliveries []Delivery, recipients []Recipient) {
	if form == nil {
		return
	}
	recipientsByDelivery := make(map[string][]Recipient)
	for _, recipient := range recipients {
		recipientsByDelivery[recipient.DeliveryID] = append(recipientsByDelivery[recipient.DeliveryID], recipient)
	}
	for _, delivery := range deliveries {
		switch delivery.DeliveryMethodType {
		case DeliveryTypeEmail:
			s.deliverEmailApproval(ctx, form, delivery, recipientsByDelivery[delivery.ID])
		case DeliveryTypeSMS:
			s.deliverSMSApproval(ctx, form, delivery, recipientsByDelivery[delivery.ID])
		}
	}
}

func (s *Service) deliverEmailApproval(ctx context.Context, form *Form, delivery Delivery, recipients []Recipient) {
	var cfg EmailSubmitMethod
	if err := json.Unmarshal([]byte(delivery.ChannelPayload), &cfg); err != nil {
		logger.WarnContext(ctx, "failed to decode approval email config", "delivery_id", delivery.ID, err)
		return
	}
	for _, recipient := range recipients {
		emailAddress := recipientEmail(recipient)
		if emailAddress == "" {
			continue
		}
		link := approvalURL(recipient.AccessToken)
		body := strings.ReplaceAll(cfg.Body, "{{#url#}}", link)
		if body == "" {
			body = link
		}
		subject := sanitizeSubject(cfg.Subject)
		if subject == "" {
			subject = "Approval request"
		}
		warnIfUnresolvedEmailTemplate(ctx, form, "subject", subject)
		warnIfUnresolvedEmailTemplate(ctx, form, "body", body)
		if err := s.sendApprovalEmail([]string{emailAddress}, subject, body); err != nil {
			s.recordDeliveryError(ctx, delivery.ID, err)
			logger.WarnContext(ctx, "failed to send approval email", "delivery_id", delivery.ID, "recipient_id", recipient.ID, err)
			continue
		}
		s.recordDeliverySent(ctx, delivery.ID)
	}
}

func (s *Service) deliverSMSApproval(ctx context.Context, form *Form, delivery Delivery, recipients []Recipient) {
	var cfg SMSSubmitMethod
	if err := json.Unmarshal([]byte(delivery.ChannelPayload), &cfg); err != nil {
		logger.WarnContext(ctx, "failed to decode approval sms config", "delivery_id", delivery.ID, err)
		return
	}
	for _, recipient := range recipients {
		phone := recipientPhone(recipient)
		if phone == "" {
			continue
		}
		req := notificationsms.Request{
			Provider:       strings.TrimSpace(cfg.Provider),
			Phone:          phone,
			Template:       strings.TrimSpace(cfg.Template),
			TemplateParams: approvalSMSTemplateParams(cfg, recipient.AccessToken),
			Source:         "workflow_approval",
			SourceID:       form.WorkflowRunID,
		}
		if err := s.sendApprovalSMS(ctx, req); err != nil {
			s.recordDeliveryError(ctx, delivery.ID, err)
			logger.WarnContext(ctx, "failed to send approval sms", "delivery_id", delivery.ID, "recipient_id", recipient.ID, "phone", notificationsms.MaskPhone(phone), err)
			continue
		}
		s.recordDeliverySent(ctx, delivery.ID)
	}
}

func (s *Service) sendApprovalEmail(to []string, subject, body string) error {
	if s == nil || s.emailSender == nil {
		return email.SendEmail(to, subject, body)
	}
	return s.emailSender.SendEmail(to, subject, body)
}

func (s *Service) sendApprovalSMS(ctx context.Context, req notificationsms.Request) error {
	if err := s.ensureSMSEnabled(); err != nil {
		return err
	}
	_, err := s.smsSender.Send(ctx, req)
	return err
}

func (s *Service) recordDeliveryError(ctx context.Context, deliveryID string, err error) {
	if err == nil {
		return
	}
	_ = s.db.WithContext(ctx).Model(&Delivery{}).Where("id = ?", deliveryID).Update("last_error", err.Error()).Error
}

func (s *Service) recordDeliverySent(ctx context.Context, deliveryID string) {
	now := time.Now()
	_ = s.db.WithContext(ctx).Model(&Delivery{}).Where("id = ?", deliveryID).Update("sent_at", now).Error
}

func warnIfUnresolvedEmailTemplate(ctx context.Context, form *Form, field, value string) {
	if form == nil || !rawTemplatePlaceholderPattern.MatchString(value) {
		return
	}
	logger.WarnContext(ctx, "approval email contains unresolved template placeholder",
		"workflow_run_id", form.WorkflowRunID,
		"form_id", form.ID,
		"node_id", form.NodeID,
		"field", field,
	)
}

func (s *Service) runtimeFormPayload(ctx context.Context, form *Form) (*RuntimeForm, error) {
	payload, err := s.formPayload(ctx, form)
	if err != nil {
		return nil, err
	}
	return &RuntimeForm{Form: form, Payload: payload}, nil
}

func (s *Service) formPayload(ctx context.Context, form *Form) (FormPayload, error) {
	definition, err := decodeDefinition(form.FormDefinition)
	if err != nil {
		return FormPayload{}, err
	}
	token := ""
	var recipient Recipient
	if err := s.db.WithContext(ctx).
		Where("form_id = ? AND recipient_type IN ?", form.ID, []string{RecipientTypeWebApp, RecipientTypeConsole}).
		Order("created_at ASC").
		First(&recipient).Error; err == nil {
		token = recipient.AccessToken
	}
	return FormPayload{
		ID:                    form.ID,
		Token:                 token,
		NodeID:                form.NodeID,
		NodeTitle:             form.NodeTitle,
		Content:               definition.Rendered,
		Fields:                definition.Fields,
		Actions:               definition.Actions,
		SubmitMethods:         definition.SubmitMethods,
		ResolvedDefaultValues: definition.DefaultValues,
		ExpirationAt:          form.ExpirationTime.Unix(),
	}, nil
}

func (s *Service) getFormAndRecipientByToken(ctx context.Context, token string) (*Form, *Recipient, error) {
	if s == nil || s.db == nil {
		return nil, nil, fmt.Errorf("approval service is not initialized")
	}
	var recipient Recipient
	if err := s.db.WithContext(ctx).First(&recipient, "access_token = ?", token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrFormNotFound
		}
		return nil, nil, fmt.Errorf("load approval recipient: %w", err)
	}
	var form Form
	if err := s.db.WithContext(ctx).First(&form, "id = ?", recipient.FormID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrFormNotFound
		}
		return nil, nil, fmt.Errorf("load approval form: %w", err)
	}
	return &form, &recipient, nil
}

func validateRuntimeParams(params CreateRuntimeFormParams) error {
	if strings.TrimSpace(params.TenantID) == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(params.AppID) == "" {
		return fmt.Errorf("app_id is required")
	}
	if strings.TrimSpace(params.WorkflowRunID) == "" {
		return fmt.Errorf("workflow_run_id is required")
	}
	if strings.TrimSpace(params.NodeID) == "" {
		return fmt.Errorf("node_id is required")
	}
	return ValidateConfig(params.Config)
}

func ValidateConfig(config NodeConfig) error {
	seenFields := make(map[string]struct{})
	for _, field := range config.Fields {
		key := strings.TrimSpace(field.Key)
		if key == "" {
			return fmt.Errorf("approval field key is required")
		}
		if _, exists := seenFields[key]; exists {
			return fmt.Errorf("duplicated approval field key: %s", key)
		}
		seenFields[key] = struct{}{}
		switch field.Type {
		case "", "text", "textarea":
		default:
			return fmt.Errorf("unsupported approval field type: %s", field.Type)
		}
	}

	if len(config.Actions) == 0 {
		return fmt.Errorf("approval actions are required")
	}
	seenActions := make(map[string]struct{})
	for _, action := range config.Actions {
		id := strings.TrimSpace(action.ID)
		if id == "" {
			return fmt.Errorf("approval action id is required")
		}
		if id == ActionExpired {
			return fmt.Errorf("approval action id %s is reserved", ActionExpired)
		}
		if !actionIDPattern.MatchString(id) {
			return fmt.Errorf("invalid approval action id: %s", id)
		}
		if _, exists := seenActions[id]; exists {
			return fmt.Errorf("duplicated approval action id: %s", id)
		}
		seenActions[id] = struct{}{}
	}
	if config.SubmitMethods.SMS.Enabled {
		if err := validateSMSSubmitMethod(config.SubmitMethods.SMS); err != nil {
			return err
		}
	}
	return nil
}

func validateSMSSubmitMethod(cfg SMSSubmitMethod) error {
	if strings.TrimSpace(cfg.NotificationTitle) == "" {
		return fmt.Errorf("approval sms notification_title is required")
	}
	if len(cfg.Recipients) == 0 {
		return fmt.Errorf("approval sms recipients are required")
	}
	for key := range cfg.TemplateParams {
		switch strings.TrimSpace(key) {
		case notificationsms.TemplateParamNotificationTitle, notificationsms.TemplateParamLinkCode, "link_suffix":
			return fmt.Errorf("approval sms template param %s is generated by the system", key)
		}
	}
	return nil
}

func (s *Service) validateApprovalSMSTemplateParams(cfg SMSSubmitMethod) error {
	template := strings.TrimSpace(cfg.Template)
	if template == "" {
		template = notificationsms.TemplatePendingActionNotification
	}
	if err := s.smsSender.ValidateTemplateParams(template, approvalSMSTemplateParams(cfg, "token")); err != nil {
		return fmt.Errorf("approval sms: %w", err)
	}
	return nil
}

func validateSubmission(definition FormDefinition, req SubmitRequest) error {
	actionFound := false
	for _, action := range definition.Actions {
		if action.ID == req.Action {
			actionFound = true
			break
		}
	}
	if !actionFound {
		return fmt.Errorf("invalid approval action: %s", req.Action)
	}
	if req.Inputs == nil {
		req.Inputs = map[string]interface{}{}
	}
	for _, field := range definition.Fields {
		if !field.Required {
			continue
		}
		if _, exists := req.Inputs[field.Key]; !exists {
			return fmt.Errorf("missing approval input: %s", field.Key)
		}
	}
	return nil
}

func ensureFormReadable(form *Form) error {
	if form == nil {
		return ErrFormNotFound
	}
	if form.Status == FormStatusSubmitted {
		return ErrFormAlreadySubmitted
	}
	if form.Status == FormStatusTimeout || form.Status == FormStatusExpired || time.Now().After(form.ExpirationTime) {
		return ErrFormExpired
	}
	return nil
}

func ensureFormSubmittable(form *Form) error {
	return ensureFormReadable(form)
}

func decodeDefinition(raw string) (FormDefinition, error) {
	var definition FormDefinition
	if err := json.Unmarshal([]byte(raw), &definition); err != nil {
		return FormDefinition{}, fmt.Errorf("decode approval definition: %w", err)
	}
	return definition, nil
}

func expirationTime(timeout TimeoutConfig) time.Time {
	duration := timeout.Duration
	if duration <= 0 {
		duration = defaultTimeoutDuration
	}
	unit := strings.TrimSpace(timeout.Unit)
	if unit == "" {
		unit = defaultTimeoutUnit
	}
	now := time.Now()
	switch unit {
	case "day", "days":
		return now.Add(time.Duration(duration) * 24 * time.Hour)
	default:
		return now.Add(time.Duration(duration) * time.Hour)
	}
}

func isWebAppEnabled(method WebAppSubmitMethod) bool {
	if method.Enabled == nil {
		return true
	}
	return *method.Enabled
}

func approvalURL(token string) string {
	base := strings.TrimRight(appconfig.Current().Console.WebURL, "/")
	if base == "" {
		base = strings.TrimRight(appconfig.Current().Email.ConsoleWebURL, "/")
	}
	return base + approvalPublicURLPath + url.PathEscape(token)
}

func recipientEmail(recipient Recipient) string {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(recipient.RecipientPayload), &payload); err != nil {
		return ""
	}
	emailAddress, _ := payload["email"].(string)
	return strings.TrimSpace(emailAddress)
}

func recipientPhone(recipient Recipient) string {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(recipient.RecipientPayload), &payload); err != nil {
		return ""
	}
	phone, _ := payload["phone"].(string)
	return notificationsms.NormalizePhoneNumbers(phone)
}

func approvalSMSTemplateParams(cfg SMSSubmitMethod, token string) map[string]string {
	params := make(map[string]string, len(cfg.TemplateParams)+2)
	for key, value := range cfg.TemplateParams {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			params[key] = value
		}
	}
	params[notificationsms.TemplateParamNotificationTitle] = strings.TrimSpace(cfg.NotificationTitle)
	params[notificationsms.TemplateParamLinkCode] = approvalLinkCode(token)
	return params
}

func approvalLinkCode(token string) string {
	return url.PathEscape(token)
}

func sanitizeSubject(subject string) string {
	subject = strings.ReplaceAll(subject, "\r", " ")
	subject = strings.ReplaceAll(subject, "\n", " ")
	return strings.Join(strings.Fields(subject), " ")
}

var (
	ErrFormNotFound         = errors.New("approval form not found")
	ErrFormAlreadySubmitted = errors.New("approval form already submitted")
	ErrFormExpired          = errors.New("approval form expired")
)
