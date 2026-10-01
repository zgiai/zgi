package core

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const IdentifierMaxLength = 128

var identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
var hmacSHA256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Identifier is a normalized, stable Connector/plugin/runtime identity.
type Identifier string

func NormalizeIdentifier(value string) (Identifier, error) {
	normalized := Identifier(strings.ToLower(strings.TrimSpace(value)))
	if !normalized.Valid() {
		return "", invalidModel()
	}
	return normalized, nil
}

func (identifier Identifier) String() string { return string(identifier) }

func (identifier Identifier) Valid() bool {
	return identifierPattern.MatchString(string(identifier))
}

type Classification string

const (
	ClassificationConnector  Classification = "connector"
	ClassificationDataSource Classification = "data_source"
)

func (classification Classification) Valid() bool {
	return classification == ClassificationConnector || classification == ClassificationDataSource
}

type DefinitionStatus string

const (
	DefinitionStatusActive      DefinitionStatus = "active"
	DefinitionStatusDisabled    DefinitionStatus = "disabled"
	DefinitionStatusUnavailable DefinitionStatus = "unavailable"
)

func (status DefinitionStatus) Valid() bool {
	switch status {
	case DefinitionStatusActive, DefinitionStatusDisabled, DefinitionStatusUnavailable:
		return true
	default:
		return false
	}
}

type Readiness string

const (
	ReadinessReady             Readiness = "ready"
	ReadinessReconnectRequired Readiness = "reconnect_required"
	ReadinessUnavailable       Readiness = "unavailable"
)

func (readiness Readiness) Valid() bool {
	switch readiness {
	case ReadinessReady, ReadinessReconnectRequired, ReadinessUnavailable:
		return true
	default:
		return false
	}
}

type SetupState string

const (
	SetupStateVerifying SetupState = "verifying"
	SetupStateReady     SetupState = "ready"
	SetupStateFailed    SetupState = "failed"
	SetupStateCancelled SetupState = "cancelled"
)

func (state SetupState) Valid() bool {
	switch state {
	case SetupStateVerifying, SetupStateReady, SetupStateFailed, SetupStateCancelled:
		return true
	default:
		return false
	}
}

type TargetType string

const (
	TargetTypeChat     TargetType = "chat"
	TargetTypeAgent    TargetType = "agent"
	TargetTypeWorkflow TargetType = "workflow"
)

func (targetType TargetType) Valid() bool {
	switch targetType {
	case TargetTypeChat, TargetTypeAgent, TargetTypeWorkflow:
		return true
	default:
		return false
	}
}

type BindingStatus string

const (
	BindingStatusActive           BindingStatus = "active"
	BindingStatusDisabled         BindingStatus = "disabled"
	BindingStatusNeedsReplacement BindingStatus = "needs_replacement"
)

func (status BindingStatus) Valid() bool {
	switch status {
	case BindingStatusActive, BindingStatusDisabled, BindingStatusNeedsReplacement:
		return true
	default:
		return false
	}
}

type DecisionStatus string

const (
	DecisionStatusSubmitted  DecisionStatus = "submitted"
	DecisionStatusDenied     DecisionStatus = "denied"
	DecisionStatusBlocked    DecisionStatus = "blocked"
	DecisionStatusCancelled  DecisionStatus = "cancelled"
	DecisionStatusFailed     DecisionStatus = "failed"
	DecisionStatusSuccessful DecisionStatus = "successful"
	DecisionStatusUnknown    DecisionStatus = "unknown"
)

func (status DecisionStatus) Valid() bool {
	switch status {
	case DecisionStatusSubmitted,
		DecisionStatusDenied,
		DecisionStatusBlocked,
		DecisionStatusCancelled,
		DecisionStatusFailed,
		DecisionStatusSuccessful,
		DecisionStatusUnknown:
		return true
	default:
		return false
	}
}

type OutcomeStatus string

const (
	OutcomeStatusSuccessful OutcomeStatus = "successful"
	OutcomeStatusFailed     OutcomeStatus = "failed"
	OutcomeStatusUnknown    OutcomeStatus = "unknown"
)

func (status OutcomeStatus) Valid() bool {
	return status == OutcomeStatusSuccessful || status == OutcomeStatusFailed || status == OutcomeStatusUnknown
}

