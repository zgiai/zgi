package uuid

import (
	"github.com/google/uuid"
)

const (
	// BuiltInWorkflowNamespace is the namespace UUID for built-in workflows
	// Generated once and fixed: uuid.NewSHA1(uuid.NameSpaceURL, []byte("zgi.ai/built-in-workflows"))
	BuiltInWorkflowNamespace = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
)

// GenerateBuiltInWorkflowUUID generates a deterministic UUID v5 for a business scenario
// It uses SHA-1 hashing with a fixed namespace to ensure the same scenario name
// always produces the same UUID across different deployments
func GenerateBuiltInWorkflowUUID(scenario string) uuid.UUID {
	namespace := uuid.MustParse(BuiltInWorkflowNamespace)
	return uuid.NewSHA1(namespace, []byte(scenario))
}

// VerifyBuiltInWorkflowUUID checks if a UUID matches a business scenario
// Returns true if the provided UUID was generated from the given scenario name
func VerifyBuiltInWorkflowUUID(id uuid.UUID, scenario string) bool {
	expected := GenerateBuiltInWorkflowUUID(scenario)
	return id == expected
}
