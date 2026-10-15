package zgimarket

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

// Where a member's request to activate a market item stands.
const (
	ActivationPending   = "pending"
	ActivationApproved  = "approved"
	ActivationRejected  = "rejected"
	ActivationCancelled = "cancelled"
)

const (
	maxActivationReason   = 500
	maxActivationTitle    = 200
	maxActivationRequests = 100
)

// ActivationRequest is a member's request that an administrator activate one market item for the
// organization. Only administrators activate; a member asks, and an administrator approves, which
// activates the item, or rejects with a reason.
type ActivationRequest struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	RequesterID    uuid.UUID `gorm:"type:uuid;not null" json:"requester_id"`
	// RequesterName is the requester's name when the requests are read; empty when it is gone.
	RequesterName string `gorm:"-" json:"requester_name"`
	ListingID     uint64 `gorm:"not null" json:"listing_id"`
	ListingKind   string `gorm:"type:varchar(16);not null" json:"listing_kind"`
	ListingSlug   string `gorm:"type:varchar(128);not null" json:"listing_slug"`
	ListingTitle  string `gorm:"type:varchar(200);not null" json:"listing_title"`
	// OfferingID is the API offering asked for; 0 asks for the listing as a whole.
	OfferingID    uint64     `gorm:"not null" json:"offering_id"`
	OfferingTitle string     `gorm:"type:varchar(200);not null" json:"offering_title"`
	Reason        string     `gorm:"type:varchar(500);not null" json:"reason"`
	Status        string     `gorm:"type:varchar(16);not null" json:"status"`
	ReviewerID    *uuid.UUID `gorm:"type:uuid" json:"reviewer_id"`
	ReviewerName  string     `gorm:"-" json:"reviewer_name"`
	ReviewNote    string     `gorm:"type:varchar(500);not null" json:"review_note"`
	ReviewedAt    *time.Time `json:"reviewed_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TableName names the table of market activation requests.
func (ActivationRequest) TableName() string { return "zgi_market_activation_requests" }

// ActivationRequestInput is what a member sends to ask for an item.
type ActivationRequestInput struct {
	ListingID     uint64 `json:"listing_id"`
	ListingKind   string `json:"listing_kind"`
	ListingSlug   string `json:"listing_slug"`
	ListingTitle  string `json:"listing_title"`
	OfferingID    uint64 `json:"offering_id"`
	OfferingTitle string `json:"offering_title"`
	Reason        string `json:"reason"`
}

var (
	errActivationInput    = errors.New("invalid activation request")
	errActivationNotFound = errors.New("activation request not found")
	errActivationSettled  = errors.New("activation request is no longer pending")
)

var activationKinds = map[string]bool{
	"api": true, "mcp": true, "skill": true, "plugin": true, "connector": true,
}

// clean checks the input and trims it to what the table holds.
func (in ActivationRequestInput) clean() (ActivationRequestInput, error) {
	in.ListingKind = strings.TrimSpace(in.ListingKind)
	in.ListingSlug = strings.TrimSpace(in.ListingSlug)
	if in.ListingID == 0 || !activationKinds[in.ListingKind] || !marketSlugPattern.MatchString(in.ListingSlug) {
		return in, errActivationInput
	}
	if in.OfferingID != 0 && in.ListingKind != "api" {
		return in, errActivationInput
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(in.Reason) > maxActivationReason {
		return in, errActivationInput
	}
	in.ListingTitle = cut(strings.TrimSpace(in.ListingTitle), maxActivationTitle)
	in.OfferingTitle = cut(strings.TrimSpace(in.OfferingTitle), maxActivationTitle)
	return in, nil
}

type activationStore struct {
	db  *gorm.DB
	now func() time.Time
}

// create files a request, or returns the requester's pending one for the same item.
func (s activationStore) create(ctx context.Context, organizationID, requesterID uuid.UUID, in ActivationRequestInput) (*ActivationRequest, error) {
	in, err := in.clean()
	if err != nil {
		return nil, err
	}
	var pending ActivationRequest
	err = s.db.WithContext(ctx).Where(
		"organization_id = ? AND requester_id = ? AND listing_id = ? AND offering_id = ? AND status = ?",
		organizationID, requesterID, in.ListingID, in.OfferingID, ActivationPending,
	).First(&pending).Error
	if err == nil {
		return &pending, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	now := s.now().UTC()
	row := &ActivationRequest{
		ID: uuid.New(), OrganizationID: organizationID, RequesterID: requesterID,
		ListingID: in.ListingID, ListingKind: in.ListingKind, ListingSlug: in.ListingSlug, ListingTitle: in.ListingTitle,
		OfferingID: in.OfferingID, OfferingTitle: in.OfferingTitle, Reason: in.Reason,
		Status: ActivationPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// ActivationRequestQuery narrows a read of the requests.
type ActivationRequestQuery struct {
	Status    string
	ListingID uint64
	// RequesterID limits the read to one member's requests; uuid.Nil reads everyone's.
	RequesterID uuid.UUID
}

// ActivationRequestPage is one read of the requests, newest first.
type ActivationRequestPage struct {
	Data    []*ActivationRequest `json:"data"`
	Pending int64                `json:"pending"`
}

func (s activationStore) list(ctx context.Context, organizationID uuid.UUID, query ActivationRequestQuery) (*ActivationRequestPage, error) {
	scope := func() *gorm.DB {
		scoped := s.db.WithContext(ctx).Model(&ActivationRequest{}).Where("organization_id = ?", organizationID)
		if query.RequesterID != uuid.Nil {
			scoped = scoped.Where("requester_id = ?", query.RequesterID)
		}
		return scoped
	}
	page := &ActivationRequestPage{Data: []*ActivationRequest{}}
	if err := scope().Where("status = ?", ActivationPending).Count(&page.Pending).Error; err != nil {
		return nil, err
	}
	rows := scope()
	if query.Status != "" {
		rows = rows.Where("status = ?", query.Status)
	}
	if query.ListingID != 0 {
		rows = rows.Where("listing_id = ?", query.ListingID)
	}
	if err := rows.Order("created_at DESC").Limit(maxActivationRequests).Find(&page.Data).Error; err != nil {
		return nil, err
	}
	s.nameAccounts(ctx, page.Data)
	return page, nil
}

// settle moves a pending request to status. Only an administrator approves or rejects, which the
// caller checks; requesterID, when given, limits it to that member's own request.
func (s activationStore) settle(
	ctx context.Context, organizationID, id uuid.UUID, status string, reviewerID, requesterID *uuid.UUID, note string,
) (*ActivationRequest, error) {
	var row ActivationRequest
	found := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, id)
	if requesterID != nil {
		found = found.Where("requester_id = ?", *requesterID)
	}
	if err := found.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errActivationNotFound
		}
		return nil, err
	}
	if row.Status != ActivationPending {
		return nil, errActivationSettled
	}
	now := s.now().UTC()
	updates := map[string]any{"status": status, "review_note": cut(strings.TrimSpace(note), maxActivationReason), "updated_at": now}
	if reviewerID != nil {
		updates["reviewer_id"] = *reviewerID
		updates["reviewed_at"] = now
	}
	// The status is checked again in the update, so two reviewers cannot both settle it.
	result := s.db.WithContext(ctx).Model(&ActivationRequest{}).
		Where("id = ? AND status = ?", id, ActivationPending).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, errActivationSettled
	}
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	s.nameAccounts(ctx, []*ActivationRequest{&row})
	return &row, nil
}

// settleActivated approves the pending requests the activation of an item answers: those for the
// listing that name the activated offering or none. The member who activated it is their reviewer.
func (s activationStore) settleActivated(ctx context.Context, organizationID uuid.UUID, listingID, offeringID uint64, actorID uuid.UUID) error {
	now := s.now().UTC()
	return s.db.WithContext(ctx).Model(&ActivationRequest{}).
		Where("organization_id = ? AND listing_id = ? AND status = ? AND offering_id IN ?",
			organizationID, listingID, ActivationPending, []uint64{0, offeringID}).
		Updates(map[string]any{
			"status": ActivationApproved, "reviewer_id": actorID, "reviewed_at": now, "updated_at": now,
		}).Error
}

// closeActivated settles the requests a member's own activation of a free item answered. It is
// best effort: the item is activated whatever becomes of them, and an administrator may still
// answer them.
func (p *Proxy) closeActivated(ctx context.Context, who caller, listingID, offeringID uint64) {
	if p.tools == nil || listingID == 0 {
		return
	}
	organizationID, errOrganization := uuid.Parse(who.organizationID)
	accountID, errAccount := uuid.Parse(who.accountID)
	if errOrganization != nil || errAccount != nil {
		return
	}
	if err := p.tools.requests.settleActivated(ctx, organizationID, listingID, offeringID, accountID); err != nil {
		logger.WarnContext(ctx, "the activation requests a free activation answered could not be settled",
			"listing_id", listingID, "error", err.Error())
	}
}

// nameAccounts fills in the requester's and the reviewer's names. A name that cannot be read is
// left empty.
func (s activationStore) nameAccounts(ctx context.Context, rows []*ActivationRequest) {
	ids := []uuid.UUID{}
	for _, row := range rows {
		ids = append(ids, row.RequesterID)
		if row.ReviewerID != nil {
			ids = append(ids, *row.ReviewerID)
		}
	}
	if len(ids) == 0 {
		return
	}
	var found []struct {
		ID   string
		Name string
	}
	if err := s.db.WithContext(ctx).Table("accounts").Select("id, name").Where("id IN ?", ids).Scan(&found).Error; err != nil {
		logger.WarnContext(ctx, "the names of market activation requesters could not be read", "error", err.Error())
		return
	}
	names := map[string]string{}
	for _, account := range found {
		names[account.ID] = account.Name
	}
	for _, row := range rows {
		row.RequesterName = names[row.RequesterID.String()]
		if row.ReviewerID != nil {
			row.ReviewerName = names[row.ReviewerID.String()]
		}
	}
}
