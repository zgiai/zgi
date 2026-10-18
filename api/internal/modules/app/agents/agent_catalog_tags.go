package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	shared_visibility "github.com/zgiai/zgi/api/internal/modules/shared/visibility"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxAgentCatalogTags     = 8
	maxAgentCatalogTagRunes = 32
	maxRunnableWebAppTagIDs = 8
)

func (s *agentsService) GetAgentCatalogTagFacets(ctx context.Context, accountID string, req dto.GetAgentCatalogTagFacetsRequest) (*dto.AgentCatalogTagFacetsResponse, error) {
	resp := &dto.AgentCatalogTagFacetsResponse{Items: []dto.AgentCatalogTagFacet{}}
	currentOrganization, err := s.tenantService.GetCurrentOrganization(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current organization: %w", err)
	}
	if currentOrganization == nil || currentOrganization.OrganizationID == "" {
		return nil, errCurrentOrganizationNotFound
	}

	assetKind, err := resolveAgentListAssetKind(dto.GetAgentsListRequest{AssetKind: req.AssetKind})
	if err != nil {
		return nil, err
	}
	visibleWorkspaceIDs, err := shared_visibility.ResolveVisibleWorkspaceIDs(
		ctx,
		s.enterpriseService,
		currentOrganization.OrganizationID,
		accountID,
		strings.TrimSpace(req.WorkspaceID),
		visiblePermissionCodesForAssetKind(assetKind)...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to determine user permissions: %w", err)
	}
	if len(visibleWorkspaceIDs) == 0 || s.db == nil {
		return resp, nil
	}

	type facetRow struct {
		TagID uuid.UUID `gorm:"column:tag_id"`
		Name  string    `gorm:"column:name"`
		Count int64     `gorm:"column:count"`
	}
	query := s.db.WithContext(ctx).
		Table("agent_catalog_tags AS tags").
		Select("tags.id AS tag_id, tags.name, COUNT(DISTINCT agents.id) AS count").
		Joins("JOIN agent_catalog_tag_bindings AS bindings ON bindings.tag_id = tags.id").
		Joins("JOIN agents ON agents.id = bindings.agent_id").
		Where("tags.organization_id = ?", currentOrganization.OrganizationID).
		Where("agents.tenant_id IN ?", visibleWorkspaceIDs).
		Where("agents.deleted_at IS NULL")
	if agentTypes := agentTypeFiltersForAssetKind(assetKind); len(agentTypes) > 0 {
		query = query.Where("agents.agent_type IN ?", agentTypes)
	}

	var rows []facetRow
	if err := query.
		Group("tags.id, tags.name").
		Order("tags.name ASC, tags.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load agent catalog tag facets: %w", err)
	}
	resp.Items = make([]dto.AgentCatalogTagFacet, 0, len(rows))
	for _, row := range rows {
		resp.Items = append(resp.Items, dto.AgentCatalogTagFacet{
			AgentCatalogTag: dto.AgentCatalogTag{ID: row.TagID.String(), Name: row.Name},
			Count:           row.Count,
		})
	}
	return resp, nil
}

func (s *agentsService) GetAgentCatalogTags(ctx context.Context, agentID, accountID string) (*dto.AgentCatalogTagsResponse, error) {
	ag, err := s.catalogTagAgent(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}

	tagsByAgent, err := s.agentCatalogTagsByAgentIDs(ctx, []uuid.UUID{ag.ID})
	if err != nil {
		return nil, err
	}
	return &dto.AgentCatalogTagsResponse{Items: tagsByAgent[ag.ID]}, nil
}

