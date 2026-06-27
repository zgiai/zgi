package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func (s *agentsService) CreateAgent(ctx context.Context, tenantID string, req interface{}, accountID string) (interface{}, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(accountID) == "" || req == nil {
		return nil, fmt.Errorf("invalid parameters")
	}

	var (
		name        string
		iconType    string
		icon        string
		agentType   string
		description string
		internal    bool
	)

	switch v := req.(type) {
	case *dto.CreateAgentRequest:
		name = v.Name
		iconType = v.IconType
		icon = v.Icon
		agentType = v.AgentType
		if agentType == "" && v.AgentType != "" {
			agentType = v.AgentType
		}
		description = v.Description
		if v.Internal != nil {
			internal = *v.Internal
		}
	case dto.CreateAgentRequest:
		name = v.Name
		iconType = v.IconType
		icon = v.Icon
		agentType = v.AgentType
		if agentType == "" && v.AgentType != "" {
			agentType = v.AgentType
		}
		description = v.Description
		if v.Internal != nil {
			internal = *v.Internal
		}
	case map[string]interface{}:
		if s2, ok2 := v["name"].(string); ok2 {
			name = s2
		}
		if s2, ok2 := v["icon_type"].(string); ok2 {
			iconType = s2
		}
		if s2, ok2 := v["icon"].(string); ok2 {
			icon = s2
		}
		if s2, ok2 := v["agentType"].(string); ok2 {
			agentType = s2
		}
		if s2, ok2 := v["agent_type"].(string); ok2 && agentType == "" {
			agentType = s2
		}
		if s2, ok2 := v["description"].(string); ok2 {
			description = s2
		}
		if b, ok := v["internal"].(bool); ok {
			internal = b
		}
	default:
		return nil, fmt.Errorf("unsupported request type")
	}

	if strings.TrimSpace(name) == "" || strings.TrimSpace(agentType) == "" {
		return nil, fmt.Errorf("name and agentType are required")
	}

	if err := s.ensureWorkspacePermission(ctx, tenantID, accountID, model.WorkspacePermissionAgentCreate); err != nil {
		return nil, err
	}

	// Duplicate name in tenant
	exists, err := s.agentsRepo.ExistsByName(ctx, tenantID, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("agent with the same name already exists")
	}

	// Step 1: Get organization ID from workspace for quota checking and default model resolution.
	var groupID *uuid.UUID
	organizationID := strings.TrimSpace(tenantID)
	if s.enterpriseService != nil {
		group, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, tenantID)
		if err == nil && group != nil {
			if strings.TrimSpace(group.ID) != "" {
				organizationID = strings.TrimSpace(group.ID)
			}
			// Parse organization ID string to UUID.
			parsedGroupID, parseErr := uuid.Parse(group.ID)
			if parseErr == nil {
				groupID = &parsedGroupID
			}
		}
	}

	// Step 2: Check AI agents quota if groupID exists
	if groupID != nil && s.quotaService != nil {
		canProceed, currentUsage, limit, err := s.quotaService.CheckQuota(ctx, *groupID, "ai_agents", 1)
		if err != nil {
			return nil, fmt.Errorf("failed to check AI agents quota: %w", err)
		}

		// Step 3: If quota exceeded, return error
		if !canProceed {
			return nil, fmt.Errorf("AI智能体配额不足。当前: %d, 限制: %d", currentUsage, limit)
		}
	}

	// Build Agent model
	ag := &Agent{
		TenantID:    parseUUID(tenantID),
		Name:        name,
		Description: description,
		AgentsType:  normalizeMode(agentType),
		EnableAPI:   true,
		Internal:    internal,
	}
	if iconType != "" {
		ag.IconType = &iconType
	}
	if icon != "" {
		ag.Icon = &icon
	}

	// Generate web_app_id
	ag.WebAppID = uuid.New()

	// Set created_by
	if uid, err := uuid.Parse(accountID); err == nil {
		ag.CreatedBy = &uid
	}

	// Step 4: Create agent and record usage in transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Create agent
		if err := tx.WithContext(ctx).Create(ag).Error; err != nil {
			return fmt.Errorf("failed to create agent: %w", err)
		}

		// Create installed record
		inst := &InstalledAgent{TenantID: ag.TenantID, AgentID: ag.ID, AgentOwnerTenantID: ag.TenantID, Position: 0, IsPinned: false}
		if err := tx.WithContext(ctx).Create(inst).Error; err != nil {
			return fmt.Errorf("failed to create installed agent: %w", err)
		}

		// Step 5: Record usage history if groupID exists
		if groupID != nil && s.quotaService != nil {
			// Parse accountID to UUID
			accountUUID, err := uuid.Parse(accountID)
			if err != nil {
				return fmt.Errorf("failed to parse account ID: %w", err)
			}

			// Parse workspaceID to UUID.
			workspaceUUID, err := uuid.Parse(tenantID)
			if err != nil {
				return fmt.Errorf("failed to parse tenant ID: %w", err)
			}

			// Create usage history record
			agentIDStr := ag.ID.String()
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      *groupID,
				AccountID:    accountUUID,
				TenantID:     &workspaceUUID,
				ResourceType: quota_model.ResourceTypeAIAgents,
				Delta:        1, // +1 for creating an agent
				ResourceID:   &agentIDStr,
				ResourceName: &ag.Name,
				Metadata: &quota_model.JSONMap{
					"agent_id":   ag.ID.String(),
					"agent_name": ag.Name,
					"agent_type": ag.AgentsType,
				},
			}

			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record AI agent usage: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if ag.AgentsType == "AGENT" || ag.AgentsType == "CHAT_AGENT" || ag.AgentsType == "GENERATION_AGENT" {
		cfg := &AgentsConfig{AgentsID: ag.ID, PromptType: "simple"}
		if provider, model, err := s.resolveDefaultLLMModel(ctx, organizationID, "agent creation"); err == nil && model != "" {
			cfg.ModelProvider = &provider
			cfg.ModelVersionID = &model
		} else if err != nil && !isAgentSuggestedQuestionsConfigurationError(err) {
			logger.WarnContext(ctx, "failed to resolve default LLM model for agent creation", "organization_id", organizationID, err)
		}
		if err := s.agentsRepo.CreateAgentsConfig(ctx, cfg); err != nil {
			return nil, err
		}
		ag.AgentsModelConfigID = &cfg.ID
	}

	if ag.AgentsType == "WORKFLOW" || ag.AgentsType == "CONVERSATIONAL_WORKFLOW" || ag.AgentsType == "CHAT_AGENT" {
		if s.workflowService != nil {
			// Create default workflow using SyncDraftWorkflow
			// defaultGraph := map[string]interface{}{
			// 	"nodes": []map[string]interface{}{
			// 		{
			// 			"id":   fmt.Sprintf("%d", time.Now().UnixMilli()),
			// 			"type": "custom",
			// 			"data": map[string]interface{}{
			// 				"type":      "start",
			// 				"title":     "Start",
			// 				"desc":      "",
			// 				"variables": []interface{}{},
			// 			},
			// 			"position": map[string]interface{}{
			// 				"x": 80,
			// 				"y": 282,
			// 			},
			// 			"targetPosition": "left",
			// 			"sourcePosition": "right",
			// 		},
			// 	},
			// 	"edges": []interface{}{},
			// }

			// defaultFeatures := map[string]interface{}{
			// 	"opening_statement":   "",
			// 	"suggested_questions": []string{},
			// 	"suggested_questions_after_answer": map[string]interface{}{
			// 		"enabled": false,
			// 	},
			// 	"speech_to_text": map[string]interface{}{
			// 		"enabled": false,
			// 	},
			// 	"text_to_speech": map[string]interface{}{
			// 		"enabled": false,
			// 	},
			// 	"retriever_resource": map[string]interface{}{
			// 		"enabled": false,
			// 	},
			// 	"annotation_reply": map[string]interface{}{
			// 		"enabled": false,
			// 	},
			// 	"file_upload": map[string]interface{}{
			// 		"image": map[string]interface{}{
			// 			"enabled":          false,
			// 			"number_limits":    3,
			// 			"detail":           "high",
			// 			"transfer_methods": []string{"remote_url", "local_file"},
			// 		},
			// 	},
			// }

			syncReq := &dto.SyncDraftWorkflowRequest{
				Graph:    nil,
				Features: nil,
				Type:     changeWorkflowType(agentType),
				Internal: &internal,
			}

			logCtx := logger.WithFields(ctx,
				zap.String("agent_id", ag.ID.String()),
				zap.String("tenant_id", tenantID),
				zap.String("account_id", accountID),
			)

			// Create workflow
			logger.DebugContext(logCtx, "creating default workflow for agent")
			_, err := s.workflowService.SyncDraftWorkflow(ctx, tenantID, ag.ID.String(), syncReq, accountID)
			if err != nil {
				logger.ErrorContext(logCtx, "failed to create default workflow for agent", err)
				return nil, fmt.Errorf("failed to create default workflow: %w", err)
			}
			logger.DebugContext(logCtx, "default workflow created for agent")

			// Get the created workflow to set WorkflowID
			workflowData, err := s.workflowService.GetDraftWorkflow(ctx, ag.ID.String())
			if err != nil {
				logger.ErrorContext(logCtx, "failed to get default workflow for agent", err)
				return nil, fmt.Errorf("failed to get created workflow: %w", err)
			}
			logger.DebugContext(logCtx, "retrieved default workflow for agent")

			if workflowMap, ok := workflowData.(map[string]interface{}); ok {
				if workflowID, ok := workflowMap["id"].(string); ok {
					logCtx = logger.WithFields(logCtx, zap.String("workflow_id", workflowID))
					logger.DebugContext(logCtx, "found workflow id for agent")
					if workflowUUID, err := uuid.Parse(workflowID); err == nil {
						ag.WorkflowID = &workflowUUID
						logger.DebugContext(logCtx, "set workflow id for agent")
					} else {
						logger.ErrorContext(logCtx, "failed to parse workflow id for agent", err)
					}
				} else {
					logger.ErrorContext(logCtx, "workflow id missing from default workflow response")
				}
			} else {
				logger.ErrorContext(logCtx, "default workflow response has unexpected shape")
			}
		}
	}

	// Persist agent updates if any
	if ag.AgentsModelConfigID != nil || ag.WorkflowID != nil {
		if err := s.agentsRepo.Update(ctx, ag); err != nil {
			return nil, err
		}
	}

	return ag, nil
}

