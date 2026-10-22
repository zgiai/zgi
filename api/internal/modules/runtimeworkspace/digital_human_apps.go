package runtimeworkspace

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

// DigitalHumans join Runtime Workspaces next to Agents (ADR 0010). A bound
// DigitalHuman is callable when it is active, has a published version, and
// its API surface is open; its own API key setting is that surface.

// digitalHumanCapabilities are the operations /api/connect/v1 serves for a
// bound DigitalHuman.
var digitalHumanCapabilities = []string{"chat", "conversations", "files", "tasks", "reports"}

// bindingAppType reads a binding's application type; rows written before
// DigitalHumans could be bound are Agents.
func bindingAppType(binding AgentBinding) string {
	if binding.AppType == AppTypeDigitalHuman {
		return AppTypeDigitalHuman
	}
	return AppTypeAgent
}

// loadBoundDigitalHumans loads the DigitalHumans of bindings and reports
// which of them the Runtime Workspace may call.
func (s *Service) loadBoundDigitalHumans(ctx context.Context, runtimeWorkspace *RuntimeWorkspace, ids []uuid.UUID) (map[uuid.UUID]runtimeAgentRecord, map[uuid.UUID]struct{}, error) {
	records := make(map[uuid.UUID]runtimeAgentRecord, len(ids))
	authorized := map[uuid.UUID]struct{}{}
	if runtimeWorkspace == nil || len(ids) == 0 {
		return records, authorized, nil
	}
	var rows []runtimeAgentRecord
	if err := s.db.WithContext(ctx).Table("digital_humans AS digital_humans").
		Select("digital_humans.id, digital_humans.workspace_id AS tenant_id, workspaces.organization_id, workspaces.name AS workspace_name, workspaces.status AS workspace_status, "+
			"digital_humans.name, digital_humans.description, COALESCE(digital_humans.icon, '') AS icon, COALESCE(digital_humans.icon_type, '') AS icon_type, "+
			"(digital_humans.status = 'active' AND versions.id IS NOT NULL) AS published, COALESCE(CAST(versions.version_number AS TEXT), '') AS version, "+
			"versions.id AS version_uuid, COALESCE(versions.published_at, versions.created_at) AS published_at").
		Joins("JOIN workspaces AS workspaces ON workspaces.id = digital_humans.workspace_id").
		Joins("LEFT JOIN digital_human_versions AS versions ON versions.id = digital_humans.published_version_id AND versions.digital_human_id = digital_humans.id AND versions.state = 'published'").
		Where("digital_humans.id IN ? AND digital_humans.deleted_at IS NULL", ids).Find(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("load runtime workspace digital humans: %w", err)
	}
	candidates := make([]runtimeauth.ResourceAuthorizationCandidate, 0, len(rows))
	for _, row := range rows {
		// The API surface decides whether a DigitalHuman is callable; the
		// Agent-only fields are set so shared checks pass.
		row.AppType, row.EnableAPI, row.WebAppStatus = AppTypeDigitalHuman, true, runtimeauth.WebAppStatusActive
		records[row.ID] = row
		if row.OrganizationID != runtimeWorkspace.OrganizationID || row.WorkspaceStatus != workspace_model.WorkspaceStatusNormal || !row.Published {
			continue
		}
		candidates = append(candidates, runtimeauth.ResourceAuthorizationCandidate{ResourceID: row.ID,
			Fallback: runtimeauth.PublishedRuntimePolicy{WebAppStatus: runtimeauth.WebAppStatusInactive, APIEnabled: false, InternalInvocation: true}})
	}
	audience := runtimeauth.RuntimeAudience{OrganizationID: runtimeWorkspace.OrganizationID, WorkspaceIDs: []uuid.UUID{runtimeWorkspace.ResourceWorkspaceID()}}
	authorizedIDs, err := runtimeauth.NewStore(s.db).FilterAuthorizedResourceIDs(ctx, runtimeauth.PublishedRuntimeResourceDigitalHuman, runtimeauth.PublishedRuntimeSurfaceAPI, runtimeWorkspace.OrganizationID, candidates, audience)
	if err != nil {
		return nil, nil, fmt.Errorf("authorize runtime workspace digital humans: %w", err)
	}
	for _, id := range authorizedIDs {
		authorized[id] = struct{}{}
	}
	return records, authorized, nil
}

