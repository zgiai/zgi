// Package backgroundauth owns bounded, revocable authority for published
// employee connector invocations. Callers must first restore and authorize the
// live task identity; model arguments are never a source of Scope.
package backgroundauth

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Scope struct {
	OrganizationID     uuid.UUID `json:"organization_id" gorm:"type:uuid;not null;index"`
	OwnerID            uuid.UUID `json:"owner_id" gorm:"type:uuid;not null;index"`
	WorkspaceID        uuid.UUID `json:"workspace_id" gorm:"type:uuid;not null"`
	EmployeeID         uuid.UUID `json:"employee_id" gorm:"type:uuid;not null"`
	VersionID          uuid.UUID `json:"version_id" gorm:"type:uuid;not null"`
	BindingID          uuid.UUID `json:"binding_id" gorm:"type:uuid;not null;index"`
	ConnectionID       uuid.UUID `json:"connection_id" gorm:"type:uuid;not null"`
	BindingRevision    int       `json:"binding_revision" gorm:"not null"`
	ConnectionRevision int       `json:"connection_revision" gorm:"not null"`
	CredentialVersion  int       `json:"credential_version" gorm:"not null"`
	ContractDigest     string    `json:"contract_digest" gorm:"type:varchar(64);not null"`
}
type Grant struct {
	ID               uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	Scope            `gorm:"embedded"`
	OperationsJSON   string     `json:"-" gorm:"column:operations_json;type:text;not null"`
	DestinationsJSON string     `json:"-" gorm:"column:destinations_json;type:text;not null"`
	ExpiresAt        time.Time  `json:"expires_at" gorm:"not null"`
	MaxInvocations   int        `json:"max_invocations" gorm:"not null"`
	UsedInvocations  int        `json:"used_invocations" gorm:"not null;default:0"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at" gorm:"not null"`
}

func (Grant) TableName() string { return "connector_background_grants" }

type Consumption struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	GrantID            uuid.UUID `gorm:"type:uuid;not null;index"`
	BindingID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:background_invocation"`
	LogicalOperationID string    `gorm:"type:varchar(128);not null;uniqueIndex:background_invocation"`
	OperationID        string    `gorm:"type:varchar(128);not null"`
	CreatedAt          time.Time `gorm:"not null"`
}

func (Consumption) TableName() string { return "connector_background_consumptions" }