func (s *agentsService) GetAgent(ctx context.Context, agentID string) (interface{}, error) {
	// Implement detailed agent retrieval based on type and related entities
	ag, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	// Get current account ID from context
	accountID := ""
	if v := ctx.Value("account_id"); v != nil {
		if id, ok := v.(string); ok {
			accountID = id
		}
	}

	callerOrganizationID := ""
	if v := ctx.Value("tenant_id"); v != nil {
		if id, ok := v.(string); ok {
			callerOrganizationID = strings.TrimSpace(id)
		}
	}

	if accountID != "" && callerOrganizationID != "" && s.enterpriseService != nil {
		canView, err := s.enterpriseService.CheckWorkspaceOrganizationAnyPermission(
			ctx,
			callerOrganizationID,
			ag.TenantID.String(),
			accountID,
			agentAssetVisiblePermissionCodes()...,
		)
		if err != nil {
			logger.Error(fmt.Sprintf("GetAgent: failed to check workspace permission for agent %s, account %s", agentID, accountID), err)
			return nil, fmt.Errorf("failed to verify permissions")
		}
		if !canView {
			return nil, fmt.Errorf("permission denied")
		}
	}

	// Check edit permission using permission service
	canEdit := false
	if accountID != "" && s.resourcePermissionService != nil {
		var createdBy string
		if ag.CreatedBy != nil {
			createdBy = ag.CreatedBy.String()
		}

		canEditResult, err := s.resourcePermissionService.CheckSingleResourceEditPermission(ctx, interfaces.SingleResourcePermissionParams{
			AccountID:       accountID,
			TenantID:        ag.TenantID.String(),
			OrganizationID:  callerOrganizationID,
			CreatedBy:       createdBy,
			GroupID:         nil, // Agents don't have group_id
			PermissionCodes: agentUpdatePermissionCodes(),
		})
		if err != nil {
			logger.Error(fmt.Sprintf("GetAgent: Failed to check edit permission for agent %s, account %s: %v", agentID, accountID, err), err)
			// Continue with canEdit=false on error
		} else {
			canEdit = canEditResult
		}
	}

	iconUrl := ""
	if ag.IconType != nil && *ag.IconType == "image" && ag.Icon != nil && *ag.Icon != "" {
		fileURL, err := s.fileService.GetFileURL(ctx, *ag.Icon)
		if err != nil {
			logger.Warn(fmt.Sprintf("failed to get file URL for icon %s: %v", *ag.Icon, err))
		} else {
			iconUrl = fileURL
		}
	}

	isPublished := false
	if hasPublished, err := s.hasPublishedVersion(ctx, ag); err == nil {
		isPublished = hasPublished
	} else {
		logger.Warn("GetAgent: Failed to check published workflow", map[string]interface{}{
			"agent_id": ag.ID.String(),
			"error":    err.Error(),
		})
	}

	resp := map[string]interface{}{
		"id":             ag.ID.String(),
		"name":           ag.Name,
		"description":    ag.Description,
		"agent_type":     ag.AgentsType,
		"icon_type":      ag.IconType,
		"icon":           ag.Icon,
		"icon_url":       iconUrl,
		"enable_api":     ag.EnableAPI,
		"web_app_id":     ag.WebAppID.String(),
		"web_app_status": string(NormalizeAgentWebAppStatus(ag.WebAppStatus)),
		"is_published":   isPublished,
		"created_at":     ag.CreatedAt.Unix(),
		"updated_at":     ag.UpdatedAt.Unix(),
		"is_editor":      false,
		"can_edit":       canEdit,
	}

	if ag.CreatedBy != nil {
		createdBy := ag.CreatedBy.String()
		resp["created_by"] = createdBy
	}
	if ag.UpdatedBy != nil {
		updatedBy := ag.UpdatedBy.String()
		resp["updated_by"] = updatedBy
	}

	// Permission from extension (best-effort)
	if ext, err := s.agentsRepo.GetExtensionByAgentID(ctx, ag.ID.String()); err == nil && ext != nil && ext.Permission != nil {
		resp["permission"] = *ext.Permission
	}

	// Set internal field from agent
	resp["internal"] = ag.Internal

	if ag.AgentsType == "WORKFLOW" || ag.AgentsType == "CONVERSATIONAL_WORKFLOW" || ag.AgentsType == "CHAT_AGENT" {
		// Workflow information will be handled by workflow service
		resp["workflow"] = nil
		resp["agent_config"] = nil
	} else {
		if ag.AgentsModelConfigID != nil {
			if cfg, err := s.agentsRepo.GetAgentsConfigByID(ctx, ag.AgentsModelConfigID.String()); err == nil && cfg != nil {
				cfgMap := map[string]interface{}{
					"id":               cfg.ID.String(),
					"model_provider":   cfg.ModelProvider,
					"model_version_id": cfg.ModelVersionID,
					"prompt_type":      cfg.PromptType,
					"created_at":       cfg.CreatedAt.Unix(),
					"updated_at":       cfg.UpdatedAt.Unix(),
				}
				resp["agent_config"] = cfgMap
			}
		}
	}

	// Attach tenant brief
	tenantID := ag.TenantID.String()
	if t, err := s.tenantService.GetWorkspaceByID(ctx, tenantID); err == nil && t != nil {
		resp["tenant"] = map[string]interface{}{"id": t.ID, "name": t.Name}
	}

	// Attach owner account brief
	if ag.CreatedBy != nil {
		if owner, err := s.accountService.GetAccountByID(ctx, ag.CreatedBy.String()); err == nil && owner != nil {
			resp["owner_account"] = map[string]interface{}{"id": owner.ID, "name": owner.Name}
		}
	}

	// Determine is_editor based on current account in context
	if ag.CreatedBy != nil && accountID != "" {
		resp["is_editor"] = strings.EqualFold(accountID, ag.CreatedBy.String())
	}

	return resp, nil
}

