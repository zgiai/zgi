package orgaudit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository stores and lists audit records.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a repository on db.
func NewRepository(db *gorm.DB) *Repository {
	if db == nil {
		panic("org audit repository requires db")
	}
	return &Repository{db: db}
}

// Create inserts records.
func (r *Repository) Create(ctx context.Context, logs []Log) error {
	if len(logs) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Create(&logs).Error; err != nil {
		return fmt.Errorf("insert org audit logs: %w", err)
	}
	return nil
}

// Row is a record with the display names of its actor and target account.
type Row struct {
	Log
	ActorName   string `json:"actor_name"`
	ActorEmail  string `json:"actor_email"`
	TargetName  string `json:"target_name"`
	TargetEmail string `json:"target_email"`
}

// List returns one page of an organization's records, newest first, and the
// total matching the filter.
func (r *Repository) List(ctx context.Context, filter Filter) ([]Row, int64, error) {
	query := r.db.WithContext(ctx).Table("org_audit_logs AS l").Where("l.organization_id = ?", filter.OrganizationID)
	if filter.ActorAccountID != nil {
		query = query.Where("l.actor_account_id = ?", *filter.ActorAccountID)
	}
	if filter.TargetAccountID != nil {
		query = query.Where("l.target_account_id = ?", *filter.TargetAccountID)
	}
	if filter.Action != "" {
		query = query.Where("l.action = ?", filter.Action)
	}
	if filter.Result != "" {
		query = query.Where("l.result = ?", filter.Result)
	}
	if filter.From != nil {
		query = query.Where("l.created_at >= ?", filter.From.UTC())
	}
	if filter.To != nil {
		query = query.Where("l.created_at < ?", filter.To.UTC())
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count org audit logs: %w", err)
	}

	var rows []listRow
	err := query.
		Select(`l.*, COALESCE(actor.name, '') AS actor_name, COALESCE(actor.email, '') AS actor_email,
			COALESCE(target.name, '') AS target_name, COALESCE(target.email, '') AS target_email`).
		Joins("LEFT JOIN accounts AS actor ON actor.id = l.actor_account_id").
		Joins("LEFT JOIN accounts AS target ON target.id = l.target_account_id").
		Order("l.created_at DESC, l.id DESC").
		Offset((filter.Page - 1) * filter.Limit).
		Limit(filter.Limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list org audit logs: %w", err)
	}
	result := make([]Row, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toRow())
	}
	return result, total, nil
}

// listRow mirrors the joined columns flatly so Scan fills every field.
type listRow struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	WorkspaceID     *uuid.UUID
	ActorAccountID  *uuid.UUID
	TargetAccountID *uuid.UUID
	TargetType      string
	TargetID        string
	Action          Action
	Result          Result
	Before          []byte
	After           []byte
	RequestID       string
	IP              string `gorm:"column:ip"`
	CreatedAt       time.Time
	ActorName       string
	ActorEmail      string
	TargetName      string
	TargetEmail     string
}

func (r listRow) toRow() Row {
	return Row{
		Log: Log{
			ID: r.ID, OrganizationID: r.OrganizationID, WorkspaceID: r.WorkspaceID,
			ActorAccountID: r.ActorAccountID, TargetAccountID: r.TargetAccountID,
			TargetType: r.TargetType, TargetID: r.TargetID, Action: r.Action, Result: r.Result,
			Before: r.Before, After: r.After, RequestID: r.RequestID, IP: r.IP, CreatedAt: r.CreatedAt,
		},
		ActorName: r.ActorName, ActorEmail: r.ActorEmail, TargetName: r.TargetName, TargetEmail: r.TargetEmail,
	}
}
