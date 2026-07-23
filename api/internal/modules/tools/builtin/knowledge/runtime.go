package knowledge

import (
	"fmt"
	"strings"

	dataset_service "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type knowledgeRuntime struct {
	runtime  *tools.ToolRuntime
	tenantID string
}

func (r knowledgeRuntime) scope(userID string, appID *string) (dataset_service.KnowledgeScope, error) {
	tenantID := strings.TrimSpace(r.tenantID)
	if r.runtime != nil && strings.TrimSpace(r.runtime.TenantID) != "" {
		tenantID = strings.TrimSpace(r.runtime.TenantID)
	}
	organizationID := ""
	workspaceID := ""
	accountID := strings.TrimSpace(userID)
	if r.runtime != nil {
		organizationID = strings.TrimSpace(stringValue(r.runtime.RuntimeParameters, "organization_id"))
		workspaceID = strings.TrimSpace(stringValue(r.runtime.RuntimeParameters, "workspace_id"))
		if r.runtime.InvokeFrom == tools.ToolInvokeFromAgent {
			if boundBy := strings.TrimSpace(stringValue(r.runtime.RuntimeParameters, "knowledge_bound_by_account_id")); boundBy != "" {
				accountID = boundBy
			}
		}
	}
	if accountID == "" {
		return dataset_service.KnowledgeScope{}, fmt.Errorf("account_id is required")
	}
	if organizationID == "" && workspaceID == "" {
		if r.runtime != nil && r.runtime.InvokeFrom == tools.ToolInvokeFromAIChat {
			organizationID = tenantID
		} else {
			workspaceID = tenantID
		}
	}
	if organizationID == "" && workspaceID == "" {
		return dataset_service.KnowledgeScope{}, fmt.Errorf("organization_id or workspace_id is required")
	}
	scope := dataset_service.KnowledgeScope{
		WorkspaceID:    workspaceID,
		OrganizationID: organizationID,
		AccountID:      accountID,
	}
	if appID != nil {
		scope.AppID = strings.TrimSpace(*appID)
	}
	return scope, nil
}

func (r knowledgeRuntime) applyAgentConfig(req *dataset_service.KnowledgeRetrieveRequest) {
	if r.runtime == nil || req == nil || len(r.runtime.RuntimeParameters) == 0 {
		return
	}
	if datasetIDs := stringListValue(r.runtime.RuntimeParameters, "knowledge_dataset_ids"); len(datasetIDs) > 0 {
		req.DatasetIDs = datasetIDs
		for _, datasetID := range datasetIDs {
			if authorization, ok := tools.AgentBindingAuthorizationFor(
				r.runtime.RuntimeParameters,
				"knowledge_dataset",
				"",
				datasetID,
				"read",
			); ok {
				req.Scope.AccountID = authorization.BoundByAccountID
				break
			}
		}
	}
	if config := mapValue(r.runtime.RuntimeParameters, "knowledge_retrieval_config"); len(config) > 0 {
		req.RetrievalConfig = config
	}
	if boolValue(r.runtime.RuntimeParameters, "knowledge_binding_grant") {
		req.AgentBindingGrant = true
	}
}

func (r knowledgeRuntime) authorize(kind string) error {
	if r.runtime == nil {
		return nil
	}
	switch kind {
	case ToolListAccessibleKnowledge, ToolRetrieveKnowledge:
		if r.runtime.InvokeFrom != "" && r.runtime.InvokeFrom != tools.ToolInvokeFromAIChat {
			return fmt.Errorf("%s is only available to internal AIChat skills", kind)
		}
	case ToolRetrieveAgentKnowledge:
		if r.runtime.InvokeFrom != "" && r.runtime.InvokeFrom != tools.ToolInvokeFromAIChat && r.runtime.InvokeFrom != tools.ToolInvokeFromAgent {
			return fmt.Errorf("%s is only available to AIChat or Agent skill runtimes", kind)
		}
	}
	return nil
}