func (s *agentsService) ReplaceAgentCatalogTags(ctx context.Context, agentID, accountID string, req dto.ReplaceAgentCatalogTagsRequest) (*dto.AgentCatalogTagsResponse, error) {
	ag, err := s.catalogTagAgent(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	if len(req.Tags) > maxAgentCatalogTags {
		return nil, fmt.Errorf("%w: at most %d tags are allowed", runtimeservice.ErrInvalidInput, maxAgentCatalogTags)
	}
	if s.db == nil {
		return nil, fmt.Errorf("database is required")
	}

	organizationID, err := s.organizationUUIDForAgentWorkspace(ctx, ag.TenantID.String())
	if err != nil {
		return nil, err
	}
	actorID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return nil, runtimeservice.ErrUnauthorized
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tags, err := resolveAgentCatalogTags(ctx, tx, organizationID, actorID, req.Tags)
		if err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Where("agent_id = ?", ag.ID).Delete(&AgentCatalogTagBinding{}).Error; err != nil {
			return fmt.Errorf("failed to replace agent catalog tags: %w", err)
		}
		if len(tags) == 0 {
			return nil
		}
		bindings := make([]AgentCatalogTagBinding, 0, len(tags))
		for _, tag := range tags {
			bindings = append(bindings, AgentCatalogTagBinding{
				AgentID:   ag.ID,
				TagID:     tag.ID,
				CreatedBy: &actorID,
			})
		}
		if err := tx.WithContext(ctx).Create(&bindings).Error; err != nil {
			return fmt.Errorf("failed to bind agent catalog tags: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetAgentCatalogTags(ctx, agentID, accountID)
}

func (s *agentsService) catalogTagAgent(ctx context.Context, agentID, accountID string, manage bool) (*Agent, error) {
	agentID = strings.TrimSpace(agentID)
	accountID = strings.TrimSpace(accountID)
	if _, err := uuid.Parse(agentID); err != nil {
		return nil, fmt.Errorf("%w: invalid agent id", runtimeservice.ErrInvalidInput)
	}
	if _, err := uuid.Parse(accountID); err != nil {
		return nil, runtimeservice.ErrUnauthorized
	}
	ag, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil || isSystemManagedAgent(ag) {
		return nil, fmt.Errorf("%w: agent not found", runtimeservice.ErrNotFound)
	}
	if manage {
		if err := s.ensureCanManageAgent(ctx, ag, accountID, agentUpdatePermissionCodes(ag.AgentsType)...); err != nil {
			if strings.EqualFold(err.Error(), "permission denied") {
				return nil, runtimeservice.ErrPermissionDenied
			}
			return nil, err
		}
		return ag, nil
	}
	if err := s.ensureCanViewAgent(ctx, ag, accountID); err != nil {
		return nil, err
	}
	return ag, nil
}

func resolveAgentCatalogTags(ctx context.Context, tx *gorm.DB, organizationID, actorID uuid.UUID, inputs []dto.AgentCatalogTagInput) ([]AgentCatalogTag, error) {
	resolved := make([]AgentCatalogTag, 0, len(inputs))
	seen := make(map[uuid.UUID]struct{}, len(inputs))
	for _, input := range inputs {
		tag, err := resolveAgentCatalogTag(ctx, tx, organizationID, actorID, input)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[tag.ID]; ok {
			continue
		}
		seen[tag.ID] = struct{}{}
		resolved = append(resolved, tag)
	}
	return resolved, nil
}

func resolveAgentCatalogTag(ctx context.Context, tx *gorm.DB, organizationID, actorID uuid.UUID, input dto.AgentCatalogTagInput) (AgentCatalogTag, error) {
	if id := strings.TrimSpace(input.ID); id != "" {
		tagID, err := uuid.Parse(id)
		if err != nil {
			return AgentCatalogTag{}, fmt.Errorf("%w: invalid tag id", runtimeservice.ErrInvalidInput)
		}
		var tag AgentCatalogTag
		if err := tx.WithContext(ctx).Where("id = ? AND organization_id = ?", tagID, organizationID).Take(&tag).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return AgentCatalogTag{}, fmt.Errorf("%w: tag not found", runtimeservice.ErrInvalidInput)
			}
			return AgentCatalogTag{}, fmt.Errorf("failed to load agent catalog tag: %w", err)
		}
		return tag, nil
	}

	name, normalizedName, err := normalizeAgentCatalogTagName(input.Name)
	if err != nil {
		return AgentCatalogTag{}, err
	}
	tag := AgentCatalogTag{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		Name:           name,
		NormalizedName: normalizedName,
		CreatedBy:      &actorID,
	}
	result := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "organization_id"}, {Name: "normalized_name"}},
		DoNothing: true,
	}).Create(&tag)
	if result.Error != nil {
		return AgentCatalogTag{}, fmt.Errorf("failed to create agent catalog tag: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return tag, nil
	}
	// Do not reuse tag here: its generated primary key would make GORM append
	// `id = <new id>` to the conflict lookup and hide the existing row.
	var existingTag AgentCatalogTag
	if err := tx.WithContext(ctx).
		Where("organization_id = ? AND normalized_name = ?", organizationID, normalizedName).
		Take(&existingTag).Error; err != nil {
		return AgentCatalogTag{}, fmt.Errorf("failed to load agent catalog tag after conflict: %w", err)
	}
	return existingTag, nil
}

func normalizeAgentCatalogTagName(raw string) (string, string, error) {
	name := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if name == "" {
		return "", "", fmt.Errorf("%w: tag name is required", runtimeservice.ErrInvalidInput)
	}
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > maxAgentCatalogTagRunes {
		return "", "", fmt.Errorf("%w: tag name must contain at most %d characters", runtimeservice.ErrInvalidInput, maxAgentCatalogTagRunes)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", "", fmt.Errorf("%w: tag name contains control characters", runtimeservice.ErrInvalidInput)
		}
	}
	return name, strings.ToLower(name), nil
}

func (s *agentsService) agentCatalogTagsByAgentIDs(ctx context.Context, agentIDs []uuid.UUID) (map[uuid.UUID][]dto.AgentCatalogTag, error) {
	out := make(map[uuid.UUID][]dto.AgentCatalogTag, len(agentIDs))
	for _, agentID := range agentIDs {
		out[agentID] = []dto.AgentCatalogTag{}
	}
	if s.db == nil || len(agentIDs) == 0 {
		return out, nil
	}
	type row struct {
		AgentID uuid.UUID `gorm:"column:agent_id"`
		TagID   uuid.UUID `gorm:"column:tag_id"`
		Name    string    `gorm:"column:name"`
	}
	var rows []row
	if err := s.db.WithContext(ctx).
		Table("agent_catalog_tag_bindings AS bindings").
		Select("bindings.agent_id, tags.id AS tag_id, tags.name").
		Joins("JOIN agent_catalog_tags AS tags ON tags.id = bindings.tag_id").
		Where("bindings.agent_id IN ?", agentIDs).
		Order("tags.name ASC, tags.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load agent catalog tags: %w", err)
	}
	for _, row := range rows {
		out[row.AgentID] = append(out[row.AgentID], dto.AgentCatalogTag{ID: row.TagID.String(), Name: row.Name})
	}
	return out, nil
}

func parseRunnableWebAppTagIDs(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	if len(parts) > maxRunnableWebAppTagIDs {
		return nil, fmt.Errorf("%w: too many tag ids", runtimeservice.ErrInvalidInput)
	}
	seen := make(map[string]struct{}, len(parts))
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		parsed, err := uuid.Parse(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("%w: invalid tag id", runtimeservice.ErrInvalidInput)
		}
		normalized := parsed.String()
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		ids = append(ids, normalized)
	}
	return ids, nil
}