// digitalHumanCandidateIDs lists the DigitalHumans of workspaces, by name.
func (s *Service) digitalHumanCandidateIDs(ctx context.Context, workspaceIDs []uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	if len(workspaceIDs) == 0 {
		return ids, nil
	}
	if err := s.db.WithContext(ctx).Table("digital_humans").
		Where("workspace_id IN ? AND deleted_at IS NULL", workspaceIDs).
		Order("name ASC").Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list runtime workspace digital human candidate ids: %w", err)
	}
	return ids, nil
}

// CallerActive reports whether a Runtime Workspace credential may call a
// bound DigitalHuman now: the credential and its Runtime Workspace are
// active and the DigitalHuman is bound and callable.
func (s *Service) CallerActive(ctx context.Context, runtimeWorkspaceID, credentialID, digitalHumanID uuid.UUID) error {
	if s == nil || runtimeWorkspaceID == uuid.Nil || credentialID == uuid.Nil || digitalHumanID == uuid.Nil {
		return ErrUnauthorized
	}
	var credential Credential
	if err := s.db.WithContext(ctx).Where("id = ? AND status = ?", credentialID, CredentialActive).
		Where("expires_at IS NULL OR expires_at > ?", s.now()).First(&credential).Error; err != nil {
		return ErrUnauthorized
	}
	owner, err := s.loadRuntimeWorkspace(ctx, credential.RuntimeWorkspaceID)
	if err != nil || owner.LogicalID() != runtimeWorkspaceID {
		return ErrUnauthorized
	}
	return s.digitalHumanCallable(ctx, runtimeWorkspaceID, digitalHumanID)
}

// SubjectBound reports whether an end-user subject was issued by a Runtime
// Workspace that still binds the DigitalHuman as a callable app.
func (s *Service) SubjectBound(ctx context.Context, subjectID, digitalHumanID uuid.UUID) error {
	if s == nil || subjectID == uuid.Nil || digitalHumanID == uuid.Nil {
		return ErrUnauthorized
	}
	var issuers []uuid.UUID
	if err := s.db.WithContext(ctx).Table("user_subjects").
		Where("id = ? AND kind = ? AND issuer_type = ? AND issuer_id IS NOT NULL", subjectID, "api_end_user", SubjectIssuerType).
		Pluck("issuer_id", &issuers).Error; err != nil {
		return fmt.Errorf("load runtime workspace end-user subject: %w", err)
	}
	if len(issuers) != 1 {
		return ErrUnauthorized
	}
	return s.digitalHumanCallable(ctx, issuers[0], digitalHumanID)
}

// digitalHumanCallable reports whether an active Runtime Workspace binds the
// DigitalHuman and may call it.
func (s *Service) digitalHumanCallable(ctx context.Context, runtimeWorkspaceID, digitalHumanID uuid.UUID) error {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, runtimeWorkspaceID)
	if err != nil || runtimeWorkspace.Status != StatusActive || runtimeWorkspace.Mode != ModeAttached && runtimeWorkspace.Mode != ModeIsolated {
		return ErrWorkspaceDisabled
	}
	var binding AgentBinding
	if err := s.db.WithContext(ctx).Where("runtime_workspace_id = ? AND agent_id = ? AND app_type = ? AND status = ?",
		runtimeWorkspace.LogicalID(), digitalHumanID, AppTypeDigitalHuman, AgentBindingActive).First(&binding).Error; err != nil {
		return ErrAppNotFound
	}
	_, authorized, err := s.loadBoundDigitalHumans(ctx, runtimeWorkspace, []uuid.UUID{digitalHumanID})
	if err != nil {
		return err
	}
	if _, ok := authorized[digitalHumanID]; !ok {
		return ErrAppDisabled
	}
	return nil
}
