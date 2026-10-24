package container

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chats "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	workspace "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

// Host-specific knowledge stays at the composition root. Connector packages
// depend only on their own TargetAuthority and TargetAuthorizer ports.
type connectorTargetAuthority struct{ container *ServiceContainer }

func (c *ServiceContainer) GetConnectorTargetAuthority() *connectorTargetAuthority {
	if c.connectorTargetAuthority == nil {
		c.connectorTargetAuthority = &connectorTargetAuthority{container: c}
	}
	return c.connectorTargetAuthority
}

func (authority *connectorTargetAuthority) RequireTargetEdit(ctx context.Context, organizationID, actor uuid.UUID, target bindings.TargetIdentity) error {
	return authority.authorize(ctx, organizationID, actor, core.TargetRef{Type: target.Type, ID: target.ID, Scope: target.Scope, VersionID: target.VersionID}, true)
}

func (authority *connectorTargetAuthority) AuthorizeTarget(ctx context.Context, request execution.TargetAuthorizationRequest) error {
	b, c := request.Binding, request.Connection
	if request.ActingPrincipalID != request.OwnerAccountID || request.OwnerAccountID == uuid.Nil ||
		b.OrganizationID != request.OrganizationID || c.OrganizationID != request.OrganizationID || b.OwnerAccountID != request.OwnerAccountID || c.OwnerAccountID != request.OwnerAccountID ||
		b.ConnectionID != c.ID || b.ConnectorID != c.ConnectorID || !request.OperationID.Valid() {
		return targetDenied()
	}
	if err := authority.authorize(ctx, request.OrganizationID, request.ActingPrincipalID, b.Target, false); err != nil {
		return err
	}
	return authority.verifyHostReference(ctx, request)
}

func targetDenied() error { return core.NewError(core.ErrorCodeAccessDenied, false, nil) }

func (authority *connectorTargetAuthority) authorize(ctx context.Context, org, actor uuid.UUID, target core.TargetRef, edit bool) error {
	if authority == nil || authority.container == nil || authority.container.db == nil || ctx == nil || org == uuid.Nil || actor == uuid.Nil {
		return targetDenied()
	}
	id, err := uuid.Parse(target.ID)
	if err != nil || id == uuid.Nil || id.String() != target.ID {
		return targetDenied()
	}
	if target.Type == core.TargetTypeDigitalHuman {
		target.BindingInstanceID = "00000000-0000-0000-0000-000000000001"
	}
	if target.Type == core.TargetTypeWorkflow {
		target.BindingInstanceID = "authority-check"
	}
	if target.Validate() != nil || edit && target.EffectiveScope() == core.ScopePublished {
		return targetDenied()
	}
	c := authority.container
	organizations := c.GetOrganizationService()
	member, err := organizations.IsOrganizationMember(ctx, org.String(), actor.String())
	if err != nil {
		return targetDenied()
	}
	// External Agent API keys are virtual principals, not organization
	// members. They still need to resolve the published Agent target so the
	// runtime can discover its owner-scoped Connector bindings. The binding
	// rows remain queried under the API-key owner below, so this exception does
	// not grant access to another principal's Personal Connection.
	apiKeyPublishedRead := !member && !edit && target.Type == core.TargetTypeAgent && target.EffectiveScope() == core.ScopePublished
	if !member && !edit && target.Type == core.TargetTypeDigitalHuman && target.EffectiveScope() == core.ScopePublished {
		// A Runtime Workspace end user acts with their own Connection in a
		// per-user DigitalHuman slot (ADR 0007). They are not members, so
		// only a live run of theirs on this exact published version, in an
		// instance that is not suspended or erased, authorizes the use.
		return authority.authorizeEndUserRun(ctx, org, actor, id, target.VersionID)
	}
	if !member && !apiKeyPublishedRead {
		return targetDenied()
	}
	if target.Type == core.TargetTypeChat {
		chat, err := chats.NewConversationRepository(c.db).GetScoped(ctx, id, org, actor)
		if err != nil || chat == nil || chat.ID != id || chat.OrganizationID != org || chat.AccountID != actor || chat.DeletedAt != nil || chat.CallerType != runtimemodel.ConversationCallerAIChat || chat.ConversationType != runtimemodel.ConversationTypeChat {
			return targetDenied()
		}
		return nil
	}
	agentRepository := agents.NewAgentsRepository(c.db)
	workspaceID := ""
	permission := workspace.WorkspacePermissionAgentUpdate
	switch target.Type {
	case core.TargetTypeDigitalHuman:
		var host struct{ WorkspaceID uuid.UUID }
		query := c.db.WithContext(ctx).
			Table("digital_humans AS d").
			Select("d.workspace_id").
			Joins("JOIN digital_human_versions AS v ON v.digital_human_id = d.id AND v.organization_id = d.organization_id AND v.workspace_id = d.workspace_id").
			Where("d.id = ? AND d.organization_id = ? AND d.deleted_at IS NULL", id, org)
		if target.EffectiveScope() == core.ScopePublished {
			query = query.Where("v.id = ? AND v.state = ?", target.VersionID, "published")
		} else {
			query = query.Where("v.id = d.draft_version_id AND v.state = ?", "draft")
		}
		if query.Take(&host).Error != nil || host.WorkspaceID == uuid.Nil {
			return targetDenied()
		}
		workspaceID = host.WorkspaceID.String()
		permission = workspace.WorkspacePermissionDigitalHumanRun
		if edit {
			permission = workspace.WorkspacePermissionDigitalHumanUpdate
		}
	case core.TargetTypeAgent:
		agent, err := agentRepository.GetByID(ctx, id.String())
		if err != nil || agent == nil || agent.ID != id || agent.DeletedAt != nil || agent.TenantID == uuid.Nil {
			return targetDenied()
		}
		workspaceID = agent.TenantID.String()
		if target.EffectiveScope() == core.ScopePublished {
			var version agents.AgentPublishedVersion
			err := c.db.WithContext(ctx).Where("agent_id = ? AND version_uuid = ? AND deleted_at IS NULL", id, target.VersionID).Take(&version).Error
			if err != nil || version.VersionUUID.String() != target.VersionID {
				return targetDenied()
			}
			permission = workspace.WorkspacePermissionAgentView
		}
		if strings.Contains(strings.ToUpper(agent.AgentsType), "WORKFLOW") || agent.AgentsType == "CONVERSATIONAL_AGENT" {
			permission = workspace.WorkspacePermissionWorkflowUpdate
			if !edit {
				permission = workspace.WorkspacePermissionWorkflowRunDraft
			}
			if target.EffectiveScope() == core.ScopePublished {
				permission = workspace.WorkspacePermissionWorkflowView
			}
		}
	case core.TargetTypeWorkflow:
		repository := workflow.NewWorkflowRepository(c.db)
		var row *workflow.Workflow
		if target.EffectiveScope() == core.ScopePublished {
			if target.VersionID != target.ID {
				return targetDenied()
			}
			row, err = repository.GetByVersionUUID(ctx, target.VersionID)
		} else {
			row, err = repository.GetByID(ctx, target.ID)
		}
		if err != nil || row == nil || row.Internal {
			return targetDenied()
		}
		draft := strings.EqualFold(strings.TrimSpace(row.Version), "draft")
		if draft {
			if target.EffectiveScope() != core.ScopeDraft || row.ID != target.ID {
				return targetDenied()
			}
		} else {
			if target.EffectiveScope() != core.ScopePublished || !row.VersionUUID.Valid || row.VersionUUID.UUID != id {
				return targetDenied()
			}
		}
		workspaceID = row.TenantID
		agent, err := agentRepository.GetByID(ctx, row.AgentID)
		if err != nil || agent == nil || agent.DeletedAt != nil || agent.TenantID.String() != workspaceID {
			return targetDenied()
		}
		permission = workspace.WorkspacePermissionWorkflowUpdate
		if !edit {
			permission = workspace.WorkspacePermissionWorkflowRunDraft
			if !draft {
				permission = workspace.WorkspacePermissionWorkflowView
			}
		}
	default:
		return targetDenied()
	}
	allowed, err := organizations.CheckWorkspaceOrganizationAnyPermission(ctx, org.String(), workspaceID, actor.String(), permission)
	if err != nil || !allowed {
		if apiKeyPublishedRead && authority.isActiveAgentAPIKeyForTarget(ctx, actor, id, workspaceID) {
			return nil
		}
		return targetDenied()
	}
	return nil
}

