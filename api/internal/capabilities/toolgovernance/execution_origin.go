package toolgovernance

import (
	"errors"

	"github.com/google/uuid"
)

// ExecutionOrigin binds a platform approval to its server-authenticated Linux
// owner and publication. It contains references, never credentials or file data.
type ExecutionOrigin struct {
	OrganizationID  string `json:"organization_id"`
	WorkspaceID     string `json:"workspace_id"`
	ApplicationType string `json:"application_type"`
	ApplicationID   string `json:"application_id"`
	Entry           string `json:"entry"`
	SubjectKind     string `json:"subject_kind"`
	SubjectID       string `json:"subject_id"`
	ConversationID  string `json:"conversation_id"`
	MessageID       string `json:"message_id"`
	PublicationRef  string `json:"publication_ref"`
}

// Validate refuses ambiguous owners and unsupported interactive surfaces.
func (o ExecutionOrigin) Validate() error {
	invalid := errors.New("invalid platform execution origin")
	for _, raw := range []string{o.OrganizationID, o.WorkspaceID, o.ApplicationID, o.SubjectID, o.ConversationID, o.MessageID, o.PublicationRef} {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil || id.String() != raw {
			return invalid
		}
	}
	if o.ApplicationType != "agent" && o.ApplicationType != "digital_human" {
		return invalid
	}
	switch o.Entry {
	case "console", "webapp":
		if o.SubjectKind != "account" {
			return invalid
		}
	case "external-api":
		if o.SubjectKind != "api_end_user" {
			return invalid
		}
	default:
		return invalid
	}
	return nil
}
