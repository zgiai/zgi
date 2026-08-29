package wecom

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
)

const maxMessageRecipients = 100

type sendMessageResponse struct {
	apiEnvelope
	MsgID          string `json:"msgid"`
	InvalidUser    string `json:"invaliduser"`
	InvalidParty   string `json:"invalidparty"`
	InvalidTag     string `json:"invalidtag"`
	UnlicensedUser string `json:"unlicenseduser"`
}

type resolvedMessageRecipient struct {
	reference string
	userID    string
}

func (adapter *Adapter) sendUser(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, error) {
	references, err := messageRecipientReferences(input)
	if err != nil {
		return nil, 0, err
	}
	recipients := make([]resolvedMessageRecipient, 0, len(references))
	seenUserIDs := make(map[string]struct{}, len(references))
	for _, reference := range references {
		claims, decodeErr := adapter.decodeSubjectRef(reference, creds, SubjectTypeMember, SubjectIDKindUserID)
		if decodeErr != nil {
			return nil, 0, wecomError(integrations.ErrorCodeInvalidInput, "WeCom recipient reference is invalid or no longer bound to this connection", decodeErr)
		}
		key := strings.ToLower(claims.SubjectID)
		if _, exists := seenUserIDs[key]; exists {
			return nil, 0, integrations.NewErrorWithReason(
				integrations.ErrorCodeInvalidInput,
				"wecom_duplicate_message_recipient",
				"WeCom message recipients must be unique",
				nil,
			)
		}
		seenUserIDs[key] = struct{}{}
		recipients = append(recipients, resolvedMessageRecipient{reference: reference, userID: claims.SubjectID})
	}

	content := inputString(input, "content")
	if strings.TrimSpace(content) == "" || !utf8.ValidString(content) || len([]byte(content)) > 2048 {
		return nil, 0, integrations.NewErrorWithReason(
			integrations.ErrorCodeInvalidInput,
			"wecom_message_content_invalid",
			"WeCom message content must contain between 1 and 2048 UTF-8 bytes",
			nil,
		)
	}
	agentID, err := strconv.ParseInt(creds.AgentID, 10, 64)
	if err != nil {
		return nil, 0, wecomError(integrations.ErrorCodeConnectionInvalid, "WeCom Agent ID is invalid", err)
	}
	userIDs := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		userIDs = append(userIDs, recipient.userID)
	}
	safe := 0
	if value, ok := input["safe"].(bool); ok && value {
		safe = 1
	}
	payload := map[string]interface{}{
		"touser":                   strings.Join(userIDs, "|"),
		"msgtype":                  "text",
		"agentid":                  agentID,
		"text":                     map[string]string{"content": content},
		"safe":                     safe,
		"enable_duplicate_check":   1,
		"duplicate_check_interval": 1800,
	}
	response := sendMessageResponse{}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodPost, "/cgi-bin/message/send", nil, payload, &response); err != nil {
		return nil, 0, err
	}
	messageID := bounded(response.MsgID, 255)
	if messageID == "" {
		return nil, 0, integrations.NewErrorWithReason(
			integrations.ErrorCodeResponseInvalid,
			"wecom_message_acceptance_unconfirmed",
			"WeCom returned success without a message ID",
			nil,
		)
	}

	referenceByUserID := make(map[string]string, len(recipients))
	for _, recipient := range recipients {
		referenceByUserID[strings.ToLower(recipient.userID)] = recipient.reference
	}
	invalidReferences, invalidSet, err := reconcileRejectedUserIDs(response.InvalidUser, referenceByUserID)
	if err != nil {
		return nil, 0, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an unknown invalid recipient", err)
	}
	unlicensedReferences, unlicensedSet, err := reconcileRejectedUserIDs(response.UnlicensedUser, referenceByUserID)
	if err != nil {
		return nil, 0, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom returned an unknown unlicensed recipient", err)
	}
	rejected := make(map[string]struct{}, len(invalidSet)+len(unlicensedSet))
	for userID := range invalidSet {
		rejected[userID] = struct{}{}
	}
	for userID := range unlicensedSet {
		rejected[userID] = struct{}{}
	}
	acceptedCount := len(recipients) - len(rejected)
	if acceptedCount <= 0 {
		// errcode=0 with a message ID but no accepted target contradicts the
		// provider contract. Treat this as an unknown outcome rather than a
		// retry-safe rejection, because the provider claimed acceptance.
		return nil, 0, integrations.NewErrorWithReason(
			integrations.ErrorCodeResponseInvalid,
			"wecom_message_recipient_outcome_inconsistent",
			"WeCom message response could not be reconciled with the intended recipients",
			nil,
		)
	}
	invalidPartyCount := len(splitProviderResultList(response.InvalidParty))
	invalidTagCount := len(splitProviderResultList(response.InvalidTag))
	operationStatus := "succeeded"
	if len(rejected) > 0 || invalidPartyCount > 0 || invalidTagCount > 0 {
		operationStatus = "partially_succeeded"
	}
	return map[string]interface{}{
		"provider":         IntegrationID,
		"operation_status": operationStatus,
		"message": map[string]interface{}{
			"message_id":            messageID,
			"recipient_refs":        references,
			"provider_accepted":     true,
			"requested_count":       len(recipients),
			"accepted_count":        acceptedCount,
			"rejected_count":        len(rejected),
			"invalid_recipients":    invalidReferences,
			"unlicensed_recipients": unlicensedReferences,
			"invalid_party_count":   invalidPartyCount,
			"invalid_tag_count":     invalidTagCount,
			"delivery_status":       "accepted_unknown_delivery",
			"operation_status":      operationStatus,
		},
	}, acceptedCount, nil
}