type ConnectorDefinition struct {
	ConnectorID       Identifier
	PluginID          Identifier
	PluginVersion     string
	PluginDigest      string
	SourceDialect     string
	SchemaVersion     string
	ProviderID        Identifier
	DriverID          Identifier
	Classification    Classification
	Display           map[string]any
	KeySchema         map[string]any
	CapabilitySummary map[string]any
	ContractDigest    string
	Status            DefinitionStatus
	AcceptedAt        time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (definition ConnectorDefinition) Validate() error {
	if !definition.ConnectorID.Valid() || !definition.PluginID.Valid() ||
		!definition.ProviderID.Valid() || !definition.DriverID.Valid() ||
		!definition.Classification.Valid() || !definition.Status.Valid() ||
		strings.TrimSpace(definition.PluginVersion) == "" ||
		strings.TrimSpace(definition.PluginDigest) == "" ||
		strings.TrimSpace(definition.SourceDialect) == "" ||
		strings.TrimSpace(definition.SchemaVersion) == "" ||
		strings.TrimSpace(definition.ContractDigest) == "" ||
		definition.Display == nil || definition.KeySchema == nil || definition.CapabilitySummary == nil {
		return invalidModel()
	}
	return nil
}

// PersonalConnection is the authoritative owner-only authorization. The
// encrypted envelope is available to persistence/runtime code but is excluded
// from every JSON representation.
type PersonalConnection struct {
	Constraint            *AccessConstraint `json:"-"`
	ID                    uuid.UUID
	OrganizationID        uuid.UUID
	OwnerAccountID        uuid.UUID
	ConnectorID           Identifier
	ProviderID            Identifier
	DriverID              Identifier
	AuthMethodID          Identifier
	Name                  string
	ExternalAccountID     *string
	ExternalDisplayName   *string
	EncryptedCredentials  string `json:"-"`
	CredentialVersion     int
	ProviderEvidence      *ProviderEvidence `json:",omitempty"`
	State                 Readiness
	UsableOperationIDs    []Identifier
	UnavailableOperations map[Identifier]string
	DiagnosticCode        *string
	Revision              int
	LastVerifiedAt        time.Time
	LastProviderSuccessAt *time.Time
	LastProviderFailureAt *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time
}

func (connection PersonalConnection) Validate() error {
	if connection.ID == uuid.Nil || connection.OrganizationID == uuid.Nil || connection.OwnerAccountID == uuid.Nil ||
		!connection.ConnectorID.Valid() || !connection.ProviderID.Valid() || !connection.DriverID.Valid() ||
		!connection.AuthMethodID.Valid() || strings.TrimSpace(connection.Name) == "" ||
		connection.EncryptedCredentials == "" || connection.CredentialVersion < 1 ||
		!connection.State.Valid() || connection.Revision < 1 || connection.LastVerifiedAt.IsZero() ||
		connection.UsableOperationIDs == nil || connection.UnavailableOperations == nil ||
		(connection.State == ReadinessReady && len(connection.UsableOperationIDs) == 0) {
		return invalidModel()
	}
	for _, operationID := range connection.UsableOperationIDs {
		if !operationID.Valid() {
			return invalidModel()
		}
	}
	for operationID := range connection.UnavailableOperations {
		if !operationID.Valid() {
			return invalidModel()
		}
	}
	return nil
}

type TargetRef struct {
	Type              TargetType  `json:"type"`
	ID                string      `json:"id"`
	Scope             TargetScope `json:"scope,omitempty"`
	VersionID         string      `json:"version_id,omitempty"`
	BindingInstanceID string      `json:"binding_instance_id,omitempty"`
}

func (target TargetRef) Validate() error {
	if !target.Type.Valid() || strings.TrimSpace(target.ID) == "" {
		return invalidModel()
	}
	scope := target.EffectiveScope()
	if target.Type == TargetTypeChat && (scope != ScopeLive || target.VersionID != "") {
		return invalidModel()
	}
	if target.Type != TargetTypeChat {
		if scope != ScopeDraft && scope != ScopePublished {
			return invalidModel()
		}
		if scope == ScopePublished {
			id, err := uuid.Parse(target.VersionID)
			if err != nil || id == uuid.Nil || id.String() != target.VersionID {
				return invalidModel()
			}
		} else if target.VersionID != "" {
			return invalidModel()
		}
	}
	instanceID := strings.TrimSpace(target.BindingInstanceID)
	if target.Type == TargetTypeWorkflow {
		if instanceID == "" {
			return invalidModel()
		}
	} else if instanceID != "" {
		return invalidModel()
	}
	return nil
}

type KeySetupAttempt struct {
	ID                 uuid.UUID
	OrganizationID     uuid.UUID
	OwnerAccountID     uuid.UUID
	ConnectorID        Identifier
	ConnectionID       uuid.UUID
	Target             *TargetRef
	IdempotencyKeyHash string `json:"-"`
	// RequestFingerprint is a server-keyed HMAC over the normalized command,
	// Connector, target, submitted field names, and candidate values. It permits
	// conflict detection without persisting a reversible Key hash or raw input.
	RequestFingerprint string `json:"-"`
	State              SetupState
	ErrorCode          ErrorCode
	ResultConnectionID uuid.UUID
	ExpiresAt          time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (attempt KeySetupAttempt) Validate() error {
	if attempt.ID == uuid.Nil || attempt.OrganizationID == uuid.Nil || attempt.OwnerAccountID == uuid.Nil ||
		!attempt.ConnectorID.Valid() || !hmacSHA256HexPattern.MatchString(attempt.IdempotencyKeyHash) ||
		!hmacSHA256HexPattern.MatchString(attempt.RequestFingerprint) ||
		!attempt.State.Valid() || attempt.ExpiresAt.IsZero() {
		return invalidModel()
	}
	if attempt.Target != nil {
		if err := attempt.Target.Validate(); err != nil {
			return err
		}
	}
	if attempt.State == SetupStateReady {
		if attempt.ResultConnectionID == uuid.Nil || attempt.ErrorCode != "" {
			return invalidModel()
		}
	} else if attempt.ResultConnectionID != uuid.Nil {
		return invalidModel()
	}
	if attempt.State == SetupStateFailed {
		if !attempt.ErrorCode.Valid() {
			return invalidModel()
		}
	} else if attempt.ErrorCode != "" {
		return invalidModel()
	}
	return nil
}

type TargetBinding struct {
	Constraint     *AccessConstraint `json:"-"`
	ID             uuid.UUID
	OrganizationID uuid.UUID
	OwnerAccountID uuid.UUID
	ConnectionID   uuid.UUID
	ConnectorID    Identifier
	Target         TargetRef
	Status         BindingStatus

	Revision   int
	DisabledAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (binding TargetBinding) Validate() error {
	if binding.ID == uuid.Nil || binding.OrganizationID == uuid.Nil || binding.OwnerAccountID == uuid.Nil ||
		binding.ConnectionID == uuid.Nil || !binding.ConnectorID.Valid() ||
		!binding.Status.Valid() || binding.Revision < 1 {
		return invalidModel()
	}
	return binding.Target.Validate()
}

type LifecycleDecision struct {
	ID                 uuid.UUID
	OrganizationID     uuid.UUID
	OwnerAccountID     uuid.UUID
	ActingPrincipalID  uuid.UUID
	ConnectorID        Identifier
	ConnectionID       uuid.UUID
	Target             *TargetRef
	OperationID        Identifier
	LogicalOperationID string
	Decision           string
	Status             DecisionStatus
	CorrelationID      string
	ReceiptID          string
	Details            map[string]any
	CreatedAt          time.Time
}

func (decision LifecycleDecision) Validate() error {
	if decision.ID == uuid.Nil || decision.OrganizationID == uuid.Nil || decision.OwnerAccountID == uuid.Nil ||
		decision.ActingPrincipalID == uuid.Nil ||
		(decision.Status != DecisionStatusDenied && decision.ActingPrincipalID != decision.OwnerAccountID) || !decision.ConnectorID.Valid() ||
		strings.TrimSpace(decision.Decision) == "" || !decision.Status.Valid() ||
		strings.TrimSpace(decision.CorrelationID) == "" || decision.Details == nil {
		return invalidModel()
	}
	if decision.Target != nil {
		if err := decision.Target.Validate(); err != nil {
			return err
		}
	}
	if decision.OperationID != "" && !decision.OperationID.Valid() {
		return invalidModel()
	}
	if decision.Status == DecisionStatusSubmitted &&
		(decision.ConnectionID == uuid.Nil || decision.Target == nil || !decision.OperationID.Valid() || strings.TrimSpace(decision.LogicalOperationID) == "") {
		return invalidModel()
	}
	return nil
}

type OperationOutcome struct {
	ID                 uuid.UUID     `json:"id"`
	DecisionID         uuid.UUID     `json:"decision_id"`
	OrganizationID     uuid.UUID     `json:"organization_id"`
	OwnerAccountID     uuid.UUID     `json:"owner_account_id"`
	ActingPrincipalID  uuid.UUID     `json:"acting_principal_id"`
	ConnectorID        Identifier    `json:"connector_id"`
	ConnectionID       uuid.UUID     `json:"connection_id"`
	Target             TargetRef     `json:"target"`
	OperationID        Identifier    `json:"operation_id"`
	LogicalOperationID string        `json:"logical_operation_id"`
	Status             OutcomeStatus `json:"status"`
	ProviderRequestID  string        `json:"provider_request_id,omitempty"`
	DiagnosticCode     string        `json:"diagnostic_code,omitempty"`
	CreatedAt          time.Time     `json:"created_at"`
}

func (outcome OperationOutcome) Validate() error {
	if outcome.ID == uuid.Nil || outcome.DecisionID == uuid.Nil || outcome.OrganizationID == uuid.Nil ||
		outcome.OwnerAccountID == uuid.Nil || outcome.ActingPrincipalID != outcome.OwnerAccountID ||
		!outcome.ConnectorID.Valid() || outcome.ConnectionID == uuid.Nil ||
		!outcome.OperationID.Valid() || strings.TrimSpace(outcome.LogicalOperationID) == "" ||
		!outcome.Status.Valid() {
		return invalidModel()
	}
	return outcome.Target.Validate()
}
