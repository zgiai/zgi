package runtimeworkspace

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func (s *Service) catalogTagsByAgentIDs(ctx context.Context, agentIDs []uuid.UUID) (map[uuid.UUID][]CatalogTag, error) {
	tagsByAgent := make(map[uuid.UUID][]CatalogTag, len(agentIDs))
	for _, agentID := range agentIDs {
		tagsByAgent[agentID] = []CatalogTag{}
	}
	if s.db == nil || len(agentIDs) == 0 {
		return tagsByAgent, nil
	}

	type catalogTagRow struct {
		AgentID uuid.UUID `gorm:"column:agent_id"`
		TagID   uuid.UUID `gorm:"column:tag_id"`
		Name    string    `gorm:"column:name"`
	}
	var rows []catalogTagRow
	if err := s.db.WithContext(ctx).
		Table("agent_catalog_tag_bindings AS bindings").
		Select("bindings.agent_id, tags.id AS tag_id, tags.name").
		Joins("JOIN agent_catalog_tags AS tags ON tags.id = bindings.tag_id").
		Where("bindings.agent_id IN ?", agentIDs).
		Order("tags.name ASC, tags.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load Runtime Workspace agent catalog tags: %w", err)
	}

	for _, row := range rows {
		tagsByAgent[row.AgentID] = append(tagsByAgent[row.AgentID], CatalogTag{
			ID:   row.TagID.String(),
			Name: row.Name,
		})
	}
	return tagsByAgent, nil
}
