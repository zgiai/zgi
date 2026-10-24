package digitalhuman

import (
	"context"
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
