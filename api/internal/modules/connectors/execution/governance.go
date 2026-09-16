package execution

import (
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// connectorGovernanceManifest converts the admitted operation contract into
// the existing Tool Governance shape. It intentionally copies only public
// operation metadata; provider input and credentials never enter the grant
// matcher.
func connectorGovernanceManifest(
	definition catalog.Definition,
	operation catalog.Operation,
	risk RiskClassification,
	target core.TargetRef,
) toolgovernance.Manifest {
	riskLevel := toolgovernance.RiskLevelHigh
	if risk == RiskTrustedLow {
		riskLevel = toolgovernance.RiskLevelLow
	}
	assetType := strings.TrimSpace(string(target.Type))
	if assetType == "" {
		assetType = "connector_target"
	}
	return toolgovernance.Manifest{
		ToolID:                operation.ID,
		SkillID:               operation.SkillID,
		Domain:                definition.ID,
		Effect:                toolgovernance.Effect(operation.Effect),
		AssetType:             assetType,
		RiskLevel:             riskLevel,
		Reversible:            operation.Effect != catalog.OperationEffectDelete,
		ExternalSideEffect:    operation.Effect != catalog.OperationEffectRead,
		DataEgress:            operation.DataEgress,
		ExternalDestination:   operation.ExternalDestination,
		SensitiveDataAllowed:  operation.SensitiveDataAllowed,
		DefaultApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk,
		AuditRequired:         true,
		IdempotencyRequired:   operation.Idempotent,
	}
}

func connectorGovernanceAssets(target core.TargetRef) []toolgovernance.AssetRef {
	if target.Validate() != nil {
		return nil
	}
	return []toolgovernance.AssetRef{{
		ID:   strings.TrimSpace(target.ID),
		Type: strings.TrimSpace(string(target.Type)),
	}}
}

func connectorGovernanceRequest(request ExecutionRequest, definition catalog.Definition, operation catalog.Operation, risk RiskClassification) toolgovernance.Request {
	conversationID := strings.TrimSpace(request.ConversationID)
	if conversationID == "" && request.Target.Type == core.TargetTypeChat {
		conversationID = strings.TrimSpace(request.Target.ID)
	}
	return toolgovernance.Request{
		Manifest:       connectorGovernanceManifest(definition, operation, risk, request.Target),
		ConversationID: conversationID,
		OrganizationID: request.OrganizationID.String(),
		UserID:         request.OwnerAccountID.String(),
		SkillID:        strings.TrimSpace(operation.SkillID),
		ProviderType:   "connector",
		ProviderID:     strings.TrimSpace(definition.ID),
		ConnectionID:   request.ConnectionID.String(),
		BindingID:      request.BindingID.String(),
		Assets:         connectorGovernanceAssets(request.Target),
		SessionGrants:  request.SessionGrants,
	}
}

func connectorSessionGrantMatches(request ExecutionRequest, definition catalog.Definition, operation catalog.Operation, risk RiskClassification) (toolgovernance.SessionGrant, bool) {
	if len(request.SessionGrants) == 0 || request.OwnerAccountID == uuid.Nil {
		return toolgovernance.SessionGrant{}, false
	}
	governanceRequest := connectorGovernanceRequest(request, definition, operation, risk)
	return toolgovernance.MatchSessionGrant(
		governanceRequest,
		governanceRequest.Manifest,
		governanceRequest.Assets,
	)
}