func messageRecipientReferences(input map[string]interface{}) ([]string, error) {
	single := strings.TrimSpace(inputString(input, "recipient_ref"))
	plural := inputStringList(input, "recipient_refs")
	if (single == "") == (len(plural) == 0) {
		return nil, integrations.NewErrorWithReason(
			integrations.ErrorCodeInvalidInput,
			"wecom_message_recipient_selection_invalid",
			"Provide exactly one of recipient_ref or recipient_refs",
			nil,
		)
	}
	if single != "" {
		return []string{single}, nil
	}
	if len(plural) > maxMessageRecipients {
		return nil, wecomError(integrations.ErrorCodeInvalidInput, "WeCom message has too many recipients", nil)
	}
	for index := range plural {
		plural[index] = strings.TrimSpace(plural[index])
		if plural[index] == "" || len(plural[index]) > 4096 {
			return nil, wecomError(integrations.ErrorCodeInvalidInput, "WeCom recipient reference is invalid", nil)
		}
	}
	return plural, nil
}

func inputStringList(input map[string]interface{}, key string) []string {
	switch values := input[key].(type) {
	case []string:
		return append([]string(nil), values...)
	case []interface{}:
		out := make([]string, 0, len(values))
		for _, raw := range values {
			value, ok := raw.(string)
			if !ok {
				return nil
			}
			out = append(out, value)
		}
		return out
	default:
		return nil
	}
}

func reconcileRejectedUserIDs(raw string, referenceByUserID map[string]string) ([]string, map[string]struct{}, error) {
	providerUserIDs := splitProviderResultList(raw)
	references := make([]string, 0, len(providerUserIDs))
	seen := make(map[string]struct{}, len(providerUserIDs))
	for _, providerUserID := range providerUserIDs {
		key := strings.ToLower(providerUserID)
		if _, exists := seen[key]; exists {
			continue
		}
		reference, exists := referenceByUserID[key]
		if !exists {
			return nil, nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "provider recipient is not part of the request", nil)
		}
		seen[key] = struct{}{}
		references = append(references, reference)
	}
	return references, seen, nil
}

func splitProviderResultList(raw string) []string {
	parts := strings.Split(strings.TrimSpace(raw), "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