func (s *agentsService) UpdateAgent(ctx context.Context, agentID string, req interface{}) (interface{}, error) {
	// Validate agent ID
	if strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("invalid agent ID")
	}

	// Get current account ID from context
	accountID := ""
	if v := ctx.Value("account_id"); v != nil {
		if id, ok := v.(string); ok {
			accountID = id
		}
	}
	if accountID == "" {
		return nil, fmt.Errorf("unauthorized: account ID not found in context")
	}

	// Load agent
	ag, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("agent not found")
	}

	creatorID := ""
	if ag.CreatedBy != nil {
		creatorID = ag.CreatedBy.String()
	}

	canUpdate := false
	callerOrganizationID := ""
	if v := ctx.Value("tenant_id"); v != nil {
		if id, ok := v.(string); ok {
			callerOrganizationID = strings.TrimSpace(id)
		}
	}

	if callerOrganizationID != "" && s.enterpriseService != nil {
		canUpdate, err = s.enterpriseService.CheckWorkspaceOrganizationAnyPermission(
			ctx,
			callerOrganizationID,
			ag.TenantID.String(),
			accountID,
			agentUpdatePermissionCodes()...,
		)
		if err != nil {
			logger.Error(fmt.Sprintf("UpdateAgent: failed to check workspace permission for agent %s, account %s", agentID, accountID), err)
			return nil, fmt.Errorf("failed to verify permissions")
		}
	} else if s.resourcePermissionService != nil {
		canUpdate, err = s.resourcePermissionService.CheckSingleResourceEditPermission(ctx, interfaces.SingleResourcePermissionParams{
			AccountID:       accountID,
			TenantID:        ag.TenantID.String(),
			OrganizationID:  callerOrganizationID,
			CreatedBy:       creatorID,
			GroupID:         nil,
			PermissionCodes: agentUpdatePermissionCodes(),
		})
		if err != nil {
			logger.Error(fmt.Sprintf("UpdateAgent: failed to check edit permission for agent %s, account %s", agentID, accountID), err)
			return nil, fmt.Errorf("failed to verify permissions")
		}
	} else if creatorID != "" && strings.EqualFold(creatorID, accountID) {
		canUpdate = true
	}

	if !canUpdate {
		return nil, fmt.Errorf("permission denied")
	}

	// Parse request fields
	var (
		namePtr        *string
		descPtr        *string
		iconTypePtr    *string
		iconPtr        *string
		workspaceIDPtr *string

		internalPtr *bool
	)

	switch v := req.(type) {
	case map[string]interface{}:
		if s2, ok := v["name"].(string); ok {
			namePtr = &s2
		}
		if s2, ok := v["description"].(string); ok {
			descPtr = &s2
		}
		if s2, ok := v["icon_type"].(string); ok {
			iconTypePtr = &s2
		}
		if s2, ok := v["icon"].(string); ok {
			iconPtr = &s2
		}
		if s2, ok := v["tenant_id"].(string); ok {
			workspaceIDPtr = &s2
		}

		if b, ok := v["internal"].(bool); ok {
			internalPtr = &b
		}
	case *dto.CreateAgentRequest:
		if v != nil {
			if v.Name != "" {
				namePtr = &v.Name
			}
			if v.Description != "" {
				descPtr = &v.Description
			}
			if v.IconType != "" {
				iconTypePtr = &v.IconType
			}
			if v.Icon != "" {
				iconPtr = &v.Icon
			}
			if v.WorkspaceID != "" {
				workspaceIDPtr = &v.WorkspaceID
			}

			if v.Internal != nil {
				internalPtr = v.Internal
			}
		}
	case dto.CreateAgentRequest:
		if v.Name != "" {
			namePtr = &v.Name
		}
		if v.Description != "" {
			descPtr = &v.Description
		}
		if v.IconType != "" {
			iconTypePtr = &v.IconType
		}
		if v.Icon != "" {
			iconPtr = &v.Icon
		}
		if v.WorkspaceID != "" {
			workspaceIDPtr = &v.WorkspaceID
		}

		if v.Internal != nil {
			internalPtr = v.Internal
		}
	default:
		// Allow empty update
	}

	// Apply name change with duplicate check (within target tenant)
	if namePtr != nil {
		newName := strings.TrimSpace(*namePtr)
		if newName == "" {
			return nil, fmt.Errorf("invalid name")
		}
		// Determine target workspace for name conflict check.
		workspaceForName := ag.TenantID.String()
		if workspaceIDPtr != nil && strings.TrimSpace(*workspaceIDPtr) != "" {
			workspaceForName = strings.TrimSpace(*workspaceIDPtr)
		}
		if !strings.EqualFold(newName, ag.Name) {
			if exists, err := s.agentsRepo.ExistsByName(ctx, workspaceForName, newName); err != nil {
				return nil, err
			} else if exists {
				return nil, fmt.Errorf("agent with the same name already exists")
			}
		}
		ag.Name = newName
	}

	// Apply description
	if descPtr != nil {
		ag.Description = *descPtr
	}
	// Apply icon/icon_type
	if iconTypePtr != nil {
		val := strings.TrimSpace(*iconTypePtr)
		ag.IconType = &val
	}
	if iconPtr != nil {
		val := strings.TrimSpace(*iconPtr)
		ag.Icon = &val
	}

	// Apply workspace change.
	if workspaceIDPtr != nil && strings.TrimSpace(*workspaceIDPtr) != "" {
		if uid, err := uuid.Parse(strings.TrimSpace(*workspaceIDPtr)); err == nil {
			ag.TenantID = uid
		} else {
			return nil, fmt.Errorf("invalid tenant_id")
		}
	}

	// Apply internal field
	if internalPtr != nil {
		ag.Internal = *internalPtr
	}

	// Update audit fields
	if uid, err := uuid.Parse(accountID); err == nil {
		ag.UpdatedBy = &uid
	}
	ag.UpdatedAt = time.Now()

	// Persist agent
	if err := s.agentsRepo.Update(ctx, ag); err != nil {
		return nil, err
	}

	// Update workflow's internal field if provided
	if internalPtr != nil && s.workflowService != nil {
		// Get the current workflow to update its internal field
		workflowData, err := s.workflowService.GetDraftWorkflow(ctx, ag.ID.String())
		if err == nil && workflowData != nil {
			if workflowMap, ok := workflowData.(map[string]interface{}); ok {
				// Prepare sync request with current workflow data and updated internal field
				syncReq := &dto.SyncDraftWorkflowRequest{
					Internal: internalPtr,
				}

				// Extract existing graph and features to preserve them
				if graph, ok := workflowMap["graph"].(map[string]interface{}); ok {
					syncReq.Graph = graph
				}
				if features, ok := workflowMap["features"].(map[string]interface{}); ok {
					syncReq.Features = features
				}
				if workflowType, ok := workflowMap["type"].(dto.WorkflowType); ok {
					syncReq.Type = workflowType
				} else if workflowTypeStr, ok := workflowMap["type"].(string); ok {
					syncReq.Type = dto.WorkflowType(workflowTypeStr)
				}

				// Extract environment and conversation variables
				if envVars, ok := workflowMap["environment_variables"].([]dto.Variable); ok {
					syncReq.EnvironmentVariables = envVars
				}
				if convVars, ok := workflowMap["conversation_variables"].([]dto.Variable); ok {
					syncReq.ConversationVariables = convVars
				}

				// Update the workflow with the new internal field
				_, updateErr := s.workflowService.SyncDraftWorkflow(ctx, ag.TenantID.String(), ag.ID.String(), syncReq, accountID)
				if updateErr != nil {
					logger.Error("Failed to update workflow internal field: %v", updateErr)
					// Don't fail the entire update operation if workflow update fails
				}
			}
		}
	}

	return ag, nil
}

