package digitalhuman

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"gorm.io/datatypes"
)

// EndUserInstanceStatus is the operator-controlled state of one end user's
// relationship with a DigitalHuman.
type EndUserInstanceStatus string

const (
	EndUserInstanceActive    EndUserInstanceStatus = "active"
	EndUserInstanceSuspended EndUserInstanceStatus = "suspended"
	EndUserInstanceErased    EndUserInstanceStatus = "erased"
)

// EndUserInstance is one DigitalHuman's relationship with one external end
// user (ADR 0006). It has no configuration of its own: identity, rules, and
// bindings come from the published DigitalHuman. It is keyed by the existing
// user subject, which also keys that end user's conversations, files, tasks,
// and relationship memory, so the raw external user value is never stored.
type EndUserInstance struct {
	ID             uuid.UUID             `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID             `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID    uuid.UUID             `gorm:"type:uuid;not null" json:"workspace_id"`
	DigitalHumanID uuid.UUID             `gorm:"type:uuid;not null" json:"digital_human_id"`
	SubjectKind    string                `gorm:"type:varchar(32);not null" json:"subject_kind"`
	SubjectID      uuid.UUID             `gorm:"type:uuid;not null" json:"subject_id"`
	IssuerType     string                `gorm:"type:varchar(32);not null" json:"issuer_type"`
	IssuerID       *uuid.UUID            `gorm:"type:uuid" json:"issuer_id,omitempty"`
	Status         EndUserInstanceStatus `gorm:"type:varchar(16);not null" json:"status"`
	Label          *string               `gorm:"type:varchar(128)" json:"label,omitempty"`
	QuotaPolicy    datatypes.JSON        `gorm:"type:jsonb;not null" json:"quota_policy"`
	FirstSeenAt    time.Time             `gorm:"not null" json:"first_seen_at"`
	LastActiveAt   time.Time             `gorm:"not null" json:"last_active_at"`
	CreatedAt      time.Time             `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time             `gorm:"not null" json:"updated_at"`
}

// TableName binds instances to their PostgreSQL table.
func (EndUserInstance) TableName() string { return "digital_human_end_user_instances" }

// EndUserInstancesEnabled switches on end-user instances: runs of external end
// users are admitted through their instance. It defaults to off.
func EndUserInstancesEnabled() bool {
	return envFlagEnabled("ZGI_DIGITAL_HUMAN_INSTANCES_ENABLED")
}

// endUserInstanceStore admits a run together with its end user's instance.
// Stores without instance support keep admitting runs without one.
type endUserInstanceStore interface {
	CreateRunWithOutboxForEndUser(ctx context.Context, run *Run, outbox *RunOutbox, instance *EndUserInstance) error
}

// endUserInstanceForRun returns the instance a new run belongs to, or nil when
// the run is not an external end user's run with a durable subject. Anonymous
// WebApp sessions have no durable subject and get no instance.
func endUserInstanceForRun(scope Scope, run *Run, now time.Time) *EndUserInstance {
	if !EndUserInstancesEnabled() || run == nil || run.UserSubjectID == nil || *run.UserSubjectID == uuid.Nil ||
		RunSourceTrust(run) != SourceTrustExternalEndUser || run.UserSubjectKind == "" {
		return nil
	}
	instance := &EndUserInstance{
		ID: uuid.New(), OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, DigitalHumanID: run.DigitalHumanID,
		SubjectKind: run.UserSubjectKind, SubjectID: *run.UserSubjectID, IssuerType: run.Surface, Status: EndUserInstanceActive,
		QuotaPolicy: datatypes.JSON(`{}`), FirstSeenAt: now, LastActiveAt: now, CreatedAt: now, UpdatedAt: now,
	}
	switch run.Surface {
	case SurfaceAPI:
		instance.IssuerID = cloneUUIDPointer(&scope.PrincipalID)
	case SurfaceWebApp:
		instance.IssuerID = cloneUUIDPointer(scope.WebAppID)
	default:
		return nil
	}
	return instance
}

// CreateRunWithOutboxForEndUser records the end user's activity on their
// instance, creating it on first contact, and stores the run for that
// instance. Concurrent first requests resolve to one instance through the
// unique (DigitalHuman, subject) key. A suspended instance admits no run. An
// erased instance only keeps a tombstone, so a returning end user starts a new
// relationship in it.
func (r *GormRepository) CreateRunWithOutboxForEndUser(ctx context.Context, run *Run, outbox *RunOutbox, instance *EndUserInstance) error {
	if instance == nil {
		return r.CreateRunWithOutbox(ctx, run, outbox)
	}
	return r.Transaction(ctx, func(tx *GormRepository) error {
		// An API end user's subject is otherwise registered only once the run
		// executes; the instance needs it now. The issuer matches that later
		// registration, so both paths agree.
		subject := usersubject.EnsureRequest{Subject: usersubject.Ref{Kind: usersubject.Kind(instance.SubjectKind), ID: instance.SubjectID}}
		if subject.Subject.Kind == usersubject.APIEndUser {
			issuerID := instance.DigitalHumanID
			subject.IssuerType, subject.IssuerID = SurfaceAPI, &issuerID
		}
		if _, err := usersubject.NewService(tx.db).Ensure(ctx, subject); err != nil {
			return fmt.Errorf("admit DigitalHuman end user subject: %w", err)
		}
		var admitted EndUserInstance
		err := tx.db.WithContext(ctx).Raw(`
			INSERT INTO digital_human_end_user_instances (
				id, organization_id, workspace_id, digital_human_id, subject_kind, subject_id, issuer_type, issuer_id,
				status, quota_policy, first_seen_at, last_active_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (digital_human_id, subject_id) DO UPDATE SET
				status = CASE WHEN digital_human_end_user_instances.status = 'erased' THEN 'active' ELSE digital_human_end_user_instances.status END,
				first_seen_at = CASE WHEN digital_human_end_user_instances.status = 'erased' THEN EXCLUDED.first_seen_at ELSE digital_human_end_user_instances.first_seen_at END,
				last_active_at = GREATEST(digital_human_end_user_instances.last_active_at, EXCLUDED.last_active_at),
				updated_at = EXCLUDED.updated_at
			RETURNING *`,
			instance.ID, instance.OrganizationID, instance.WorkspaceID, instance.DigitalHumanID, instance.SubjectKind, instance.SubjectID,
			instance.IssuerType, instance.IssuerID, instance.Status, instance.QuotaPolicy, instance.FirstSeenAt, instance.LastActiveAt,
			instance.CreatedAt, instance.UpdatedAt,
		).Scan(&admitted).Error
		if err != nil {
			return fmt.Errorf("admit DigitalHuman end user instance: %w", err)
		}
		if admitted.ID == uuid.Nil || admitted.OrganizationID != run.OrganizationID || admitted.WorkspaceID != run.WorkspaceID {
			return fmt.Errorf("admit DigitalHuman end user instance: scope mismatch")
		}
		if admitted.Status == EndUserInstanceSuspended {
			return ErrEndUserSuspended
		}
		run.EndUserInstanceID = &admitted.ID
		if err := tx.CreateRun(ctx, run); err != nil {
			return err
		}
		return tx.CreateRunOutbox(ctx, outbox)
	})
}
