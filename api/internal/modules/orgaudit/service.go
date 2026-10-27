package orgaudit

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// Lister reads stored records.
type Lister interface {
	List(ctx context.Context, filter Filter) ([]Row, int64, error)
}

// AdminChecker says whether an account administers an organization.
type AdminChecker interface {
	IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error)
}

// Service answers audit log queries for organization owners and admins.
type Service struct {
	lister Lister
	admins AdminChecker
}

// NewService builds the query service.
func NewService(lister Lister, admins AdminChecker) *Service {
	if lister == nil || admins == nil {
		panic("org audit service requires a lister and an admin checker")
	}
	return &Service{lister: lister, admins: admins}
}

// Page is one page of records.
type Page struct {
	Items   []Row `json:"items"`
	Total   int64 `json:"total"`
	Page    int   `json:"page"`
	Limit   int   `json:"limit"`
	HasMore bool  `json:"has_more"`
}

// List returns a page of the organization's records to an owner or admin.
func (s *Service) List(ctx context.Context, accountID string, filter Filter) (*Page, error) {
	if filter.OrganizationID == uuid.Nil {
		return nil, ErrFilterInvalid
	}
	allowed, err := s.admins.IsOrganizationAdminOrOwner(ctx, filter.OrganizationID.String(), accountID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, fmt.Errorf("%w: %w", ErrNotFound, err)
		}
		return nil, fmt.Errorf("check org audit access: %w", err)
	}
	if !allowed {
		return nil, ErrAccessDenied
	}
	if err := normalize(&filter); err != nil {
		return nil, err
	}
	rows, total, err := s.lister.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &Page{
		Items:   rows,
		Total:   total,
		Page:    filter.Page,
		Limit:   filter.Limit,
		HasMore: int64(filter.Page*filter.Limit) < total,
	}, nil
}

func normalize(filter *Filter) error {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 {
		filter.Limit = defaultLimit
	}
	if filter.Limit > maxLimit {
		filter.Limit = maxLimit
	}
	if filter.Result != "" && filter.Result != ResultSuccess && filter.Result != ResultDenied && filter.Result != ResultFailed {
		return ErrFilterInvalid
	}
	if len(filter.Action) > 64 {
		return ErrFilterInvalid
	}
	if filter.From != nil && filter.To != nil && !filter.From.Before(*filter.To) {
		return ErrFilterInvalid
	}
	return nil
}
