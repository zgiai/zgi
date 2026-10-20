package socialops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"strings"
	"time"
)

// Outbound journals private replies and team handoffs separately from public
// publication archives. It records acceptance, never recipient read status.
type Outbound struct {
	ID                    uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	Scope                 `gorm:"embedded"`
	ScopeKey              string    `json:"-" gorm:"type:varchar(64);not null;uniqueIndex:scope_outbound,priority:1"`
	WorkKey               string    `json:"work_key" gorm:"type:varchar(128);not null;uniqueIndex:scope_outbound,priority:2"`
	ConfigurationRevision int64     `json:"configuration_revision" gorm:"not null"`
	Kind                  string    `json:"kind" gorm:"type:varchar(16);not null"`
	ConnectorID           string    `json:"connector_id" gorm:"type:varchar(128);not null"`
	ConnectionID          uuid.UUID `json:"connection_id" gorm:"type:uuid;not null"`
	OperationID           string    `json:"operation_id" gorm:"type:varchar(128);not null"`
	AccountID             string    `json:"account_id" gorm:"type:varchar(255);not null"`
	ConversationID        string    `json:"conversation_id,omitempty" gorm:"type:varchar(255);not null"`
	Destination           string    `json:"-" gorm:"type:varchar(512);not null"`
	BodyArtifactID        uuid.UUID `json:"body_artifact_id" gorm:"type:uuid;not null"`
	ReviewedDigest        string    `json:"-" gorm:"type:varchar(64);not null"`
	State                 string    `json:"state" gorm:"type:varchar(32);not null"`
	Revision              int64     `json:"revision" gorm:"not null"`
	ReceiptID             string    `json:"receipt_id,omitempty" gorm:"type:varchar(512);not null"`
	ReceiptStatus         string    `json:"receipt_status,omitempty" gorm:"type:varchar(64);not null"`
	CreatedAt             time.Time `json:"created_at" gorm:"not null"`
	UpdatedAt             time.Time `json:"updated_at" gorm:"not null"`
}

func (Outbound) TableName() string { return "social_operation_outbound" }

type OutboundRequest struct {
	Scope
	WorkKey, Kind, AccountID, ConversationID string
	ConnectionID, BodyArtifactID             uuid.UUID
}
type OutboundMessage struct {
	Text    string `json:"text"`
	Subject string `json:"subject,omitempty"`
	Chinese string `json:"chinese,omitempty"`
}
type OutboundReceipt struct {
	ID     string
	Status string
}
type OutboundGateway interface {
	SendOutbound(context.Context, Outbound, OutboundMessage) (OutboundReceipt, error)
}

