package digitalhuman

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
)

// BrainAdapter maps a sealed DigitalHuman snapshot into the existing ChatRuntime.
// It is a compatibility adapter only: DigitalHuman remains the caller and never gains an Agent ID.
type BrainAdapter struct {
	bindingReader resourcebindings.Reader
}

func NewBrainAdapter(bindingReader resourcebindings.Reader) *BrainAdapter {
	if bindingReader == nil {
		panic("DigitalHuman BrainAdapter requires binding reader")
	}
	return &BrainAdapter{bindingReader: bindingReader}
}

func (a *BrainAdapter) RunConfig(resolved *ResolvedExecution) (chatruntime.RunConfig, error) {
	if resolved == nil || resolved.Snapshot.Subject.ID == uuid.Nil || resolved.Snapshot.Revision == "" || strings.TrimSpace(resolved.Snapshot.Model) == "" {
		return chatruntime.RunConfig{}, ErrInvalidRequest
	}
	snapshotRef := resourcebindings.SnapshotRef{
		Subject: resolved.Snapshot.Subject, Phase: resourcebindings.PhasePublished, VersionID: &resolved.Snapshot.DigitalHumanVersion,
	}
	config := chatruntime.RunConfig{
		SystemPrompt: buildDigitalHumanSystemPrompt(resolved.Snapshot), SystemPromptVersion: resolved.Snapshot.Revision,
		Model: resolved.Snapshot.Model, BillingAppID: resolved.Snapshot.Subject.ID.String(),
		BillingAppType: runtimemodel.ConversationCallerDigitalHuman,
		UseMemory:      false, AgentMemoryEnabled: false, AgentMemoryAutoExtractionEnabled: false,
		BindingReader: a.bindingReader, BindingSnapshot: snapshotRef,
		IntegrationConnectionIDs: make(map[string]string), IntegrationSelectedConnectionIDs: make(map[string][]string),
	}
	databaseByID := make(map[string]*chatruntime.AgentDatabaseBinding)
	for _, binding := range resolved.Snapshot.Bindings {
		authorization := chatruntime.ResourceBindingAuthorization{
			BindingType: string(binding.Type), ResourceID: binding.ResourceID, ParentResourceID: binding.ParentResourceID,
			AccessMode: string(binding.AccessMode), AllowedActionIDs: resourcebindings.AllowedIntegrationActions(binding.Metadata),
		}
		if binding.Authorization.AuthorizedBy != nil {
			authorization.BoundByAccountID = binding.Authorization.AuthorizedBy.String()
		}
		if binding.Authorization.AuthorizedAt != nil {
			authorization.BoundAtUnix = binding.Authorization.AuthorizedAt.Unix()
		}
		config.BindingAuthorizations = append(config.BindingAuthorizations, authorization)
		switch binding.Type {
		case resourcebindings.BindingTypeSkill:
			config.EnabledSkillIDs = append(config.EnabledSkillIDs, binding.ResourceID)
		case resourcebindings.BindingTypeKnowledgeDataset:
			config.KnowledgeDatasetIDs = append(config.KnowledgeDatasetIDs, binding.ResourceID)
		case resourcebindings.BindingTypeDatabase:
			if _, ok := databaseByID[binding.ResourceID]; !ok {
				databaseByID[binding.ResourceID] = &chatruntime.AgentDatabaseBinding{DataSourceID: binding.ResourceID}
			}
		case resourcebindings.BindingTypeDatabaseTable:
			dataSourceID := binding.ParentResourceID
			if dataSourceID == "" {
				return chatruntime.RunConfig{}, fmt.Errorf("%w: database table binding requires parent resource", ErrResourceBindingInvalid)
			}
			database := databaseByID[dataSourceID]
			if database == nil {
				database = &chatruntime.AgentDatabaseBinding{DataSourceID: dataSourceID}
				databaseByID[dataSourceID] = database
			}
			database.TableIDs = append(database.TableIDs, binding.ResourceID)
			if binding.AccessMode == resourcebindings.AccessModeWrite {
				database.WritableTableIDs = append(database.WritableTableIDs, binding.ResourceID)
			}
		case resourcebindings.BindingTypeWorkflow:
			workflow, err := workflowBindingFromSnapshot(binding)
			if err != nil {
				return chatruntime.RunConfig{}, err
			}
			config.WorkflowBindings = append(config.WorkflowBindings, workflow)
		case resourcebindings.BindingTypeIntegrationConnection:
			integrationID := strings.ToLower(strings.TrimSpace(binding.ParentResourceID))
			if integrationID == "" {
				return chatruntime.RunConfig{}, fmt.Errorf("%w: integration binding requires parent resource", ErrResourceBindingInvalid)
			}
			config.IntegrationConnectionIDs[integrationID] = binding.ResourceID
			config.IntegrationSelectedConnectionIDs[integrationID] = append(config.IntegrationSelectedConnectionIDs[integrationID], binding.ResourceID)
		}
	}
	for _, database := range databaseByID {
		database.TableIDs = normalizedUniqueStrings(database.TableIDs)
		database.WritableTableIDs = normalizedUniqueStrings(database.WritableTableIDs)
		config.DatabaseBindings = append(config.DatabaseBindings, *database)
	}
	sort.Slice(config.DatabaseBindings, func(i, j int) bool {
		return config.DatabaseBindings[i].DataSourceID < config.DatabaseBindings[j].DataSourceID
	})
	config.EnabledSkillIDs = normalizedUniqueStrings(config.EnabledSkillIDs)
	config.KnowledgeDatasetIDs = normalizedUniqueStrings(config.KnowledgeDatasetIDs)
	for key, values := range config.IntegrationSelectedConnectionIDs {
		config.IntegrationSelectedConnectionIDs[key] = normalizedUniqueStrings(values)
	}
	return config, nil
}

