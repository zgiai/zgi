package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/artifacts"
	"gorm.io/gorm"
)

// ArtifactOwnerNames names artifact owners for the console: a member by
// their name, an end user by the identifier their application gave them.
// A workspace's artifacts are the team's and need no name.
type ArtifactOwnerNames struct{ db *gorm.DB }

// NewArtifactOwnerNames builds the console's owner names.
func NewArtifactOwnerNames(db *gorm.DB) *ArtifactOwnerNames {
	if db == nil {
		return nil
	}
	return &ArtifactOwnerNames{db: db}
}

// Fill names the owners of items it can; the rest keep no name.
func (n *ArtifactOwnerNames) Fill(ctx context.Context, items ...*artifacts.Summary) {
	if n == nil || n.db == nil {
		return
	}
	accounts, endUsers := []uuid.UUID{}, []uuid.UUID{}
	for _, item := range items {
		if item == nil {
			continue
		}
		switch item.OwnerKind {
		case artifacts.OwnerAccount:
			accounts = append(accounts, item.OwnerID)
		case artifacts.OwnerAPIEndUser:
			endUsers = append(endUsers, item.OwnerID)
		}
	}
	names := map[uuid.UUID]string{}
	if len(accounts) > 0 {
		var rows []struct {
			ID    uuid.UUID
			Name  string
			Email string
		}
		if n.db.WithContext(ctx).Table("accounts").Select("id, name, email").Where("id IN ?", accounts).Scan(&rows).Error == nil {
			for _, row := range rows {
				names[row.ID] = firstNonEmptyString(row.Name, row.Email)
			}
		}
	}
	if len(endUsers) > 0 {
		var rows []struct {
			SubjectID      uuid.UUID
			ExternalUserID string
		}
		if n.db.WithContext(ctx).Table("runtime_workspace_end_users").Select("subject_id, external_user_id").
			Where("subject_id IN ?", endUsers).Scan(&rows).Error == nil {
			for _, row := range rows {
				names[row.SubjectID] = strings.TrimSpace(row.ExternalUserID)
			}
		}
	}
	for _, item := range items {
		if item != nil {
			item.OwnerName = names[item.OwnerID]
		}
	}
}

// ArtifactOwnerPage is one page of artifact owners, those with the most
// artifacts first.
type ArtifactOwnerPage struct {
	Items    []artifacts.OwnerCount `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	HasMore  bool                   `json:"has_more"`
}

// Page names the owners counted by owners (from artifacts.Service.Owners),
// keeps those whose name, email, end user identifier or ID matches search,
// and returns one page of them.
func (n *ArtifactOwnerNames) Page(ctx context.Context, owners *gorm.DB, search string, page, size int) (*ArtifactOwnerPage, error) {
	if n == nil || n.db == nil {
		return nil, errors.New("artifact owner names are not configured")
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}
	named := func() *gorm.DB {
		db := n.db.WithContext(ctx).Table("(?) AS o", owners).
			Joins("LEFT JOIN accounts a ON o.kind = ? AND a.id = o.id", artifacts.OwnerAccount).
			Joins("LEFT JOIN runtime_workspace_end_users e ON o.kind = ? AND e.subject_id = o.id", artifacts.OwnerAPIEndUser)
		if keyword := strings.ToLower(strings.TrimSpace(search)); keyword != "" {
			like := "%" + keyword + "%"
			db = db.Where("LOWER(COALESCE(a.name, '')) LIKE ? OR LOWER(COALESCE(a.email, '')) LIKE ? OR "+
				"LOWER(COALESCE(e.external_user_id, '')) LIKE ? OR LOWER(CAST(o.id AS TEXT)) LIKE ?", like, like, like, like)
		}
		return db
	}
	var total int64
	if err := named().Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count artifact owners: %w", err)
	}
	var rows []struct {
		Kind           artifacts.OwnerKind
		ID             uuid.UUID
		ArtifactCount  int64
		Name           string
		Email          string
		ExternalUserID string
	}
	if err := named().
		Select("o.kind, o.id, o.artifact_count, COALESCE(a.name, '') AS name, COALESCE(a.email, '') AS email, " +
			"COALESCE(e.external_user_id, '') AS external_user_id").
		Order("o.artifact_count DESC, o.last_updated_at DESC").Offset((page - 1) * size).Limit(size).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list artifact owners: %w", err)
	}
	items := make([]artifacts.OwnerCount, 0, len(rows))
	for _, row := range rows {
		item := artifacts.OwnerCount{Kind: row.Kind, ID: row.ID, Count: row.ArtifactCount, Email: row.Email}
		switch row.Kind {
		case artifacts.OwnerAccount:
			item.Name = firstNonEmptyString(row.Name, row.Email)
		case artifacts.OwnerAPIEndUser:
			item.Name = strings.TrimSpace(row.ExternalUserID)
		}
		items = append(items, item)
	}
	return &ArtifactOwnerPage{Items: items, Total: total, Page: page, PageSize: size, HasMore: int64(page*size) < total}, nil
}
