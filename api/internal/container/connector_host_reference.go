package container

import (
	"context"
	"encoding/json"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

// Recheck the persisted host selection at the final submission boundary. A
// canonical binding alone does not prove that a draft still selects it.
func (authority *connectorTargetAuthority) verifyHostReference(ctx context.Context, request execution.TargetAuthorizationRequest) error {
	b := request.Binding
	if b.Target.Type == core.TargetTypeChat {
		return nil
	}
	var row struct{ Document string }
	db := authority.container.db.WithContext(ctx)
	switch b.Target.Type {
	case core.TargetTypeAgent:
		if b.Target.EffectiveScope() == core.ScopePublished {
			db = db.Table("agent_published_versions").Select("config_snapshot AS document").Where("agent_id = ? AND version_uuid = ? AND deleted_at IS NULL", b.Target.ID, b.Target.VersionID)
		} else {
			db = db.Table("agents_configs").Select("agent_mode AS document").Where("agents_id = ? AND deleted_at IS NULL", b.Target.ID)
		}
	case core.TargetTypeWorkflow:
		db = db.Table("workflows").Select("graph AS document").Where("deleted_at IS NULL")
		if b.Target.EffectiveScope() == core.ScopePublished {
			db = db.Where("version_uuid = ? AND version <> ?", b.Target.VersionID, "draft")
		} else {
			db = db.Where("id = ? AND version = ?", b.Target.ID, "draft")
		}
	default:
		return targetDenied()
	}
	if db.Take(&row).Error != nil || !hostDocumentSelectsBinding([]byte(row.Document), request) {
		return targetDenied()
	}
	return nil
}

type hostConnectorReference struct {
	BindingID    string `json:"binding_id"`
	ConnectionID string `json:"connection_id"`
	ConnectorID  string `json:"connector_id"`
	OperationID  string `json:"operation_id"`
}

func hostDocumentSelectsBinding(raw []byte, request execution.TargetAuthorizationRequest) bool {
	b := request.Binding
	matches := func(ref hostConnectorReference) bool {
		return ref.BindingID == b.ID.String() && ref.ConnectionID == b.ConnectionID.String() && ref.ConnectorID == b.ConnectorID.String()
	}
	if b.Target.Type == core.TargetTypeAgent {
		var config struct {
			Bindings []hostConnectorReference `json:"connector_bindings"`
		}
		if json.Unmarshal(raw, &config) != nil {
			return false
		}
		count := 0
		for _, ref := range config.Bindings {
			if ref.ConnectorID == b.ConnectorID.String() {
				if !matches(ref) {
					return false
				}
				count++
			}
		}
		return count == 1
	}
	var graph struct {
		Nodes []struct {
			ID   string `json:"id"`
			Data struct {
				ProviderType string                 `json:"provider_type"`
				ProviderID   string                 `json:"provider_id"`
				Unavailable  json.RawMessage        `json:"connector_unavailable_ref"`
				Connector    hostConnectorReference `json:"connector"`
			} `json:"data"`
		} `json:"nodes"`
	}
	if json.Unmarshal(raw, &graph) != nil {
		return false
	}
	count := 0
	for _, node := range graph.Nodes {
		if node.ID != b.Target.BindingInstanceID {
			continue
		}
		if node.Data.ProviderType != "connector" || node.Data.ProviderID != b.ConnectorID.String() || len(node.Data.Unavailable) != 0 && string(node.Data.Unavailable) != "null" || !matches(node.Data.Connector) || node.Data.Connector.OperationID != request.OperationID.String() {
			return false
		}
		count++
	}
	return count == 1
}