func buildDigitalHumanSystemPrompt(snapshot ExecutionSnapshot) string {
	sections := []string{
		"You are " + strings.TrimSpace(snapshot.Name) + ".",
		strings.TrimSpace(snapshot.Persona),
	}
	if len(snapshot.Rules) > 0 {
		sections = append(sections, "Rules:\n- "+strings.Join(snapshot.Rules, "\n- "))
	}
	for _, role := range snapshot.Roles {
		var goals, responsibilities []string
		if role.Version != nil {
			_ = json.Unmarshal(role.Version.Goals, &goals)
			_ = json.Unmarshal(role.Version.Responsibilities, &responsibilities)
		}
		if len(goals) > 0 {
			sections = append(sections, "Role goals:\n- "+strings.Join(goals, "\n- "))
		}
		if len(responsibilities) > 0 {
			sections = append(sections, "Role responsibilities:\n- "+strings.Join(responsibilities, "\n- "))
		}
	}
	sections = append(sections,
		"Use only capabilities and resources present in this execution snapshot.",
		"Do not use or modify account memory or Agent memory.",
	)
	compact := make([]string, 0, len(sections))
	for _, section := range sections {
		if section = strings.TrimSpace(section); section != "" {
			compact = append(compact, section)
		}
	}
	return strings.Join(compact, "\n\n")
}

func workflowBindingFromSnapshot(binding resourcebindings.Binding) (chatruntime.AgentWorkflowBinding, error) {
	workflowID := stringMapValue(binding.Metadata, "workflow_id")
	workflowAgentID := stringMapValue(binding.Metadata, "workflow_agent_id")
	versionUUID := stringMapValue(binding.Metadata, "version_uuid")
	if workflowID == "" {
		workflowID = binding.ResourceID
	}
	if workflowAgentID == "" || workflowID == "" || versionUUID == "" {
		return chatruntime.AgentWorkflowBinding{}, fmt.Errorf("%w: workflow binding requires workflow_agent_id, workflow_id, and pinned version_uuid", ErrResourceBindingInvalid)
	}
	return chatruntime.AgentWorkflowBinding{
		BindingID: binding.ResourceID, Label: binding.DisplayName, AgentID: workflowAgentID, WorkflowID: workflowID,
		AgentType: "workflow", VersionStrategy: "pinned", VersionUUID: versionUUID,
	}, nil
}

func stringMapValue(values map[string]interface{}, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func normalizedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
