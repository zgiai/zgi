package bindings

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReferenceCommand is an explicit user decision. It never carries ownership,
// source constraints or an executable legacy identity from the client.
type ReferenceCommand = core.ReferenceCommand

type referenceRecord struct {
	ID                uuid.UUID `gorm:"primaryKey"`
	OrganizationID    uuid.UUID
	OwnerAccountID    uuid.UUID
	TargetType        string
	TargetID          string
	TargetScope       string
	TargetVersionID   string
	BindingInstanceID string
	FormerReferenceID string
	Label             string
	ReasonCode        string
	Status            string
	Revision          int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (referenceRecord) TableName() string { return "connector_unavailable_references" }
func (r referenceRecord) domain() core.UnavailableReference {
	return core.UnavailableReference{ID: r.ID, OrganizationID: r.OrganizationID, OwnerAccountID: r.OwnerAccountID,
		Target:            core.TargetRef{Type: core.TargetType(r.TargetType), ID: r.TargetID, Scope: core.TargetScope(r.TargetScope), VersionID: r.TargetVersionID, BindingInstanceID: r.BindingInstanceID},
		FormerReferenceID: r.FormerReferenceID, Label: r.Label, ReasonCode: r.ReasonCode, Status: r.Status, Revision: r.Revision}
}

// EnsureUnavailableReference records a host-owned, non-executable placement in
// the same transaction as the draft. The caller must authorize and lock the host.
func EnsureUnavailableReference(ctx context.Context, tx *gorm.DB, org, owner uuid.UUID, target core.TargetRef, former, label, reason string) (core.UnavailableReference, error) {
	if tx == nil || !validOwnerScope(org, owner) || target.Validate() != nil || target.EffectiveScope() == core.ScopePublished || strings.TrimSpace(former) == "" || len(former) > 255 || len(label) > 255 || len(reason) > 64 {
		return core.UnavailableReference{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	identity := strings.Join([]string{org.String(), owner.String(), string(target.Type), target.ID, string(target.EffectiveScope()), target.VersionID, target.BindingInstanceID, former}, "\x00")
	now := time.Now().UTC()
	r := referenceRecord{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("connector-unavailable:"+identity)), OrganizationID: org, OwnerAccountID: owner, TargetType: string(target.Type), TargetID: target.ID, TargetScope: string(target.EffectiveScope()), TargetVersionID: target.VersionID, BindingInstanceID: target.BindingInstanceID, FormerReferenceID: former, Label: label, ReasonCode: reason, Status: "needs_replacement", Revision: 1, CreatedAt: now, UpdatedAt: now}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&r).Error; err != nil {
		return core.UnavailableReference{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	if err := tx.WithContext(ctx).Where("id = ? AND organization_id = ? AND owner_account_id = ?", r.ID, org, owner).Take(&r).Error; err != nil {
		return core.UnavailableReference{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return r.domain(), nil
}

type ReferenceService struct {
	db      *gorm.DB
	targets TargetAuthority
}

func NewReferenceService(db *gorm.DB, targets TargetAuthority) *ReferenceService {
	return &ReferenceService{db, targets}
}

func referenceQuery(tx *gorm.DB, org, owner uuid.UUID, target TargetIdentity) *gorm.DB {
	scope := core.TargetRef{Type: target.Type, Scope: target.Scope}.EffectiveScope()
	return tx.Model(&referenceRecord{}).Where("organization_id = ? AND owner_account_id = ? AND target_type = ? AND target_id = ? AND target_scope = ? AND target_version_id = ?", org, owner, target.Type, target.ID, scope, target.VersionID)
}
func (s *ReferenceService) List(ctx context.Context, org, owner uuid.UUID, target TargetIdentity) ([]core.UnavailableReference, error) {
	if s == nil || s.db == nil || s.targets == nil || !validOwnerScope(org, owner) || target.Validate() != nil {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	var err error
	if target.Scope == core.ScopePublished {
		r, ok := s.targets.(TargetReadAuthority)
		if !ok {
			return nil, core.NewError(core.ErrorCodeAccessDenied, false, nil)
		}
		err = r.RequireTargetRead(ctx, org, owner, target)
	} else {
		err = s.targets.RequireTargetEdit(ctx, org, owner, target)
	}
	if err != nil {
		return nil, safeBindingDependencyError(err)
	}
	return ListReferencesInTransaction(ctx, s.db, org, owner, target)
}

// The host has already authorized this target and owns the surrounding save
// transaction. Published references remain immutable even through this port.
func ListReferencesInTransaction(ctx context.Context, tx *gorm.DB, org, owner uuid.UUID, target TargetIdentity) ([]core.UnavailableReference, error) {
	if tx == nil || !validOwnerScope(org, owner) || target.Validate() != nil {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	var rows []referenceRecord
	if err := referenceQuery(tx.WithContext(ctx), org, owner, target).Where("status = ?", "needs_replacement").Order("created_at,id").Find(&rows).Error; err != nil {
		return nil, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	result := make([]core.UnavailableReference, 0, len(rows))
	for _, r := range rows {
		result = append(result, r.domain())
	}
	return result, nil
}

func (s *ReferenceService) ResolveChat(ctx context.Context, org, owner uuid.UUID, target TargetIdentity, command ReferenceCommand) (*core.TargetBinding, error) {
	if s == nil || s.db == nil || s.targets == nil || target.Type != core.TargetTypeChat || target.Validate() != nil || !validOwnerScope(org, owner) {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	if err := s.targets.RequireTargetEdit(ctx, org, owner, target); err != nil {
		return nil, safeBindingDependencyError(err)
	}
	var result *core.TargetBinding
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = ResolveReferenceInTransaction(ctx, tx, org, owner, target, command)
		return err
	})
	return result, err
}

func ResolveReferenceInTransaction(ctx context.Context, tx *gorm.DB, org, owner uuid.UUID, target TargetIdentity, command ReferenceCommand) (*core.TargetBinding, error) {
	if tx == nil || !validOwnerScope(org, owner) || target.Validate() != nil || target.Scope == core.ScopePublished || command.ReferenceID == uuid.Nil || command.ExpectedRevision < 1 ||
		(command.Action != "replace" && command.Action != "detach") || command.Action == "replace" && command.ConnectionID == uuid.Nil || command.Action == "detach" && command.ConnectionID != uuid.Nil {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	var row referenceRecord
	err := referenceQuery(tx.WithContext(ctx), org, owner, target).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", command.ReferenceID).Take(&row).Error
	if err != nil {
		return nil, core.NewError(core.ErrorCodeNotFound, false, nil)
	}
	if row.Status != "needs_replacement" || row.Revision != command.ExpectedRevision {
		return nil, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	var result *core.TargetBinding
	status := "detached"
	if command.Action == "replace" {
		// Query only native non-secret connection metadata. The repository enforces
		// ownership, current readiness and the native connection FK again on write.
		var connection struct {
			ID          uuid.UUID
			ConnectorID string
			State       string
			Constraint  json.RawMessage `gorm:"column:connection_constraint"`
		}
		if err := tx.WithContext(ctx).Table("connector_connections").Select("id,connector_id,state,connection_constraint").Where("id = ? AND organization_id = ? AND owner_account_id = ? AND deleted_at IS NULL", command.ConnectionID, org, owner).Take(&connection).Error; err != nil || connection.State != string(core.ReadinessReady) {
			return nil, core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
		}
		id, err := core.NormalizeIdentifier(connection.ConnectorID)
		if err != nil {
			return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
		}
		connectionConstraint, err := core.DecodeConstraint(connection.Constraint)
		if err != nil {
			return nil, core.NewError(core.ErrorCodeAccessDenied, false, nil)
		}
		constraint, err := core.IntersectConstraints(NewBindingConstraint(owner, id), connectionConstraint)
		if err != nil {
			return nil, core.NewError(core.ErrorCodeAccessDenied, false, nil)
		}
		if target.Type == core.TargetTypeWorkflow && (!command.OperationID.Valid() || !constraint.Allows(owner, command.OperationID, row.domain().Target, time.Now())) {
			return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
		}
		now := time.Now().UTC()
		candidate := core.TargetBinding{ID: stableNativeBindingID(org, owner, id, row.domain().Target), OrganizationID: org, OwnerAccountID: owner, ConnectionID: connection.ID, ConnectorID: id, Target: row.domain().Target, Status: core.BindingStatusActive, Constraint: constraint, Revision: 1, CreatedAt: now, UpdatedAt: now}
		repository := NewGormRepository(tx)
		existing, err := repository.ListBindingsByTarget(ctx, org, owner, candidate.Target.Type, candidate.Target.ID, ListOptions{Scope: candidate.Target.EffectiveScope(), VersionID: candidate.Target.VersionID})
		if err != nil {
			return nil, err
		}
		expected := 0
		for _, binding := range existing {
			if binding.ConnectorID != id || binding.Target.BindingInstanceID != candidate.Target.BindingInstanceID {
				continue
			}
			if binding.Status == core.BindingStatusActive && binding.ConnectionID != candidate.ConnectionID {
				return nil, core.NewError(core.ErrorCodeConflict, false, nil)
			}
			candidate.ID = binding.ID
			candidate.Revision = binding.Revision
			expected = binding.Revision
		}
		stored, _, err := repository.UpsertBinding(ctx, org, owner, candidate, expected)
		if err != nil {
			return nil, err
		}
		result = &stored
		status = "explicitly_replaced"
	}
	updated := referenceQuery(tx.WithContext(ctx), org, owner, target).Where("id = ? AND revision = ? AND status = ?", row.ID, row.Revision, "needs_replacement").Updates(map[string]any{"status": status, "revision": row.Revision + 1, "updated_at": time.Now().UTC()})
	if updated.Error != nil || updated.RowsAffected != 1 {
		return nil, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	return result, nil
}
