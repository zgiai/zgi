package runtimeworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Runtime Workspace end users may hold credits (ADR 0011). The integrating
// server grants them after its own payment; a negative grant takes credits
// back. Usage beyond the per-end-user limits that apply is drawn from the
// credits when it is settled: lazily, before admission and on reads, bill by
// bill, so settlement is safe to run again. Credits never pay for per-app
// caps, and end users without limits never spend them.

// Credit entry kinds.
const (
	CreditEntryGrant = "grant"
	CreditEntryUsage = "usage"
)

// EndUserEventCreditsAdjusted records a grant in the end user's history.
const EndUserEventCreditsAdjusted = "credits_adjusted"

const (
	// maxCreditGrant bounds one grant, in milli-points.
	maxCreditGrant = int64(1_000_000_000_000)
	// creditSettleBatch bounds the bills one settlement handles.
	creditSettleBatch = 500
)

// ErrCreditConflict reports an external ID reused for another amount.
var ErrCreditConflict = errors.New("runtime workspace credit grant reuses an external id with another amount")

// CreditLot is credit granted at once, drawn down until it is used or
// expires.
type CreditLot struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey"`
	RuntimeWorkspaceID uuid.UUID  `gorm:"type:uuid;not null"`
	SubjectID          uuid.UUID  `gorm:"type:uuid;not null"`
	Amount             int64      `gorm:"not null"`
	Remaining          int64      `gorm:"not null"`
	ExpiresAt          *time.Time `gorm:""`
	CreatedAt          time.Time
}

func (CreditLot) TableName() string { return "runtime_workspace_end_user_credit_lots" }

// CreditEntry is one grant or draw in an end user's credit ledger.
type CreditEntry struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey"`
	RuntimeWorkspaceID uuid.UUID  `gorm:"type:uuid;not null"`
	SubjectID          uuid.UUID  `gorm:"type:uuid;not null"`
	Kind               string     `gorm:"type:varchar(16);not null"`
	Amount             int64      `gorm:"not null"`
	BalanceAfter       int64      `gorm:"not null"`
	BillID             *uuid.UUID `gorm:"type:uuid"`
	ExternalID         string     `gorm:"type:varchar(128);not null;default:''"`
	Reason             string     `gorm:"type:varchar(255);not null;default:''"`
	ExpiresAt          *time.Time
	ActorKind          string     `gorm:"type:varchar(16);not null;default:''"`
	ActorID            *uuid.UUID `gorm:"type:uuid"`
	CreatedAt          time.Time
}

func (CreditEntry) TableName() string { return "runtime_workspace_end_user_credit_entries" }

// CreditEntryView is a ledger entry in points.
type CreditEntryView struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	Points       float64    `json:"points"`
	BalanceAfter float64    `json:"balance_after"`
	ExternalID   string     `json:"external_id,omitempty"`
	Reason       string     `json:"reason,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	ActorKind    string     `json:"actor_kind,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func creditEntryView(entry CreditEntry) CreditEntryView {
	return CreditEntryView{
		ID: entry.ID.String(), Kind: entry.Kind,
		Points:       pointsOf(millipoints.FromInt64(entry.Amount)),
		BalanceAfter: pointsOf(millipoints.FromInt64(entry.BalanceAfter)),
		ExternalID:   entry.ExternalID, Reason: entry.Reason, ExpiresAt: entry.ExpiresAt,
		ActorKind: entry.ActorKind, CreatedAt: entry.CreatedAt,
	}
}

