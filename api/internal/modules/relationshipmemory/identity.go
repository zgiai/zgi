package relationshipmemory

import (
	"errors"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

var (
	ErrInvalidUser = errors.New("user must be a stable opaque identifier with at most 128 characters")
)

const apiSubjectNamespace = "external-agent-memory"
const digitalHumanAPISubjectNamespace = "external-digital-human-memory"

func AccountSubject(accountID uuid.UUID) UserSubjectRef {
	return UserSubjectRef{Kind: UserSubjectAccount, ID: accountID}
}

// APIEndUserSubject preserves the deterministic identity used by the legacy
// Agent memory implementation. Raw user input is deliberately not retained.
func APIEndUserSubject(workspaceID, agentID uuid.UUID, user string) (UserSubjectRef, error) {
	normalized := strings.TrimSpace(user)
	if normalized == "" || len([]rune(normalized)) > 128 {
		return UserSubjectRef{}, ErrInvalidUser
	}
	seed := strings.Join([]string{"zgi", apiSubjectNamespace, workspaceID.String(), agentID.String(), normalized}, "\x00")
	return UserSubjectRef{Kind: UserSubjectAPIEndUser, ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed))}, nil
}

// DigitalHumanAPIEndUserSubject derives a stable DigitalHuman-scoped subject
// without retaining the caller's raw user value.
func DigitalHumanAPIEndUserSubject(workspaceID, digitalHumanID uuid.UUID, user string) (UserSubjectRef, error) {
	normalized := strings.TrimSpace(user)
	if normalized == "" || len([]rune(normalized)) > 128 || strings.IndexFunc(normalized, unicode.IsControl) >= 0 {
		return UserSubjectRef{}, ErrInvalidUser
	}
	seed := strings.Join([]string{"zgi", digitalHumanAPISubjectNamespace, workspaceID.String(), digitalHumanID.String(), normalized}, "\x00")
	return UserSubjectRef{Kind: UserSubjectAPIEndUser, ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed))}, nil
}