func (s *agentsService) DeleteAgent(ctx context.Context, agentID string) error {
	// Validate agent ID parameter
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("invalid agent ID")
	}

	// Get current account ID from context
	accountID := ""
	if v := ctx.Value("account_id"); v != nil {
		if id, ok := v.(string); ok {
			accountID = id
		}
	}
	if accountID == "" {
		return fmt.Errorf("unauthorized: account ID not found in context")
	}

	// Get agent by ID to check if it exists and get creator info
	agent, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil {
		logger.Error("Failed to get agent by ID: %v", err)
		return fmt.Errorf("agent not found")
	}

	// Step 1: Get organization ID from workspace for quota recording.
	var groupID *uuid.UUID
	workspaceID := agent.TenantID.String()
	if s.enterpriseService != nil {
		group, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, workspaceID)
		if err == nil && group != nil {
			// Parse organization ID string to UUID.
			parsedGroupID, parseErr := uuid.Parse(group.ID)
			if parseErr == nil {
				groupID = &parsedGroupID
			}
		} else if err != nil {
			logger.Warn("DeleteAgent: failed to resolve organization for workspace", map[string]interface{}{
				"agent_id":      agentID,
				"workspace_id":  workspaceID,
				"account_id":    accountID,
				"error_message": err.Error(),
			})
		}
	}

	creatorID := ""
	if agent.CreatedBy != nil {
		creatorID = agent.CreatedBy.String()
	}

	canDelete := false
	callerOrganizationID := ""
	if v := ctx.Value("tenant_id"); v != nil {
		if id, ok := v.(string); ok {
			callerOrganizationID = strings.TrimSpace(id)
		}
	}

	if callerOrganizationID != "" && s.enterpriseService != nil {
		canDelete, err = s.enterpriseService.CheckWorkspaceOrganizationAnyPermission(
			ctx,
			callerOrganizationID,
			workspaceID,
			accountID,
			agentDeletePermissionCodes()...,
		)
		if err != nil {
			logger.Error(fmt.Sprintf("DeleteAgent: failed to check workspace permission for agent %s, account %s", agentID, accountID), err)
			return fmt.Errorf("failed to verify permissions")
		}
	} else if s.resourcePermissionService != nil {
		canDelete, err = s.resourcePermissionService.CheckSingleResourceEditPermission(ctx, interfaces.SingleResourcePermissionParams{
			AccountID:       accountID,
			TenantID:        workspaceID,
			OrganizationID:  callerOrganizationID,
			CreatedBy:       creatorID,
			GroupID:         uuidPtrToString(groupID),
			PermissionCodes: agentDeletePermissionCodes(),
		})
		if err != nil {
			logger.Error(fmt.Sprintf("DeleteAgent: failed to check edit permission for agent %s, account %s", agentID, accountID), err)
			return fmt.Errorf("failed to verify permissions")
		}
	}
	if !canDelete {
		logger.Info("DeleteAgent: permission denied", map[string]interface{}{
			"agent_id":     agentID,
			"account_id":   accountID,
			"creator_id":   creatorID,
			"workspace_id": workspaceID,
		})
		return fmt.Errorf("permission denied")
	}

	// Step 2: Perform soft delete and record usage in transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Perform soft delete by calling repository
		if err := tx.WithContext(ctx).Model(&Agent{}).Where("id = ?", agentID).Update("deleted_at", time.Now()).Error; err != nil {
			return fmt.Errorf("failed to delete agent: %w", err)
		}

		// Step 3: Record usage history if groupID exists
		if groupID != nil && s.quotaService != nil {
			// Parse accountID to UUID
			accountUUID, err := uuid.Parse(accountID)
			if err != nil {
				return fmt.Errorf("failed to parse account ID: %w", err)
			}

			// Create usage history record with negative delta
			agentIDCopy := agentID
			agentNameCopy := agent.Name
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      *groupID,
				AccountID:    accountUUID,
				TenantID:     &agent.TenantID,
				ResourceType: quota_model.ResourceTypeAIAgents,
				Delta:        -1, // -1 for deleting an agent
				ResourceID:   &agentIDCopy,
				ResourceName: &agentNameCopy,
				Metadata: &quota_model.JSONMap{
					"agent_id":   agentID,
					"agent_name": agent.Name,
					"action":     "deleted",
				},
			}

			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record AI agent deletion: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		logger.Error("Failed to delete agent: %v", err)
		return fmt.Errorf("failed to delete agent")
	}

	logger.Info("Agent %s successfully deleted by user %s", agentID, accountID)
	return nil
}