func (s *Service) PrepareOutbound(ctx context.Context, r OutboundRequest) (Outbound, error) {
	if !validScope(r.Scope) || !validKey(r.WorkKey) {
		return Outbound{}, invalid()
	}
	cfg, _, err := s.LoadCurrentConfiguration(ctx, r.Scope)
	if err != nil {
		return Outbound{}, err
	}
	if _, err = s.ReadArtifact(ctx, r.Scope, r.BodyArtifactID); err != nil {
		return Outbound{}, err
	}
	now := s.now().UTC()
	out := Outbound{ID: uuid.New(), Scope: r.Scope, ScopeKey: scopeKey(r.Scope), WorkKey: r.WorkKey, ConfigurationRevision: cfg.Revision, Kind: r.Kind, BodyArtifactID: r.BodyArtifactID, State: "draft", Revision: 1, CreatedAt: now, UpdatedAt: now}
	switch r.Kind {
	case "handoff":
		if r.ConnectionID != uuid.Nil || r.AccountID != "" || r.ConversationID != "" {
			return Outbound{}, invalid()
		}
		out.ConnectorID, out.ConnectionID, out.OperationID, out.Destination = cfg.TeamHandoff.ConnectorID, cfg.TeamHandoff.ConnectionID, cfg.TeamHandoff.OperationID, cfg.TeamHandoff.Destination
	case "dm":
		if r.ConnectionID == uuid.Nil || r.AccountID == "" || len(r.ConversationID) > 255 || !validOpaqueDestination(r.ConversationID) {
			return Outbound{}, invalid()
		}
		found := false
		for _, b := range cfg.Platforms {
			if b.ConnectionID == r.ConnectionID && b.AccountID == r.AccountID && b.Platform != "linkedin" {
				out.ConnectorID, out.ConnectionID, out.AccountID, out.ConversationID, out.OperationID = b.ConnectorID, b.ConnectionID, b.AccountID, r.ConversationID, b.ConnectorID+".message.send"
				found = true
				break
			}
		}
		if !found {
			return Outbound{}, invalid()
		}
	default:
		return Outbound{}, invalid()
	}
	if _, err = s.outboundMessage(ctx, out); err != nil {
		return Outbound{}, err
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&out)
	if result.Error != nil {
		return Outbound{}, result.Error
	}
	if result.RowsAffected == 1 {
		return out, nil
	}
	var previous Outbound
	if err = scoped(s.db.WithContext(ctx), r.Scope).Where("work_key = ?", r.WorkKey).Take(&previous).Error; err != nil {
		return Outbound{}, err
	}
	if previous.BodyArtifactID != out.BodyArtifactID || previous.Kind != out.Kind || previous.ConnectionID != out.ConnectionID || previous.AccountID != out.AccountID || previous.ConversationID != out.ConversationID || previous.Destination != out.Destination || previous.OperationID != out.OperationID {
		return Outbound{}, conflict()
	}
	return previous, nil
}
func validOpaqueDestination(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n*")
}
func (s *Service) GetOutbound(ctx context.Context, scope Scope, id uuid.UUID) (Outbound, error) {
	if !validScope(scope) || id == uuid.Nil {
		return Outbound{}, invalid()
	}
	var out Outbound
	err := scoped(s.db.WithContext(ctx), scope).Where("id = ?", id).Take(&out).Error
	return out, err
}
func (s *Service) outboundMessage(ctx context.Context, out Outbound) (OutboundMessage, error) {
	data, err := s.ReadArtifact(ctx, out.Scope, out.BodyArtifactID)
	if err != nil {
		return OutboundMessage{}, err
	}
	var message OutboundMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&message) != nil || decoder.Decode(new(any)) != io.EOF || strings.TrimSpace(message.Text) == "" || len(message.Text) > 60000 || len(message.Subject) > 998 {
		return OutboundMessage{}, invalid()
	}
	if len(message.Chinese) > 60000 || (out.Kind == "dm" && strings.TrimSpace(message.Chinese) == "") {
		return OutboundMessage{}, invalid()
	}
	return message, nil
}
func (s *Service) outboundTransition(ctx context.Context, out Outbound, from, to string, fields map[string]any) (Outbound, error) {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["state"], fields["revision"], fields["updated_at"] = to, out.Revision+1, s.now().UTC()
	result := scoped(s.db.WithContext(ctx).Model(&Outbound{}), out.Scope).Where("id = ? AND revision = ? AND state = ?", out.ID, out.Revision, from).Updates(fields)
	if result.Error != nil {
		return Outbound{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Outbound{}, conflict()
	}
	return s.GetOutbound(ctx, out.Scope, out.ID)
}
func (s *Service) ReviewOutbound(ctx context.Context, scope Scope, id uuid.UUID, revision int64, digest string) (Outbound, error) {
	out, err := s.GetOutbound(ctx, scope, id)
	if err != nil {
		return Outbound{}, err
	}
	data, err := s.ReadArtifact(ctx, scope, out.BodyArtifactID)
	if err != nil {
		return Outbound{}, err
	}
	if out.Revision != revision || digest != hash(data) {
		return Outbound{}, conflict()
	}
	return s.outboundTransition(ctx, out, "draft", "reviewed", map[string]any{"reviewed_digest": digest})
}
func (s *Service) DispatchOutbound(ctx context.Context, scope Scope, id uuid.UUID, gateway OutboundGateway) (Outbound, error) {
	if gateway == nil {
		return Outbound{}, invalid()
	}
	out, err := s.GetOutbound(ctx, scope, id)
	if err != nil {
		return Outbound{}, err
	}
	if out.State == "completed" {
		return out, nil
	}
	if out.State != "reviewed" {
		return out, conflict()
	}
	message, err := s.outboundMessage(ctx, out)
	if err != nil {
		return out, err
	}
	data, err := s.ReadArtifact(ctx, scope, out.BodyArtifactID)
	if err != nil || hash(data) != out.ReviewedDigest {
		return out, conflict()
	}
	// Configuration changes serialize with admission, without keeping a DB lock
	// across the external request. A durable dispatching row survives a crash.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record Record
		if err := scoped(tx, scope).Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind = ? AND record_key = ?", "configuration", "current").Take(&record).Error; err != nil {
			return err
		}
		if record.Revision != out.ConfigurationRevision {
			return conflict()
		}
		local := &Service{db: tx, root: s.root, now: s.now}
		var transitionErr error
		out, transitionErr = local.outboundTransition(ctx, out, "reviewed", "dispatching", nil)
		return transitionErr
	})
	if err != nil {
		return out, err
	}
	receipt, sendErr := gateway.SendOutbound(ctx, out, message)
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if receipt.ID != "" && len(receipt.ID) <= 512 && validOpaqueDestination(receipt.ID) && validOpaqueDestination(receipt.Status) && len(receipt.Status) <= 64 {
		saved, saveErr := s.outboundTransition(persist, out, "dispatching", "completed", map[string]any{"receipt_id": receipt.ID, "receipt_status": receipt.Status})
		if saveErr != nil {
			return out, saveErr
		}
		return saved, sendErr
	}
	var notSubmitted *NotSubmittedError
	next := "outcome_unknown"
	if sendErr != nil && errors.As(sendErr, &notSubmitted) {
		next = "reviewed"
	}
	saved, saveErr := s.outboundTransition(persist, out, "dispatching", next, nil)
	if saveErr != nil {
		return out, saveErr
	}
	if sendErr == nil {
		sendErr = conflict()
	}
	return saved, sendErr
}
