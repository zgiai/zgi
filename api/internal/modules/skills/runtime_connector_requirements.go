package skills

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// Requirements describe native capabilities; they never select credentials.
// The current target/owner resolver must authorize each required operation.
func requireConnectorRequirements(ctx context.Context, doc SkillDocument, execution ExecutionContext, callID string) error {
	for _, requirement := range doc.Metadata.ConnectorRequirements {
		if !requirement.Required {
			continue
		}
		operations := append([]string{}, requirement.OperationIDs...)
		requireAny := len(operations) == 0
		if requireAny {
			definitions, err := catalog.BuiltinDefinitions()
			if err != nil {
				return core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
			}
			for _, d := range definitions {
				if d.ID == requirement.ConnectorID {
					for _, operation := range d.Operations {
						operations = append(operations, operation.ID)
					}
				}
			}
		}
		if len(operations) == 0 {
			return core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
		}
		available := false
		for _, operation := range operations {
			_, err := resolveConnectorToolIdentity(ctx, doc.Metadata.ID, SkillToolDefinition{Name: operation, ProviderType: tools.ToolProviderTypeConnector, ProviderID: requirement.ConnectorID, ConnectorNative: true, ConnectorOperationID: operation}, execution, callID)
			if err == nil {
				available = true
				if requireAny {
					break
				}
			} else if !requireAny {
				return core.NewError(core.ErrorCodeTargetNotReady, false, nil)
			}
		}
		if !available {
			return core.NewError(core.ErrorCodeTargetNotReady, false, nil)
		}
	}
	return nil
}
