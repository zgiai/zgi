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
		// DigitalHuman runs are always approved per invocation. This keeps the
		// governance manifest consistent with the explicit boundary below, but
		// the boundary does not rely on this flag: a manifest is presentation
		// metadata and must never be the only thing stopping a dispatch.
		ApprovalEveryInvocation: target.Type == core.TargetTypeDigitalHuman,
		AuditRequired:           true,
		IdempotencyRequired:     operation.Idempotent,
	}
}

// digitalHumanApprovalDecision is the platform-owned approval decision for one
// invocation. It is deliberately not derived from a member setting, a plugin
// risk hint, or a governance manifest flag. The zero value leaves Chat, Agent,
// and Workflow on their existing shared path.
type digitalHumanApprovalDecision int

const (
	// digitalHumanApprovalShared means the target is not DigitalHuman; the
	// shared risk and session-grant behavior applies unchanged.
	digitalHumanApprovalShared digitalHumanApprovalDecision = iota
	// digitalHumanApprovalFreshConfirmation means the invocation requires the
	// shared fresh interactive confirmation gate: the platform decision is that
	// high/unclassified risk must be confirmed in this interactive request,
	// bound to the pinned receipt, and that a session grant must not stand in
	// for that confirmation.
	digitalHumanApprovalFreshConfirmation
	// digitalHumanApprovalNonInteractiveBlocked means the invocation needs an
	// interactive confirmation but the runtime cannot ask for one.
	digitalHumanApprovalNonInteractiveBlocked
	// digitalHumanApprovalFailClosed means the invocation is rejected before
	// any credential is opened or any provider is invoked.
	digitalHumanApprovalFailClosed
	// digitalHumanApprovalTrustedLow means a trusted-low operation, such as a
	// read, runs without confirmation, as it does for Chat and Agent, while
	// no session grant or caller preauthorization applies.
	digitalHumanApprovalTrustedLow
)

// governs reports whether the explicit DigitalHuman boundary applies.
func (decision digitalHumanApprovalDecision) governs() bool {
	return decision != digitalHumanApprovalShared
}

// sessionGrantPreauthorizes reports whether a matching reusable session grant
// may preauthorize the invocation. It is never true for a DigitalHuman target:
// a reusable grant is not sufficient authority for a DigitalHuman action. The
// shared Chat/Agent/Workflow session-grant behavior is unchanged.
func (decision digitalHumanApprovalDecision) sessionGrantPreauthorizes() bool {
	return !decision.governs()
}

// digitalHumanApproval is the explicit, platform-owned boundary for every
// TargetTypeDigitalHuman invocation. It encodes three non-negotiable rules:
//
//  1. A reusable session grant never preauthorizes a DigitalHuman invocation.
//  2. High or unclassified risk needs a fresh interactive confirmation bound to
//     the pinned receipt. Without that confirmation a non-interactive
//     invocation is blocked.
//  3. A resolver-supplied RiskTrustedLow classification runs without a
//     confirmation, like Chat and Agent reads. A low-risk confirmation
//     continuation is not a valid path and fails closed, as does any unknown
//     classification.
//
// It returns digitalHumanApprovalShared for every other target so existing
// Chat, Agent, and Workflow behavior is preserved. Now that the DigitalHuman
// surface gate (surfaceForTarget) is open for DigitalHuman, this is the
// reachable, platform-owned requirement that every DigitalHuman invocation
// carries a fresh interactive confirmation bound to the pinned receipt; a
// reusable session grant never substitutes for that confirmation.
func digitalHumanApproval(
	target core.TargetRef,
	risk RiskClassification,
	interactive bool,
	confirmation *RuntimeConfirmationCommand,
	receipt RouteClaimReceipt,
) digitalHumanApprovalDecision {
	if target.Type != core.TargetTypeDigitalHuman {
		return digitalHumanApprovalShared
	}
	// Rule 3: trusted-low runs without confirmation; only high and
	// unclassified risk reach the interactive confirmation gate. A low-risk
	// confirmation and any unknown classification fail closed.
	if risk == RiskTrustedLow {
		if confirmation != nil {
			return digitalHumanApprovalFailClosed
		}
		return digitalHumanApprovalTrustedLow
	}
	if risk != RiskHigh && risk != RiskUnclassified {
		return digitalHumanApprovalFailClosed
	}
	// A confirmation is authority only when it is bound to the pinned receipt.
	if confirmation != nil && confirmation.RouteReceipt != receipt {
		return digitalHumanApprovalFailClosed
	}
	// Rule 2: approval must happen through an interactive channel. Without one
	// the invocation is blocked rather than run, even if a confirmation value is
	// present but cannot be trusted to be live.
	if !interactive {
		return digitalHumanApprovalNonInteractiveBlocked
	}
	return digitalHumanApprovalFreshConfirmation
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