// RunBudget enforces the server-planned native Connector call ceiling for one
// restored DigitalHuman run. RunID and limits never come from tool arguments.
type RunBudget struct {
	RunID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID  uuid.UUID `gorm:"type:uuid;not null;index"`
	OwnerID         uuid.UUID `gorm:"type:uuid;not null"`
	WorkspaceID     uuid.UUID `gorm:"type:uuid;not null"`
	EmployeeID      uuid.UUID `gorm:"type:uuid;not null"`
	VersionID       uuid.UUID `gorm:"type:uuid;not null"`
	MaxInvocations  int       `gorm:"not null"`
	UsedInvocations int       `gorm:"not null;default:0"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (RunBudget) TableName() string { return "connector_background_run_budgets" }

type IssueRequest struct {
	Scope
	Operations     []string
	Destinations   map[string]string
	ExpiresAt      time.Time
	MaxInvocations int
}
type ConsumeRequest struct {
	Scope
	OperationID        string
	LogicalOperationID string
	Input              map[string]any `json:"-"`
	RunID              uuid.UUID      `json:"-"`
	MaxRunInvocations  int            `json:"-"`
}
type Receipt struct {
	GrantID       uuid.UUID
	ConsumptionID uuid.UUID
}
type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func New(db *gorm.DB, now func() time.Time) (*Service, error) {
	if db == nil {
		return nil, denied()
	}
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, now: now}, nil
}
func denied() error { return core.NewError(core.ErrorCodeAccessDenied, false, nil) }
func validScope(s Scope) bool {
	for _, id := range []uuid.UUID{s.OrganizationID, s.OwnerID, s.WorkspaceID, s.EmployeeID, s.VersionID, s.BindingID, s.ConnectionID} {
		if id == uuid.Nil {
			return false
		}
	}
	digest, err := hex.DecodeString(s.ContractDigest)
	return err == nil && len(digest) == 32 && strings.ToLower(s.ContractDigest) == s.ContractDigest && s.BindingRevision > 0 && s.ConnectionRevision > 0 && s.CredentialVersion > 0
}
func validName(v string, max int) bool {
	return v != "" && len(v) <= max && strings.TrimSpace(v) == v && !strings.ContainsAny(v, "\x00\r\n*")
}
func (s *Service) Issue(ctx context.Context, r IssueRequest) (Grant, error) {
	now := s.now().UTC()
	if !validScope(r.Scope) || !r.ExpiresAt.After(now) || r.ExpiresAt.After(now.Add(90*24*time.Hour)) || r.MaxInvocations < 1 || r.MaxInvocations > 100000 || len(r.Operations) < 1 || len(r.Operations) > 100 || len(r.Destinations) > 16 {
		return Grant{}, denied()
	}
	seen := map[string]bool{}
	for _, op := range r.Operations {
		if !core.Identifier(op).Valid() || seen[op] {
			return Grant{}, denied()
		}
		seen[op] = true
	}
	for key, value := range r.Destinations {
		if !validName(key, 64) || !validName(value, 512) {
			return Grant{}, denied()
		}
	}
	ops, err := json.Marshal(r.Operations)
	if err != nil {
		return Grant{}, denied()
	}
	destinations := r.Destinations
	if destinations == nil {
		destinations = map[string]string{}
	}
	dest, err := json.Marshal(destinations)
	if err != nil {
		return Grant{}, denied()
	}
	g := Grant{ID: uuid.New(), Scope: r.Scope, OperationsJSON: string(ops), DestinationsJSON: string(dest), ExpiresAt: r.ExpiresAt.UTC(), MaxInvocations: r.MaxInvocations, CreatedAt: now}
	if err = s.db.WithContext(ctx).Create(&g).Error; err != nil {
		return Grant{}, core.NewError(core.ErrorCodeInternal, false, err)
	}
	return g, nil
}
func (s *Service) Revoke(ctx context.Context, org, owner, id uuid.UUID) error {
	if org == uuid.Nil || owner == uuid.Nil || id == uuid.Nil {
		return denied()
	}
	now := s.now().UTC()
	result := s.db.WithContext(ctx).Model(&Grant{}).Where("id = ? AND organization_id = ? AND owner_id = ?", id, org, owner).Update("revoked_at", now)
	if result.Error != nil {
		return core.NewError(core.ErrorCodeInternal, false, result.Error)
	}
	if result.RowsAffected != 1 {
		return denied()
	}
	return nil
}
func (s *Service) List(ctx context.Context, org, owner uuid.UUID) ([]Grant, error) {
	if org == uuid.Nil || owner == uuid.Nil {
		return nil, denied()
	}
	var out []Grant
	err := s.db.WithContext(ctx).Where("organization_id = ? AND owner_id = ?", org, owner).Order("created_at DESC, id DESC").Limit(100).Find(&out).Error
	if err != nil {
		return nil, core.NewError(core.ErrorCodeInternal, false, err)
	}
	return out, nil
}
func (s *Service) Consume(ctx context.Context, r ConsumeRequest) (Receipt, error) {
	hasRunBudget := r.RunID != uuid.Nil || r.MaxRunInvocations != 0
	if !validScope(r.Scope) || !core.Identifier(r.OperationID).Valid() || !validName(r.LogicalOperationID, 128) ||
		hasRunBudget && (r.RunID == uuid.Nil || r.MaxRunInvocations < 1 || r.MaxRunInvocations > 100000) {
		return Receipt{}, denied()
	}
	var receipt Receipt
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now().UTC()
		var candidates []Grant
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND owner_id = ? AND binding_id = ? AND revoked_at IS NULL AND expires_at > ? AND used_invocations < max_invocations", r.OrganizationID, r.OwnerID, r.BindingID, now).Order("created_at, id").Limit(100).Find(&candidates).Error
		if err != nil {
			return err
		}
		for _, g := range candidates {
			if g.Scope != r.Scope {
				continue
			}
			var ops []string
			var destinations map[string]string
			if json.Unmarshal([]byte(g.OperationsJSON), &ops) != nil || json.Unmarshal([]byte(g.DestinationsJSON), &destinations) != nil {
				return denied()
			}
			allowed := false
			for _, op := range ops {
				if op == r.OperationID {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
			for field, value := range destinations {
				if !exactDestination(r.Input[field], value) {
					allowed = false
					break
				}
			}
			if !allowed {
				continue
			}
			if hasRunBudget {
				if err = consumeRunBudget(ctx, tx, r, now); err != nil {
					return err
				}
			}
			result := tx.Model(&Grant{}).Where("id = ? AND revoked_at IS NULL AND expires_at > ? AND used_invocations < max_invocations", g.ID, now).UpdateColumn("used_invocations", gorm.Expr("used_invocations + 1"))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				continue
			}
			use := Consumption{ID: uuid.New(), GrantID: g.ID, BindingID: r.BindingID, LogicalOperationID: r.LogicalOperationID, OperationID: r.OperationID, CreatedAt: now}
			if err = tx.Create(&use).Error; err != nil {
				return err
			}
			receipt = Receipt{GrantID: g.ID, ConsumptionID: use.ID}
			return nil
		}
		return denied()
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Receipt{}, err
		}
		return Receipt{}, denied()
	}
	return receipt, nil
}

func consumeRunBudget(ctx context.Context, tx *gorm.DB, request ConsumeRequest, now time.Time) error {
	budget := RunBudget{RunID: request.RunID, OrganizationID: request.OrganizationID, OwnerID: request.OwnerID,
		WorkspaceID: request.WorkspaceID, EmployeeID: request.EmployeeID, VersionID: request.VersionID,
		MaxInvocations: request.MaxRunInvocations, CreatedAt: now, UpdatedAt: now}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&budget).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", request.RunID).Take(&budget).Error; err != nil {
		return err
	}
	if budget.OrganizationID != request.OrganizationID || budget.OwnerID != request.OwnerID || budget.WorkspaceID != request.WorkspaceID ||
		budget.EmployeeID != request.EmployeeID || budget.VersionID != request.VersionID || budget.MaxInvocations != request.MaxRunInvocations {
		return denied()
	}
	result := tx.WithContext(ctx).Model(&RunBudget{}).Where("run_id = ? AND used_invocations < max_invocations", request.RunID).
		Updates(map[string]any{"used_invocations": gorm.Expr("used_invocations + 1"), "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return denied()
	}
	return nil
}

// Connector schemas use strings for chat destinations and arrays for mail
// recipients. An exact single value never authorizes additional recipients.
func exactDestination(actual any, expected string) bool {
	switch value := actual.(type) {
	case string:
		return value == expected
	case []string:
		return len(value) == 1 && value[0] == expected
	case []any:
		if len(value) != 1 {
			return false
		}
		text, ok := value[0].(string)
		return ok && text == expected
	default:
		return false
	}
}
