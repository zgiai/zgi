package integrations

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	OperationReceiptStatusExecuting      = "executing"
	OperationReceiptStatusSucceeded      = "succeeded"
	OperationReceiptStatusOutcomeUnknown = "outcome_unknown"

	operationReceiptLeaseDuration  = 2 * time.Minute
	maxOperationReplayPayloadBytes = 32 * 1024
)

// OperationReceipt is a durable proof that a guarded external side effect was
// attempted or confirmed. It stores only HMAC identities and the provider's
// schema-validated result, never the outbound message content or credentials.
type OperationReceipt struct {
	ID                uuid.UUID              `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID    uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uidx_operation_receipt_org_key" json:"organization_id"`
	WorkspaceID       *uuid.UUID             `gorm:"type:uuid" json:"workspace_id,omitempty"`
	ConversationID    uuid.UUID              `gorm:"type:uuid;not null" json:"conversation_id"`
	MessageID         uuid.UUID              `gorm:"type:uuid;not null" json:"message_id"`
	ConnectionID      uuid.UUID              `gorm:"type:uuid;not null" json:"connection_id"`
	BatchID           string                 `gorm:"size:128;not null" json:"batch_id"`
	OperationItemID   string                 `gorm:"size:128;not null" json:"operation_item_id"`
	ItemIndex         int                    `gorm:"not null" json:"item_index"`
	ItemCount         int                    `gorm:"not null" json:"item_count"`
	IntegrationID     string                 `gorm:"size:64;not null" json:"integration_id"`
	ActionID          string                 `gorm:"size:128;not null" json:"action_id"`
	OperationKey      string                 `gorm:"size:64;not null;uniqueIndex:uidx_operation_receipt_org_key" json:"-"`
	TargetHMAC        string                 `gorm:"size:64;not null" json:"-"`
	FrozenInputHMAC   string                 `gorm:"size:64;not null" json:"-"`
	Status            string                 `gorm:"size:32;not null" json:"status"`
	ClaimToken        uuid.UUID              `gorm:"type:uuid;not null" json:"-"`
	ExecutionID       *uuid.UUID             `gorm:"type:uuid" json:"execution_id,omitempty"`
	ProviderRequestID string                 `gorm:"size:128" json:"provider_request_id,omitempty"`
	ResultPayload     map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"-"`
	ResultCount       int                    `gorm:"not null" json:"result_count"`
	ProviderStartedAt *time.Time             `json:"provider_started_at,omitempty"`
	LeaseExpiresAt    time.Time              `json:"lease_expires_at"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

func (OperationReceipt) TableName() string { return "integration_operation_receipts" }

type OperationReceiptClaim struct {
	Receipt *OperationReceipt
	Claimed bool
}

type OperationReceiptRepository interface {
	Claim(context.Context, *OperationReceipt) (OperationReceiptClaim, error)
	MarkProviderStarted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
	CompleteSuccess(context.Context, uuid.UUID, uuid.UUID, *ActionResult) error
	Release(context.Context, uuid.UUID, uuid.UUID) error
	MarkOutcomeUnknown(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
}

type operationIdentity struct {
	OperationKey    string
	TargetHMAC      string
	FrozenInputHMAC string
}

func deriveOperationIdentity(key []byte, req ActionRequest, resolved ResolvedAction) (operationIdentity, error) {
	guard := resolved.Definition.SuccessDeduplication
	if guard == nil {
		return operationIdentity{}, nil
	}
	if len(key) == 0 {
		return operationIdentity{}, fmt.Errorf("operation receipt HMAC key is unavailable")
	}
	if strings.TrimSpace(req.MessageID) == "" || strings.TrimSpace(req.ConversationID) == "" || strings.TrimSpace(req.ConnectionID) == "" {
		return operationIdentity{}, invalidInput("guarded side effects require message, conversation, and connection context", nil)
	}
	target := make(map[string]interface{}, len(guard.TargetArgumentPaths))
	for _, path := range guard.TargetArgumentPaths {
		value, _ := operationTargetValue(req.Input, path)
		target[path] = value
	}
	targetJSON, err := json.Marshal(target)
	if err != nil {
		return operationIdentity{}, fmt.Errorf("marshal integration operation target: %w", err)
	}
	targetHMAC := hmacHex(key, targetJSON)
	frozenInputJSON, err := json.Marshal(req.Input)
	if err != nil {
		return operationIdentity{}, fmt.Errorf("marshal frozen integration operation input: %w", err)
	}
	frozenInputHMAC := hmacHex(key, frozenInputJSON)
	operationItemID := strings.TrimSpace(req.OperationItemID)
	if operationItemID == "" {
		operationItemID = "single"
	}
	identityJSON, err := json.Marshal([]string{
		"integration-operation-v1",
		strings.TrimSpace(req.OrganizationID),
		strings.TrimSpace(req.MessageID),
		strings.TrimSpace(req.ConnectionID),
		strings.ToLower(strings.TrimSpace(resolved.Definition.ID)),
		targetHMAC,
		operationItemID,
	})
	if err != nil {
		return operationIdentity{}, fmt.Errorf("marshal integration operation identity: %w", err)
	}
	return operationIdentity{OperationKey: hmacHex(key, identityJSON), TargetHMAC: targetHMAC, FrozenInputHMAC: frozenInputHMAC}, nil
}

func operationTargetValue(input map[string]interface{}, path string) (interface{}, bool) {
	var current interface{} = input
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func operationTargetDigest(input map[string]interface{}, guard *SuccessDeduplicationDefinition) string {
	if guard == nil || len(guard.TargetArgumentPaths) == 0 {
		return ""
	}
	target := make(map[string]interface{}, len(guard.TargetArgumentPaths))
	for _, path := range guard.TargetArgumentPaths {
		value, _ := operationTargetValue(input, path)
		target[path] = value
	}
	encoded, err := json.Marshal(target)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:16])
}

func hmacHex(key, value []byte) string {
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write(value)
	return hex.EncodeToString(digest.Sum(nil))
}

func newOperationReceipt(req ActionRequest, resolved ResolvedAction, identity operationIdentity) (*OperationReceipt, error) {
	organizationID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		return nil, err
	}
	conversationID, err := uuid.Parse(req.ConversationID)
	if err != nil {
		return nil, err
	}
	messageID, err := uuid.Parse(req.MessageID)
	if err != nil {
		return nil, err
	}
	connectionID, err := uuid.Parse(req.ConnectionID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	batchID := strings.TrimSpace(req.BatchID)
	operationItemID := strings.TrimSpace(req.OperationItemID)
	itemIndex := req.ItemIndex
	itemCount := req.ItemCount
	if batchID == "" {
		batchID = "single-" + req.MessageID
	}
	if operationItemID == "" {
		operationItemID = "single"
	}
	if itemIndex < 1 {
		itemIndex = 1
	}
	if itemCount < 1 {
		itemCount = 1
	}
	frozenInputHMAC := identity.FrozenInputHMAC
	return &OperationReceipt{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		WorkspaceID:     optionalUUID(req.WorkspaceID),
		ConversationID:  conversationID,
		MessageID:       messageID,
		ConnectionID:    connectionID,
		BatchID:         batchID,
		OperationItemID: operationItemID,
		ItemIndex:       itemIndex,
		ItemCount:       itemCount,
		IntegrationID:   strings.ToLower(strings.TrimSpace(resolved.IntegrationID)),
		ActionID:        strings.ToLower(strings.TrimSpace(resolved.Definition.ID)),
		OperationKey:    identity.OperationKey,
		TargetHMAC:      identity.TargetHMAC,
		FrozenInputHMAC: frozenInputHMAC,
		Status:          OperationReceiptStatusExecuting,
		ClaimToken:      uuid.New(),
		ResultPayload:   map[string]interface{}{},
		LeaseExpiresAt:  now.Add(operationReceiptLeaseDuration),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func replayOperationResult(receipt *OperationReceipt, outputSchema map[string]interface{}) (*ActionResult, error) {
	if receipt == nil || receipt.Status != OperationReceiptStatusSucceeded || receipt.ResultPayload == nil {
		return nil, NewError(ErrorCodeOperationOutcomeUnknown, "external operation result is unavailable", nil)
	}
	encoded, err := json.Marshal(receipt.ResultPayload)
	if err != nil || len(encoded) > maxOperationReplayPayloadBytes {
		return nil, NewError(ErrorCodeOperationOutcomeUnknown, "external operation result could not be replayed safely", err)
	}
	var output map[string]interface{}
	if err := json.Unmarshal(encoded, &output); err != nil {
		return nil, NewError(ErrorCodeOperationOutcomeUnknown, "external operation result could not be replayed safely", err)
	}
	if err := validateReplayOutput(outputSchema, output); err != nil {
		return nil, err
	}
	return &ActionResult{
		Output:            output,
		ProviderRequestID: receipt.ProviderRequestID,
		ResultCount:       receipt.ResultCount,
		Replayed:          true,
	}, nil
}

func validateReplayOutput(schema map[string]interface{}, output map[string]interface{}) error {
	if err := tools.ValidateJSONSchemaValue(schema, output); err != nil {
		return NewError(ErrorCodeOperationOutcomeUnknown, "stored external operation result no longer matches the action contract", err)
	}
	return nil
}

func shouldReleaseOperationClaim(err error) bool {
	switch ErrorCode(err) {
	case ErrorCodeInvalidInput, ErrorCodeSensitiveInput, ErrorCodeQuotaExceeded,
		ErrorCodeAuthInvalid, ErrorCodeBudgetExceeded, ErrorCodeAccessDenied,
		ErrorCodeRateLimited, ErrorCodeProviderRejected, ErrorCodeDisabled,
		ErrorCodeReconnectRequired, ErrorCodeConnectionExpired,
		ErrorCodeInsufficientScope, ErrorCodeActionAuthMethod:
		return true
	default:
		return false
	}
}

// GuardedOperationErrorStatus converts a guarded side-effect failure into a
// provider-neutral receipt outcome. Returning a structured outcome from the
// integrations facade avoids treating a safely failed attempt as a terminal
// tool-protocol failure, while ambiguous outcomes remain explicitly
// non-retryable.
func GuardedOperationErrorStatus(err error) (status string, retrySafe bool, handled bool) {
	if err == nil {
		return "", false, false
	}
	switch ErrorCode(err) {
	case ErrorCodeOperationInProgress:
		return OperationReceiptStatusExecuting, false, true
	case ErrorCodeOperationOutcomeUnknown, ErrorCodeTimeout, ErrorCodeUpstream,
		ErrorCodeResponseInvalid, ErrorCodeAuditFailed:
		return OperationReceiptStatusOutcomeUnknown, false, true
	default:
		if shouldReleaseOperationClaim(err) {
			return "failed_safe", true, true
		}
	}
	return "", false, false
}
