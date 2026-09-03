package execution

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// ExecutionState is the transport-neutral state returned to chat, agent, and
// workflow runtimes. It is a runtime result, never a configurable policy.
type ExecutionState string

const (
	ExecutionStateRequiresConfirmation ExecutionState = "requires_confirmation"
	ExecutionStateCancelled            ExecutionState = "cancelled"
	ExecutionStateBlocked              ExecutionState = "blocked"
	ExecutionStateCompleted            ExecutionState = "completed"
)

// ConfirmationDecision is the only user choice accepted by the runtime
// confirmation continuation. It cannot change risk or future behavior.
type ConfirmationDecision string

const (
	ConfirmationDecisionConfirmed ConfirmationDecision = "confirmed"
	ConfirmationDecisionCancelled ConfirmationDecision = "cancelled"
)

// RuntimeConfirmationCommand resumes the already pinned logical operation.
// RouteReceipt is an opaque server-issued capability and contains no input or
// credential material.
type RuntimeConfirmationCommand struct {
	RouteReceipt RouteClaimReceipt    `json:"route_receipt"`
	Decision     ConfirmationDecision `json:"decision"`
}

func (command RuntimeConfirmationCommand) valid() bool {
	return validReceipt(command.RouteReceipt) &&
		(command.Decision == ConfirmationDecisionConfirmed || command.Decision == ConfirmationDecisionCancelled)
}

// RuntimeConsequence is structured so every host surface can render localized
// copy without receiving the opaque operation input.
type RuntimeConsequence struct {
	ExternalEffect      string                `json:"external_effect"`
	Description         string                `json:"description,omitempty"`
	DescriptionI18n     catalog.LocalizedText `json:"description_i18n,omitempty"`
	DataEgress          bool                  `json:"data_egress"`
	ExternalDestination string                `json:"external_destination,omitempty"`
}

// RuntimeConfirmation contains exactly the Connector, action, target, and
// consequence required for informed consent. It deliberately contains no
// member policy, credential, or operation input.
type RuntimeConfirmation struct {
	RouteReceipt      RouteClaimReceipt     `json:"route_receipt"`
	ConnectorID       core.Identifier       `json:"connector_id"`
	ConnectorName     string                `json:"connector_name"`
	ConnectorNameI18n catalog.LocalizedText `json:"connector_name_i18n,omitempty"`
	OperationID       core.Identifier       `json:"operation_id"`
	ActionName        string                `json:"action_name"`
	ActionNameI18n    catalog.LocalizedText `json:"action_name_i18n,omitempty"`
	Target            core.TargetRef        `json:"target"`
	Consequence       RuntimeConsequence    `json:"consequence"`
	Risk              RiskClassification    `json:"risk_classification"`
	// AcceptedOperation is in-process authority used to build the encrypted
	// continuation. The public confirmation envelope deliberately omits it.
	AcceptedOperation *catalog.PinnedOperation `json:"-"`
}

func runtimeConfirmation(
	receipt RouteClaimReceipt,
	accepted catalog.PinnedOperation,
	target core.TargetRef,
	risk RiskClassification,
) *RuntimeConfirmation {
	definition := accepted.Definition
	operation := accepted.Operation
	externalEffect := string(operation.Effect)
	if externalEffect == "" {
		externalEffect = "unspecified"
	}
	return &RuntimeConfirmation{
		RouteReceipt: receipt, ConnectorID: core.Identifier(definition.ID), ConnectorName: definition.Name,
		ConnectorNameI18n: definition.NameI18n.Clone(), ActionNameI18n: operation.NameI18n.Clone(),
		OperationID: core.Identifier(operation.ID), ActionName: operation.Name, Target: target,
		Consequence: RuntimeConsequence{
			ExternalEffect: externalEffect, Description: strings.TrimSpace(operation.Description), DataEgress: operation.DataEgress,
			DescriptionI18n:     operation.DescriptionI18n.Clone(),
			ExternalDestination: operation.ExternalDestination,
		},
		Risk: risk, AcceptedOperation: &accepted,
	}
}