func (authority *connectorTargetAuthority) authorizeEndUserRun(ctx context.Context, org, subject, digitalHumanID uuid.UUID, versionID string) error {
	var count int64
	err := authority.container.db.WithContext(ctx).
		Table("digital_human_runs AS r").
		Joins("JOIN user_subjects AS s ON s.id = r.user_subject_id AND s.kind = ? AND s.issuer_type = ?", "api_end_user", "runtime_workspace_api").
		Joins("LEFT JOIN digital_human_end_user_instances AS i ON i.id = r.end_user_instance_id").
		Where("r.organization_id = ? AND r.digital_human_id = ? AND r.digital_human_version_id = ? AND r.user_subject_id = ?", org, digitalHumanID, versionID, subject).
		Where("r.status IN ? AND r.cancel_requested_at IS NULL", []string{"queued", "running", "waiting"}).
		Where("r.end_user_instance_id IS NULL OR i.status = ?", "active").
		Count(&count).Error
	if err != nil || count == 0 {
		return targetDenied()
	}
	return nil
}

func (authority *connectorTargetAuthority) isActiveAgentAPIKeyForTarget(ctx context.Context, keyID, agentID uuid.UUID, workspaceID string) bool {
	workspaceUUID, err := uuid.Parse(strings.TrimSpace(workspaceID))
	if authority == nil || authority.container == nil || authority.container.db == nil || keyID == uuid.Nil || agentID == uuid.Nil || err != nil || workspaceUUID == uuid.Nil {
		return false
	}
	var count int64
	err = authority.container.db.WithContext(ctx).
		Table("agent_api_keys AS k").
		Joins("JOIN agents AS a ON a.id::text = k.agent_id AND a.tenant_id::text = k.tenant_id AND a.deleted_at IS NULL").
		Where("k.id = ? AND k.agent_id = ? AND k.tenant_id = ? AND k.status = ? AND (k.expires_at IS NULL OR k.expires_at > ?)", keyID.String(), agentID.String(), workspaceUUID.String(), "active", time.Now()).
		Count(&count).Error
	return err == nil && count == 1
}

func (authority *connectorTargetAuthority) RequireTargetRead(ctx context.Context, org, actor uuid.UUID, target bindings.TargetIdentity) error {
	return authority.authorize(ctx, org, actor, core.TargetRef{Type: target.Type, ID: target.ID, Scope: target.Scope, VersionID: target.VersionID}, false)
}

func (c *ServiceContainer) GetConnectorReferenceService() *bindings.ReferenceService {
	return bindings.NewReferenceService(c.db, c.GetConnectorTargetAuthority())
}
