package digitalhuman

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
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
	if resolved == nil || resolved.Snapshot.Subject.ID == uuid.Nil || resolved.Snapshot.Revision == "" || strings.TrimSpace(resolved.Snapshot.IdentityName) == "" || strings.TrimSpace(resolved.Snapshot.Model) == "" {
		return chatruntime.RunConfig{}, ErrInvalidRequest
	}
	snapshotRef := resourcebindings.SnapshotRef{
		Subject: resolved.Snapshot.Subject, Phase: resourcebindings.PhasePublished, VersionID: &resolved.Snapshot.DigitalHumanVersion,
	}
	config := chatruntime.RunConfig{
		FileUploadEnabled: new(fileUploadEnabled(&Version{FileUploadPolicy: resolved.Snapshot.FileUploadPolicy})),
		FileUploadPolicy:  chatfile.Clone(resolved.Snapshot.FileUploadPolicy),
		SystemPrompt:      buildDigitalHumanSystemPrompt(resolved.Snapshot), SystemPromptVersion: resolved.Snapshot.Revision,
		Model: resolved.Snapshot.Model, BillingAppID: resolved.Snapshot.Subject.ID.String(),
		BillingAppType:     runtimemodel.ConversationCallerDigitalHuman,
		RuntimeInputPolicy: chatruntime.RuntimeInputPolicyCheckpointAndFollowUp,
		UseMemory:          false, AgentMemoryEnabled: false, AgentMemoryAutoExtractionEnabled: false,
		BindingReader: a.bindingReader, BindingSnapshot: snapshotRef,
		ConnectorTargetScope: "published", ConnectorTargetVersionID: resolved.Snapshot.DigitalHumanVersion.String(),
	}
	databaseByID := make(map[string]*chatruntime.AgentDatabaseBinding)
	for _, binding := range resolved.Snapshot.Bindings {
		authorization := chatruntime.ResourceBindingAuthorization{
			BindingType: string(binding.Type), ResourceID: binding.ResourceID, ParentResourceID: binding.ParentResourceID,
			AccessMode: string(binding.AccessMode),
		}
		if binding.Authorization.AuthorizedBy != nil {
			authorization.BoundByAccountID = binding.Authorization.AuthorizedBy.String()
		}
		if binding.Authorization.AuthorizedAt != nil {
			authorization.BoundAtUnix = binding.Authorization.AuthorizedAt.Unix()
		}
		if binding.Type == resourcebindings.BindingTypeWorkflow && authorization.ParentResourceID == "" {
			// Workflow bindings published before the parent was keyed by the
			// workflow Agent id keep an empty parent; evidence uses the same key
			// as Agent bindings so tool governance can find it.
			authorization.ParentResourceID = stringMapValue(binding.Metadata, "workflow_agent_id")
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
			// Published legacy rows remain immutable. Only an independently
			// authorized native companion binding may expose external operations.
		}
	}
	config.BindingAuthorizations = append(config.BindingAuthorizations, derivedDatabaseAuthorizations(config.BindingAuthorizations)...)
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
	return config, nil
}

// derivedDatabaseAuthorizations adds read evidence for a data source that is
// reached only through table bindings. Agent bindings always carry this parent
// evidence, and database discovery tools require it; the tools still return
// only the bound tables. The most recent table authorization is used so the
// evidence never predates or outlives a concrete table grant.
func derivedDatabaseAuthorizations(authorizations []chatruntime.ResourceBindingAuthorization) []chatruntime.ResourceBindingAuthorization {
	explicit := make(map[string]struct{})
	latest := make(map[string]chatruntime.ResourceBindingAuthorization)
	for _, authorization := range authorizations {
		switch resourcebindings.BindingType(authorization.BindingType) {
		case resourcebindings.BindingTypeDatabase:
			explicit[authorization.ResourceID] = struct{}{}
		case resourcebindings.BindingTypeDatabaseTable:
			if authorization.ParentResourceID == "" || authorization.BoundByAccountID == "" || authorization.BoundAtUnix <= 0 {
				continue
			}
			current, ok := latest[authorization.ParentResourceID]
			if !ok || authorization.BoundAtUnix > current.BoundAtUnix ||
				(authorization.BoundAtUnix == current.BoundAtUnix && authorization.BoundByAccountID < current.BoundByAccountID) {
				latest[authorization.ParentResourceID] = authorization
			}
		}
	}
	dataSourceIDs := make([]string, 0, len(latest))
	for dataSourceID := range latest {
		if _, ok := explicit[dataSourceID]; !ok {
			dataSourceIDs = append(dataSourceIDs, dataSourceID)
		}
	}
	sort.Strings(dataSourceIDs)
	derived := make([]chatruntime.ResourceBindingAuthorization, 0, len(dataSourceIDs))
	for _, dataSourceID := range dataSourceIDs {
		table := latest[dataSourceID]
		derived = append(derived, chatruntime.ResourceBindingAuthorization{
			BindingType: string(resourcebindings.BindingTypeDatabase), ResourceID: dataSourceID,
			AccessMode: string(resourcebindings.AccessModeRead), BoundByAccountID: table.BoundByAccountID, BoundAtUnix: table.BoundAtUnix,
		})
	}
	return derived
}

func buildDigitalHumanSystemPrompt(snapshot ExecutionSnapshot) string {
	sections := []string{
		"Your name is " + strings.TrimSpace(snapshot.IdentityName) + ".",
		strings.TrimSpace(snapshot.Persona),
	}
	if len(snapshot.Goals) > 0 {
		sections = append(sections, "Work goals:\n- "+strings.Join(snapshot.Goals, "\n- "))
	}
	if len(snapshot.Rules) > 0 {
		sections = append(sections, "Rules:\n- "+strings.Join(snapshot.Rules, "\n- "))
	}
	sections = append(sections,
		"Use only capabilities and resources present in this execution snapshot.",
		"Do not use or modify account memory or Agent memory.",
	)
	if snapshot.UseUserMemory {
		sections = append(sections, "User relationship memory is a separate user-owned data source and may only be used through the dedicated relationship-memory context and tool supplied for this run.")
	} else {
		sections = append(sections, "User relationship memory is unavailable for this invocation.")
	}
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
