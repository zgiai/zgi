package execution

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// TrustedOperationRiskPolicy is a platform-owned classification bound to the
// complete stable leaf identity visible in the admitted catalog. Plugin risk
// hints are deliberately absent from this contract and cannot create or lower
// a trusted classification.
type TrustedOperationRiskPolicy struct {
	ConnectorID    core.Identifier
	ProviderID     core.Identifier
	DriverID       core.Identifier
	AuthMethodID   core.Identifier
	OperationID    core.Identifier
	ToolName       string
	Classification RiskClassification
}

type trustedOperationRiskKey struct {
	connectorID  string
	providerID   string
	driverID     string
	authMethodID string
	operationID  string
	toolName     string
}

type pinnedRiskContractContextKey struct{}

type pinnedRiskContract struct {
	definition catalog.Definition
	operation  catalog.Operation
}

func withPinnedRiskContract(
	ctx context.Context,
	definition catalog.Definition,
	operation catalog.Operation,
) context.Context {
	return context.WithValue(ctx, pinnedRiskContractContextKey{}, pinnedRiskContract{
		definition: definition,
		operation:  operation,
	})
}

// TrustedRiskResolver resolves only explicitly curated platform policy. A
// missing definition, changed operation identity, changed tool mapping, or
// absent policy is unclassified and therefore fail-closed in Service.
type TrustedRiskResolver struct {
	catalog  DefinitionCatalog
	policies map[trustedOperationRiskKey]RiskClassification
}

var _ RiskResolver = (*TrustedRiskResolver)(nil)

func NewTrustedRiskResolver(
	definitionCatalog DefinitionCatalog,
	policies []TrustedOperationRiskPolicy,
) (*TrustedRiskResolver, error) {
	if definitionCatalog == nil {
		return nil, connectorError(core.ErrorCodeInternal, false)
	}
	trusted := make(map[trustedOperationRiskKey]RiskClassification, len(policies))
	for _, policy := range policies {
		if !policy.ConnectorID.Valid() || !policy.ProviderID.Valid() || !policy.DriverID.Valid() ||
			!policy.AuthMethodID.Valid() || !policy.OperationID.Valid() ||
			!validOpaqueText(policy.ToolName, 128) ||
			(policy.Classification != RiskTrustedLow && policy.Classification != RiskHigh) {
			return nil, connectorError(core.ErrorCodeInvalidInput, false)
		}
		key := trustedOperationRiskKey{
			connectorID: policy.ConnectorID.String(), providerID: policy.ProviderID.String(),
			driverID: policy.DriverID.String(), authMethodID: policy.AuthMethodID.String(),
			operationID: policy.OperationID.String(), toolName: policy.ToolName,
		}
		if _, duplicate := trusted[key]; duplicate {
			return nil, connectorError(core.ErrorCodeConflict, false)
		}
		trusted[key] = policy.Classification
	}
	return &TrustedRiskResolver{catalog: definitionCatalog, policies: trusted}, nil
}

func (resolver *TrustedRiskResolver) ResolveRisk(ctx context.Context, request RiskRequest) (RiskClassification, error) {
	if resolver == nil || resolver.catalog == nil || resolver.policies == nil {
		return RiskUnclassified, connectorError(core.ErrorCodeInternal, false)
	}
	if request.OrganizationID == uuid.Nil || request.OwnerAccountID == uuid.Nil ||
		request.ActingPrincipalID == uuid.Nil || request.ConnectionID == uuid.Nil ||
		!request.ConnectorID.Valid() || request.Target.Validate() != nil || !request.OperationID.Valid() {
		return RiskUnclassified, connectorError(core.ErrorCodeInvalidInput, false)
	}
	if request.ActingPrincipalID != request.OwnerAccountID {
		return RiskUnclassified, connectorError(core.ErrorCodeAccessDenied, false)
	}
	definition, operation, pinned := riskContractFromContext(ctx, request)
	if !pinned {
		var ok bool
		definition, ok = resolver.catalog.Get(request.ConnectorID.String())
		if !ok || catalog.Validate(definition) != nil {
			return RiskUnclassified, nil
		}
		operation, ok = catalogOperation(definition, request.OperationID.String())
		if !ok {
			return RiskUnclassified, nil
		}
	}
	classification, ok := resolver.policies[trustedOperationRiskKey{
		connectorID: definition.ID, providerID: definition.ProviderID,
		driverID: definition.DriverID, authMethodID: definition.AuthMethodID,
		operationID: operation.ID, toolName: operation.ToolName,
	}]
	if !ok {
		return RiskUnclassified, nil
	}
	return classification, nil
}

func riskContractFromContext(
	ctx context.Context,
	request RiskRequest,
) (catalog.Definition, catalog.Operation, bool) {
	if ctx == nil {
		return catalog.Definition{}, catalog.Operation{}, false
	}
	pinned, ok := ctx.Value(pinnedRiskContractContextKey{}).(pinnedRiskContract)
	if !ok || pinned.definition.ID != request.ConnectorID.String() ||
		pinned.operation.ID != request.OperationID.String() || catalog.Validate(pinned.definition) != nil {
		return catalog.Definition{}, catalog.Operation{}, false
	}
	operation, exists := catalogOperation(pinned.definition, pinned.operation.ID)
	if !exists || operation.ToolName != pinned.operation.ToolName {
		return catalog.Definition{}, catalog.Operation{}, false
	}
	return pinned.definition, pinned.operation, true
}

func catalogOperation(definition catalog.Definition, operationID string) (catalog.Operation, bool) {
	for _, operation := range definition.Operations {
		if operation.ID == operationID {
			return operation, true
		}
	}
	return catalog.Operation{}, false
}
