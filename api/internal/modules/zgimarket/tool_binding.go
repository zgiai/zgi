package zgimarket

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The subjects a market tool may be bound to.
const (
	SubjectAgent        = "agent"
	SubjectDigitalHuman = "digital_human"
)

// The risk levels of a market MCP tool, as the market states them.
const (
	RiskRead        = "read"
	RiskWrite       = "write"
	RiskDestructive = "destructive"
)

// maxBoundListings bounds the market listings one subject may use; the market allows 64.
const maxBoundListings = 32

var (
	// ErrSubjectNotFound reports a subject outside the caller's organization.
	ErrSubjectNotFound = errors.New("the agent or digital human was not found")
	// ErrListingNotBindable reports a listing the organization has not activated, or that is
	// neither an MCP server nor an http API offering with declared operations.
	ErrListingNotBindable = errors.New("the listing is not an activated MCP server or API offering")
	// ErrMarketToolsUnavailable reports that market tools are not configured on this instance.
	ErrMarketToolsUnavailable = errors.New("ZGI Market tools are not configured")
)

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ToolInfo is one tool of a bound listing as the market described it when it was bound.
type ToolInfo struct {
	Name      string `json:"name"`
	RiskLevel string `json:"risk_level"`
}

// ToolBindingItem is one market listing granted to a subject: an MCP server, or one offering of an
// API listing with the operations that are its tools.
type ToolBindingItem struct {
	// Kind is ItemKindMCP or ItemKindAPI. Empty is an MCP server, as before API offerings.
	Kind           string     `json:"kind,omitempty"`
	ListingID      uint64     `json:"listing_id"`
	Slug           string     `json:"slug"`
	Title          string     `json:"title,omitempty"`
	SubscriptionID uint64     `json:"subscription_id"`
	ConnectionID   *uint64    `json:"connection_id,omitempty"`
	Tools          []ToolInfo `json:"tools"`
	// OfferingSlug names the offering of an API item.
	OfferingSlug string `json:"offering_slug,omitempty"`
	// AllOperations says the item follows every bindable operation of the offering, also the
	// ones a later version adds. Otherwise only the listed operations are tools.
	AllOperations bool             `json:"all_operations,omitempty"`
	Operations    []BoundOperation `json:"operations,omitempty"`
}

// IsAPI reports whether the item is an offering of an API listing.
func (i ToolBindingItem) IsAPI() bool { return i.Kind == ItemKindAPI }

// ToolBinding is the market binding of one agent or digital human: the listings it may use and
// the credential the market issued for exactly those listings, sealed.
type ToolBinding struct {
	ID               uuid.UUID         `gorm:"type:uuid;primaryKey"`
	OrganizationID   uuid.UUID         `gorm:"type:uuid;not null"`
	SubjectType      string            `gorm:"type:varchar(32);not null"`
	SubjectID        uuid.UUID         `gorm:"type:uuid;not null"`
	MarketBindingID  uint64            `gorm:"not null"`
	MarketVersion    int               `gorm:"not null"`
	SecretCiphertext string            `gorm:"type:text;not null" json:"-"`
	Items            []ToolBindingItem `gorm:"type:jsonb;serializer:json;not null"`
	CreatedBy        uuid.UUID         `gorm:"type:uuid;not null"`
	UpdatedBy        uuid.UUID         `gorm:"type:uuid;not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// TableName names the table of market tool bindings.
func (ToolBinding) TableName() string { return "zgi_market_bindings" }

// item returns the bound MCP listing with the slug.
func (b *ToolBinding) item(slug string) (ToolBindingItem, bool) {
	for _, item := range b.Items {
		if !item.IsAPI() && item.Slug == slug {
			return item, true
		}
	}
	return ToolBindingItem{}, false
}

// offering returns the bound API offering with the slug.
func (b *ToolBinding) offering(slug string) (ToolBindingItem, bool) {
	for _, item := range b.Items {
		if item.IsAPI() && item.OfferingSlug == slug {
			return item, true
		}
	}
	return ToolBindingItem{}, false
}

// toolBindingStore keeps market tool bindings.
type toolBindingStore struct {
	db *gorm.DB
}

func (s toolBindingStore) find(ctx context.Context, organizationID uuid.UUID, subjectType string, subjectID uuid.UUID) (*ToolBinding, error) {
	var rows []ToolBinding
	err := s.db.WithContext(ctx).
		Where("organization_id = ? AND subject_type = ? AND subject_id = ?", organizationID, subjectType, subjectID).
		Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

func (s toolBindingStore) save(ctx context.Context, binding *ToolBinding) error {
	return s.db.WithContext(ctx).Save(binding).Error
}

func (s toolBindingStore) delete(ctx context.Context, id uuid.UUID) error {
	return s.db.WithContext(ctx).Delete(&ToolBinding{}, "id = ?", id).Error
}

// subjectExists reports whether the agent or digital human belongs to the organization.
func (s toolBindingStore) subjectExists(ctx context.Context, organizationID uuid.UUID, subjectType string, subjectID uuid.UUID) (bool, error) {
	var count int64
	var err error
	switch subjectType {
	case SubjectAgent:
		err = s.db.WithContext(ctx).Table("agents").
			Joins("JOIN workspaces ON workspaces.id = agents.tenant_id").
			Where("agents.id = ? AND workspaces.organization_id = ? AND agents.deleted_at IS NULL", subjectID, organizationID).
			Count(&count).Error
	case SubjectDigitalHuman:
		err = s.db.WithContext(ctx).Table("digital_humans").
			Where("id = ? AND organization_id = ? AND deleted_at IS NULL", subjectID, organizationID).
			Count(&count).Error
	default:
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check the bound subject: %w", err)
	}
	return count > 0, nil
}

// consumerRef is how the market names the subject of a binding.
func consumerRef(subjectType string, subjectID uuid.UUID) string {
	if subjectType == SubjectAgent {
		return "agent:" + subjectID.String()
	}
	return "dh:" + subjectID.String()
}
