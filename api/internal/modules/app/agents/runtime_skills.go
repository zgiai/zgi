package agents

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/util"
	"strings"
)

func (h *AgentsHandler) validateAgentRuntimeSkills(c *gin.Context, req dto.AgentConfigRequest) error {
	skillIDs := req.EnabledSkillIDs
	if h.chatRuntimeService == nil || len(skillIDs) == 0 {
		return nil
	}
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		return fmt.Errorf("unauthorized")
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil {
		return fmt.Errorf("unauthorized")
	}
	scope := runtimeservice.Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}
	skillsMetadata, err := h.chatRuntimeService.ListSkills(c.Request.Context(), scope)
	if err != nil {
		return err
	}
	metadataByID := make(map[string]runtimedto.SkillResponse, len(skillsMetadata))
	for _, item := range skillsMetadata {
		metadataByID[strings.ToLower(strings.TrimSpace(item.ID))] = skillResponseFromMetadata(item)
	}
	for _, raw := range skillIDs {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if skills.IsHiddenSystemSkill(id) {
			continue
		}
		metadata, ok := metadataByID[id]
		if !ok {
			return fmt.Errorf("skill %s is not found", id)
		}
		if !skillResponseSupportsCaller(metadata, runtimemodel.ConversationCallerAgent) {
			return fmt.Errorf("skill %s is not available for agent", id)
		}
		if skillResponseRequires(metadata, "agent_knowledge") && len(req.KnowledgeDatasetIDs) == 0 {
			return fmt.Errorf("skill %s requires configured knowledge datasets", id)
		}
		if skillResponseRequires(metadata, "agent_database") && len(normalizeAgentDatabaseBindings(req.DatabaseBindings)) == 0 {
			return fmt.Errorf("skill %s requires configured database bindings", id)
		}
	}
	return nil
}

func skillResponseFromMetadata(metadata skills.SkillDiscoveryMetadata) runtimedto.SkillResponse {
	return runtimedto.SkillResponse{
		SkillID:          metadata.ID,
		SupportedCallers: metadata.SupportedCallers,
		RequiredConfig:   metadata.RequiredConfig,
	}
}

func skillResponseSupportsCaller(metadata runtimedto.SkillResponse, callerType string) bool {
	if len(metadata.SupportedCallers) == 0 {
		return true
	}
	for _, caller := range metadata.SupportedCallers {
		if strings.EqualFold(strings.TrimSpace(caller), callerType) {
			return true
		}
	}
	return false
}

func skillResponseRequires(metadata runtimedto.SkillResponse, requirement string) bool {
	for _, value := range metadata.RequiredConfig {
		if strings.EqualFold(strings.TrimSpace(value), requirement) {
			return true
		}
	}
	return false
}
