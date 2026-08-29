package integrations

import (
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

type IntegrationApprovalPolicy string

const (
	IntegrationApprovalPolicyInherit   IntegrationApprovalPolicy = "inherit"
	IntegrationApprovalPolicyAlwaysAsk IntegrationApprovalPolicy = "always_ask"
)

type IntegrationActionPolicy struct {
	OrganizationID    uuid.UUID                 `gorm:"type:uuid;primaryKey" json:"organization_id"`
	IntegrationID     string                    `gorm:"size:64;primaryKey" json:"integration_id"`
	ActionID          string                    `gorm:"size:128;primaryKey" json:"action_id"`
	Enabled           bool                      `gorm:"not null" json:"enabled"`
	ApprovalPolicy    IntegrationApprovalPolicy `gorm:"size:32;not null" json:"approval_policy"`
	DataEgressAllowed bool                      `gorm:"not null" json:"data_egress_allowed"`
	UpdatedBy         *uuid.UUID                `gorm:"type:uuid" json:"updated_by,omitempty"`
	CreatedAt         time.Time                 `json:"created_at"`
	UpdatedAt         time.Time                 `json:"updated_at"`
}

func (IntegrationActionPolicy) TableName() string { return "integration_action_policies" }

type ActionPolicyDecision struct {
	Enabled           bool
	ApprovalPolicy    IntegrationApprovalPolicy
	DataEgressAllowed bool
}

type ActionPolicyView struct {
	IntegrationID       string                    `json:"integration_id"`
	ActionID            string                    `json:"action_id"`
	Name                string                    `json:"name"`
	Effect              toolgovernance.Effect     `json:"effect"`
	RiskLevel           toolgovernance.RiskLevel  `json:"risk_level"`
	DataEgress          bool                      `json:"data_egress"`
	ExternalDestination string                    `json:"external_destination,omitempty"`
	Enabled             bool                      `json:"enabled"`
	ApprovalPolicy      IntegrationApprovalPolicy `json:"approval_policy"`
	DataEgressAllowed   bool                      `json:"data_egress_allowed"`
}

type ActionPolicySetView struct {
	Revision string             `json:"revision"`
	Items    []ActionPolicyView `json:"items"`
}

type ActionPolicyInput struct {
	ActionID          string                    `json:"action_id"`
	Enabled           bool                      `json:"enabled"`
	ApprovalPolicy    IntegrationApprovalPolicy `json:"approval_policy"`
	DataEgressAllowed bool                      `json:"data_egress_allowed"`
}
