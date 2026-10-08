// Package agentexecution defines the server-owned identity for Linux execution.
// It does not authenticate callers or select a billing account.
package agentexecution

import (
	"errors"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

const IdentityVersion = 1
const IdentityHeader = "X-ZGI-Execution-Identity"

var ErrInvalidIdentity = errors.New("verified execution identity is required")

// Identity matches the versioned Worker contract. It is internal transport
// data, not an Agent configuration field or a client-provided authentication.
type Identity struct {
	Version         int    `json:"version"`
	WorkspaceID     string `json:"workspace_id"`
	ApplicationType string `json:"application_type"`
	ApplicationID   string `json:"application_id"`
	Entry           string `json:"entry"`
	SubjectKind     string `json:"subject_kind"`
	ConversationID  string `json:"conversation_id"`
	PublicationRef  string `json:"publication_ref"`
}

type Scope struct {
	OrganizationID string
	SubjectID      string
	Identity       *Identity
}

// VerifiedSubject must come from the entry's existing authenticated subject
// resolver. In particular, API credentials and resource owners are not users.
type VerifiedSubject struct {
	OrganizationID  uuid.UUID
	WorkspaceID     uuid.UUID
	ApplicationType string
	ApplicationID   uuid.UUID
	Entry           string
	User            usersubject.Ref
	ConversationID  uuid.UUID
	PublicationRef  string
}

func NewScope(subject VerifiedSubject) (Scope, error) {
	if subject.OrganizationID == uuid.Nil || subject.WorkspaceID == uuid.Nil ||
		subject.ApplicationID == uuid.Nil || subject.User.ID == uuid.Nil || subject.ConversationID == uuid.Nil {
		return Scope{}, ErrInvalidIdentity
	}
	identity := &Identity{
		Version: IdentityVersion, WorkspaceID: subject.WorkspaceID.String(),
		ApplicationType: subject.ApplicationType, ApplicationID: subject.ApplicationID.String(),
		Entry: subject.Entry, SubjectKind: string(subject.User.Kind),
		ConversationID: subject.ConversationID.String(), PublicationRef: subject.PublicationRef,
	}
	if err := identity.Validate(); err != nil {
		return Scope{}, err
	}
	return Scope{OrganizationID: subject.OrganizationID.String(), SubjectID: subject.User.ID.String(), Identity: identity}, nil
}

func (identity Identity) Validate() error {
	if identity.Version != IdentityVersion || (identity.ApplicationType != "agent" && identity.ApplicationType != "digital_human") {
		return ErrInvalidIdentity
	}
	for _, value := range []string{identity.WorkspaceID, identity.ApplicationID, identity.ConversationID, identity.PublicationRef} {
		if value == "" || len(value) > 256 || strings.TrimSpace(value) != value || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return ErrInvalidIdentity
		}
	}
	switch identity.Entry {
	case "console", "webapp":
		if identity.SubjectKind != string(usersubject.Account) {
			return ErrInvalidIdentity
		}
	case "external-api":
		if identity.SubjectKind != string(usersubject.APIEndUser) {
			return ErrInvalidIdentity
		}
	default:
		return ErrInvalidIdentity
	}
	return nil
}