// CreditLotView is the unused part of a lot.
type CreditLotView struct {
	Points    float64    `json:"points"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreditBalanceView is an end user's credits: the balance, debt when usage
// outran them, and the unexpired lots when asked for.
type CreditBalanceView struct {
	Balance float64         `json:"balance"`
	Debt    float64         `json:"debt,omitempty"`
	Lots    []CreditLotView `json:"lots,omitempty"`
}

// CreditGrant adds credits, or takes them back when Points is negative.
// ExternalID, such as the integrator's order ID, makes the grant
// idempotent.
type CreditGrant struct {
	Points     decimal.Decimal `json:"points"`
	ExternalID string          `json:"external_id"`
	Reason     string          `json:"reason"`
	ExpiresAt  *time.Time      `json:"expires_at"`
}

// CreditGrantResult is the recorded grant and the balance after it.
// Duplicate is set when the external ID was already granted.
type CreditGrantResult struct {
	Entry     CreditEntryView   `json:"entry"`
	Credits   CreditBalanceView `json:"credits"`
	Duplicate bool              `json:"duplicate"`
}

// ParseCreditGrant reads a grant body.
func ParseCreditGrant(body []byte) (CreditGrant, error) {
	var grant CreditGrant
	if err := json.Unmarshal(body, &grant); err != nil {
		return CreditGrant{}, ErrInvalidEndUser
	}
	return grant, nil
}

// GrantCredits records a grant to an end user. Earlier usage is settled
// first; a positive grant pays any debt before it adds a lot, and a
// negative one draws lots down and leaves debt when they do not cover it.
func (s *Service) GrantCredits(ctx context.Context, workspaceID, subjectID uuid.UUID, actor EndUserActor, grant CreditGrant) (*CreditGrantResult, error) {
	amount := millipoints.FromDecimalPoints(grant.Points).Int64()
	externalID, reason := strings.TrimSpace(grant.ExternalID), strings.TrimSpace(grant.Reason)
	now := s.now()
	if amount == 0 || amount > maxCreditGrant || -amount > maxCreditGrant ||
		externalID == "" || len(externalID) > 128 || len([]rune(reason)) > 255 ||
		grant.ExpiresAt != nil && (amount < 0 || !grant.ExpiresAt.After(now)) {
		return nil, ErrInvalidEndUser
	}
	var result *CreditGrantResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		runtimeWorkspace, user, err := s.loadEndUser(ctx, tx, workspaceID, subjectID, true)
		if err != nil {
			return err
		}
		if user.Status == EndUserErased {
			return ErrEndUserNotFound
		}
		var existing []CreditEntry
		if err := tx.Where("subject_id = ? AND external_id = ?", subjectID, externalID).Limit(1).Find(&existing).Error; err != nil {
			return fmt.Errorf("load credit grant: %w", err)
		}
		if len(existing) == 1 {
			if existing[0].Kind != CreditEntryGrant || existing[0].Amount != amount {
				return ErrCreditConflict
			}
			credits, _, err := creditBalance(ctx, tx, user, now, false)
			if err != nil {
				return err
			}
			result = &CreditGrantResult{Entry: creditEntryView(existing[0]), Credits: credits, Duplicate: true}
			return nil
		}
		if user.CreditsSince == nil {
			// Usage before the first grant is never charged.
			if err := tx.Model(&EndUser{}).Where("subject_id = ?", subjectID).Update("credits_since", now).Error; err != nil {
				return fmt.Errorf("start end-user credits: %w", err)
			}
			user.CreditsSince = &now
		}
		if err := settleEndUserCredits(ctx, tx, EndUserQuota(runtimeWorkspace.PolicyJSON), user, now); err != nil {
			return err
		}
		debt := user.CreditDebt
		if amount > 0 {
			paid := min(debt, amount)
			debt -= paid
			if rest := amount - paid; rest > 0 {
				lot := CreditLot{ID: uuid.New(), RuntimeWorkspaceID: runtimeWorkspace.LogicalID(), SubjectID: subjectID,
					Amount: rest, Remaining: rest, ExpiresAt: grant.ExpiresAt, CreatedAt: now}
				if err := tx.Create(&lot).Error; err != nil {
					return fmt.Errorf("add credit lot: %w", err)
				}
			}
		} else {
			uncovered, err := drawCreditLots(ctx, tx, subjectID, -amount, now)
			if err != nil {
				return err
			}
			debt += uncovered
		}
		if debt != user.CreditDebt {
			if err := tx.Model(&EndUser{}).Where("subject_id = ?", subjectID).Update("credit_debt", debt).Error; err != nil {
				return fmt.Errorf("update end-user credit debt: %w", err)
			}
			user.CreditDebt = debt
		}
		credits, balance, err := creditBalance(ctx, tx, user, now, false)
		if err != nil {
			return err
		}
		entry := CreditEntry{ID: uuid.New(), RuntimeWorkspaceID: runtimeWorkspace.LogicalID(), SubjectID: subjectID,
			Kind: CreditEntryGrant, Amount: amount, BalanceAfter: balance,
			ExternalID: externalID, Reason: reason, ExpiresAt: grant.ExpiresAt, ActorKind: actor.Kind, CreatedAt: now}
		if actor.ID != uuid.Nil {
			id := actor.ID
			entry.ActorID = &id
		}
		if err := tx.Create(&entry).Error; err != nil {
			return fmt.Errorf("record credit grant: %w", err)
		}
		raw, _ := json.Marshal(map[string]any{"points": json.Number(grant.Points.String())})
		event := EndUserEvent{ID: uuid.New(), RuntimeWorkspaceID: runtimeWorkspace.LogicalID(), SubjectID: subjectID,
			Action: EndUserEventCreditsAdjusted, ActorKind: actor.Kind, ActorID: entry.ActorID, Details: raw, CreatedAt: now}
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("record end-user event: %w", err)
		}
		result = &CreditGrantResult{Entry: creditEntryView(entry), Credits: credits}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// drawCreditLots takes an amount from an end user's unexpired lots, soonest
// expiry first, and returns what they did not cover.
func drawCreditLots(ctx context.Context, tx *gorm.DB, subjectID uuid.UUID, amount int64, now time.Time) (int64, error) {
	var lots []CreditLot
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("subject_id = ? AND remaining > 0 AND (expires_at IS NULL OR expires_at > ?)", subjectID, now).
		Order("expires_at ASC NULLS LAST").Order("created_at").Order("id").Find(&lots).Error; err != nil {
		return 0, fmt.Errorf("load credit lots: %w", err)
	}
	for _, lot := range lots {
		if amount == 0 {
			break
		}
		take := min(lot.Remaining, amount)
		if err := tx.WithContext(ctx).Model(&CreditLot{}).Where("id = ?", lot.ID).Update("remaining", lot.Remaining-take).Error; err != nil {
			return 0, fmt.Errorf("draw credit lot: %w", err)
		}
		amount -= take
	}
	return amount, nil
}

// creditBalance is an end user's unexpired credits less their debt, with
// the lots when asked for.
func creditBalance(ctx context.Context, db *gorm.DB, user *EndUser, now time.Time, withLots bool) (CreditBalanceView, int64, error) {
	var lots []CreditLot
	if err := db.WithContext(ctx).Where("subject_id = ? AND remaining > 0 AND (expires_at IS NULL OR expires_at > ?)", user.SubjectID, now).
		Order("expires_at ASC NULLS LAST").Order("created_at").Find(&lots).Error; err != nil {
		return CreditBalanceView{}, 0, fmt.Errorf("load credit lots: %w", err)
	}
	var total int64
	view := CreditBalanceView{}
	for _, lot := range lots {
		total += lot.Remaining
		if withLots {
			view.Lots = append(view.Lots, CreditLotView{Points: pointsOf(millipoints.FromInt64(lot.Remaining)), ExpiresAt: lot.ExpiresAt})
		}
	}
	view.Balance = pointsOf(millipoints.FromInt64(total - user.CreditDebt))
	view.Debt = pointsOf(millipoints.FromInt64(user.CreditDebt))
	return view, total - user.CreditDebt, nil
}

// settleEndUserCredits draws credits for the end user's bills not yet
// settled, in order. A bill is charged the part of it beyond the limits that
// apply, by the window it exceeds most; each settled bill is marked once.
// The end user's row must be locked.
func settleEndUserCredits(ctx context.Context, tx *gorm.DB, policy EndUserQuotaPolicy, user *EndUser, now time.Time) error {
	if user == nil || user.CreditsSince == nil {
		return nil
	}
	db := tx.WithContext(ctx)
	var bills []struct {
		ID               uuid.UUID
		TotalPoints      int64
		RequestCreatedAt time.Time
	}
	if err := db.Raw(`
		SELECT b.id, b.total_points, b.request_created_at FROM llm_usage_bills b
		WHERE b.end_user_subject_id = ? AND b.request_created_at >= ?
			AND NOT EXISTS (SELECT 1 FROM runtime_workspace_end_user_credit_bills c WHERE c.bill_id = b.id)
		ORDER BY b.request_created_at, b.id
		LIMIT ?`, user.SubjectID, *user.CreditsSince, creditSettleBatch).Scan(&bills).Error; err != nil {
		return fmt.Errorf("list unsettled end-user bills: %w", err)
	}
	if len(bills) == 0 {
		return nil
	}
	limits := appliedQuota(policy, user, now).Limits
	debt := user.CreditDebt
	for _, bill := range bills {
		var charge int64
		for _, limit := range limits {
			window, ok := QuotaWindows[limit.Window]
			if !ok || bill.TotalPoints <= 0 {
				continue
			}
			var before int64
			if err := db.Raw(`
				SELECT COALESCE(SUM(total_points), 0) FROM llm_usage_bills
				WHERE end_user_subject_id = ? AND request_created_at >= ?
					AND (request_created_at < ? OR (request_created_at = ? AND id < ?))`,
				user.SubjectID, bill.RequestCreatedAt.Add(-window), bill.RequestCreatedAt, bill.RequestCreatedAt, bill.ID).Scan(&before).Error; err != nil {
				return fmt.Errorf("sum end-user usage before a bill: %w", err)
			}
			allowed := limit.MilliPoints().Int64()
			charge = max(charge, max(0, before+bill.TotalPoints-allowed)-max(0, before-allowed))
		}
		marked := db.Exec(`INSERT INTO runtime_workspace_end_user_credit_bills (bill_id, runtime_workspace_id, subject_id, charged, created_at)
			VALUES (?, ?, ?, ?, ?) ON CONFLICT (bill_id) DO NOTHING`, bill.ID, user.RuntimeWorkspaceID, user.SubjectID, charge, now)
		if marked.Error != nil {
			return fmt.Errorf("mark settled bill: %w", marked.Error)
		}
		if marked.RowsAffected == 0 || charge == 0 {
			continue
		}
		uncovered, err := drawCreditLots(ctx, tx, user.SubjectID, charge, now)
		if err != nil {
			return err
		}
		debt += uncovered
		user.CreditDebt = debt
		_, balance, err := creditBalance(ctx, tx, user, now, false)
		if err != nil {
			return err
		}
		billID := bill.ID
		entry := CreditEntry{ID: uuid.New(), RuntimeWorkspaceID: user.RuntimeWorkspaceID, SubjectID: user.SubjectID,
			Kind: CreditEntryUsage, Amount: -charge, BalanceAfter: balance,
			BillID: &billID, CreatedAt: now}
		if err := db.Create(&entry).Error; err != nil {
			return fmt.Errorf("record credit usage: %w", err)
		}
	}
	if err := db.Model(&EndUser{}).Where("subject_id = ?", user.SubjectID).Update("credit_debt", debt).Error; err != nil {
		return fmt.Errorf("update end-user credit debt: %w", err)
	}
	return nil
}

// settledCredits settles an end user's credits under a lock and returns
// their balance.
func settledCredits(ctx context.Context, db *gorm.DB, policy EndUserQuotaPolicy, subjectID uuid.UUID, now time.Time, withLots bool) (*EndUser, CreditBalanceView, error) {
	var user EndUser
	var credits CreditBalanceView
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("subject_id = ?", subjectID).Take(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEndUserNotFound
			}
			return fmt.Errorf("load end user: %w", err)
		}
		if err := settleEndUserCredits(ctx, tx, policy, &user, now); err != nil {
			return err
		}
		var err error
		credits, _, err = creditBalance(ctx, tx, &user, now, withLots)
		return err
	})
	return &user, credits, err
}

// CreditLedger is a page of an end user's credit entries and their balance.
type CreditLedger struct {
	Credits CreditBalanceView `json:"credits"`
	Items   []CreditEntryView `json:"items"`
	Page    int               `json:"page"`
	Limit   int               `json:"limit"`
	Total   int64             `json:"total"`
	HasMore bool              `json:"has_more"`
}

// ListCredits settles an end user's credits and lists their ledger, newest
// first.
func (s *Service) ListCredits(ctx context.Context, workspaceID, subjectID uuid.UUID, page, limit int) (*CreditLedger, error) {
	runtimeWorkspace, user, err := s.loadEndUser(ctx, s.db, workspaceID, subjectID, false)
	if err != nil {
		return nil, err
	}
	page = max(page, 1)
	if limit < 1 || limit > 200 {
		limit = 50
	}
	_, credits, err := settledCredits(ctx, s.db, EndUserQuota(runtimeWorkspace.PolicyJSON), user.SubjectID, s.now(), true)
	if err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).Model(&CreditEntry{}).Where("subject_id = ? AND runtime_workspace_id = ?", user.SubjectID, runtimeWorkspace.LogicalID())
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count credit entries: %w", err)
	}
	var entries []CreditEntry
	if err := query.Order("created_at DESC").Order("id DESC").Limit(limit).Offset((page - 1) * limit).Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("list credit entries: %w", err)
	}
	items := make([]CreditEntryView, 0, len(entries))
	for _, entry := range entries {
		items = append(items, creditEntryView(entry))
	}
	return &CreditLedger{Credits: credits, Items: items, Page: page, Limit: limit, Total: total, HasMore: int64(page*limit) < total}, nil
}

// EntitlementWindow is an end user's usage in a window against the limit
// that applies, and when the oldest usage in it leaves the window.
type EntitlementWindow struct {
	Window        string     `json:"window"`
	Points        float64    `json:"points"`
	Limit         *float64   `json:"limit,omitempty"`
	Remaining     *float64   `json:"remaining,omitempty"`
	NextReleaseAt *time.Time `json:"next_release_at,omitempty"`
}

// Entitlement is what an end user may spend now: their plan, each window,
// their credits, and whether a new run would be admitted. Per-app caps are
// not included.
type Entitlement struct {
	Status      string              `json:"status"`
	QuotaSource string              `json:"quota_source"`
	Plan        *EndUserPlanView    `json:"plan,omitempty"`
	Windows     []EntitlementWindow `json:"windows"`
	Credits     *CreditBalanceView  `json:"credits,omitempty"`
	CanRun      bool                `json:"can_run"`
}

// EndUserEntitlement settles an end user's credits and reports what they may
// spend.
func (s *Service) EndUserEntitlement(ctx context.Context, workspaceID, subjectID uuid.UUID) (*Entitlement, error) {
	runtimeWorkspace, user, err := s.loadEndUser(ctx, s.db, workspaceID, subjectID, false)
	if err != nil {
		return nil, err
	}
	now := s.now()
	var credits *CreditBalanceView
	if user.CreditsSince != nil {
		var balance CreditBalanceView
		if user, balance, err = settledCredits(ctx, s.db, EndUserQuota(runtimeWorkspace.PolicyJSON), user.SubjectID, now, true); err != nil {
			return nil, err
		}
		credits = &balance
	}
	views, err := s.endUserViews(ctx, runtimeWorkspace, []EndUser{*user})
	if err != nil {
		return nil, err
	}
	view := views[0]
	entitlement := &Entitlement{Status: view.Status, QuotaSource: view.QuotaSource, Plan: view.Plan, Credits: credits}
	over := false
	for _, usage := range view.Usage {
		item := EntitlementWindow{Window: usage.Window, Points: usage.Points, Limit: usage.Limit, Remaining: usage.Remaining}
		if usage.Limit != nil && *usage.Remaining <= 0 {
			over = true
		}
		if usage.Points > 0 {
			var oldest *time.Time
			if err := s.db.WithContext(ctx).Raw(`SELECT MIN(request_created_at) FROM llm_usage_bills WHERE end_user_subject_id = ? AND request_created_at >= ?`,
				user.SubjectID, now.Add(-QuotaWindows[usage.Window])).Scan(&oldest).Error; err != nil {
				return nil, fmt.Errorf("find oldest end-user usage: %w", err)
			}
			if oldest != nil {
				release := oldest.Add(QuotaWindows[usage.Window])
				item.NextReleaseAt = &release
			}
		}
		entitlement.Windows = append(entitlement.Windows, item)
	}
	entitlement.CanRun = view.Status == EndUserActive && (!over || credits != nil && credits.Balance > 0)
	return entitlement, nil
}
