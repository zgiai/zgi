package usersubject

import (
	"errors"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

var ErrInvalidUser = errors.New("user must be a stable opaque identifier with at most 128 characters")

const (
	agentAPISubjectNamespace        = "external-agent-memory"
	digitalHumanAPISubjectNamespace = "external-digital-human-memory"
)

// AccountSubject returns the stable subject for an authenticated account.
func AccountSubject(accountID uuid.UUID) Ref {
	return Ref{Kind: Account, ID: accountID}
}

// AgentAPIEndUserSubject preserves the deterministic identity used by the
// Agent API while refusing to retain the caller's raw user value.
func AgentAPIEndUserSubject(workspaceID, agentID uuid.UUID, user string) (Ref, error) {
	return apiEndUserSubject(agentAPISubjectNamespace, workspaceID, agentID, user)
}

// DigitalHumanAPIEndUserSubject derives a stable DigitalHuman-scoped subject.
func DigitalHumanAPIEndUserSubject(workspaceID, digitalHumanID uuid.UUID, user string) (Ref, error) {
	return apiEndUserSubject(digitalHumanAPISubjectNamespace, workspaceID, digitalHumanID, user)
}

func apiEndUserSubject(namespace string, workspaceID, consumerID uuid.UUID, user string) (Ref, error) {
	normalized := strings.TrimSpace(user)
	if workspaceID == uuid.Nil || consumerID == uuid.Nil || normalized == "" || len([]rune(normalized)) > 128 || strings.IndexFunc(normalized, unicode.IsControl) >= 0 {
		return Ref{}, ErrInvalidUser
	}
	seed := strings.Join([]string{"zgi", namespace, workspaceID.String(), consumerID.String(), normalized}, "\x00")
	return Ref{Kind: APIEndUser, ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed))}, nil
}
