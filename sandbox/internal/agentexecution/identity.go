package agentexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// ExecutionIdentityVersion is negotiated separately from the legacy release
// protocol. Workers without this capability must not receive new entry points.
const ExecutionIdentityVersion = 1
const ExecutionIdentityHeader = "X-ZGI-Execution-Identity"

// ExecutionIdentity is supplied only by the authenticated API, never by a Job
// or an end user. SubjectID and OrganizationID remain in Scope. Nil denotes the
// historical console Agent contract; it cannot access identity-scoped bindings.
type ExecutionIdentity struct {
	Version         int    `json:"version"`
	WorkspaceID     string `json:"workspace_id"`
	ApplicationType string `json:"application_type"`
	ApplicationID   string `json:"application_id"`
	Entry           string `json:"entry"`
	SubjectKind     string `json:"subject_kind"`
	ConversationID  string `json:"conversation_id"`
	PublicationRef  string `json:"publication_ref"`
}

func (identity ExecutionIdentity) Validate() error {
	if identity.Version != ExecutionIdentityVersion {
		return fmt.Errorf("%w: unsupported execution identity version", ErrInvalidRequest)
	}
	for _, value := range []string{identity.WorkspaceID, identity.ApplicationID, identity.ConversationID, identity.PublicationRef} {
		if value == "" || len(value) > 256 || strings.TrimSpace(value) != value || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return fmt.Errorf("%w: incomplete execution identity", ErrInvalidRequest)
		}
	}
	if identity.ApplicationType != "agent" && identity.ApplicationType != "digital_human" {
		return fmt.Errorf("%w: unsupported execution application", ErrInvalidRequest)
	}
	switch identity.Entry {
	case "console", "webapp":
		if identity.SubjectKind != "account" {
			return fmt.Errorf("%w: authenticated account is required", ErrInvalidRequest)
		}
	case "external-api":
		if identity.SubjectKind != "api_end_user" {
			return fmt.Errorf("%w: API end user is required", ErrInvalidRequest)
		}
	default:
		return fmt.Errorf("%w: unsupported execution entry", ErrInvalidRequest)
	}
	return nil
}

func cloneExecutionIdentity(identity *ExecutionIdentity) *ExecutionIdentity {
	if identity == nil {
		return nil
	}
	copy := *identity
	return &copy
}

func sameExecutionIdentity(left, right *ExecutionIdentity) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

// DecodeExecutionIdentity rejects unknown fields, unsupported versions and
// trailing data rather than silently interpreting a future identity as legacy.
func DecodeExecutionIdentity(value string) (*ExecutionIdentity, error) {
	if len(value) > 4096 {
		return nil, ErrInvalidRequest
	}
	var identity ExecutionIdentity
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&identity); err != nil {
		return nil, fmt.Errorf("%w: invalid execution identity", ErrInvalidRequest)
	}
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	// json.Valid also rejects a second JSON value after the object.
	if !json.Valid([]byte(value)) {
		return nil, ErrInvalidRequest
	}
	return &identity, nil
}
