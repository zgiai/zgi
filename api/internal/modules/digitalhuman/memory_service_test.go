package digitalhuman

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
)

func TestValidateMemoryProposalPolicyRejectsUnsafeAutonomousWrites(t *testing.T) {
	t.Parallel()
	base := MemoryProposalRequest{
		MemoryClass: memoryruntime.MemoryClassSelf,
		MemoryType:  memoryruntime.MemoryTypeExperience,
		Content:     "Prefer concise, evidence-backed summaries.",
		Visibility:  MemoryVisibilityWorkspace,
		Confidence:  0.8,
	}
	if err := validateMemoryProposalPolicy(base, true); err != nil {
		t.Fatalf("safe autonomous proposal error = %v", err)
	}
	// Convergence: self is the only class, so an autonomous evidence-backed fact
	// is no longer gated behind a business scope and must be accepted.
	fact := base
	fact.MemoryType = memoryruntime.MemoryTypeFact
	if err := validateMemoryProposalPolicy(fact, true); err != nil {
		t.Fatalf("autonomous self fact error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*MemoryProposalRequest)
	}{
		{name: "identity", mutate: func(value *MemoryProposalRequest) {
			value.MemoryType = memoryruntime.MemoryTypeIdentity
			value.AdminApproved = true
		}},
		{name: "private", mutate: func(value *MemoryProposalRequest) { value.SourceContext.PrivateConversation = true }},
		{name: "credential", mutate: func(value *MemoryProposalRequest) { value.Content = "api_key=secret-value" }},
		{name: "business object", mutate: func(value *MemoryProposalRequest) { value.Content = `{"customer":{"email":"a@example.com"}}` }},
		{name: "oversize", mutate: func(value *MemoryProposalRequest) { value.Content = strings.Repeat("a", maxMemoryContentBytes+1) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.mutate(&value)
			if err := validateMemoryProposalPolicy(value, true); !errors.Is(err, ErrMemoryPolicyRejected) {
				t.Fatalf("error = %v, want policy rejection", err)
			}
		})
	}
}

func TestValidateMemoryProposalPolicyRequiresExplicitIdentityApproval(t *testing.T) {
	t.Parallel()
	request := MemoryProposalRequest{
		MemoryClass: memoryruntime.MemoryClassSelf,
		MemoryType:  memoryruntime.MemoryTypeIdentity,
		Content:     "I am responsible for growth research.",
		Visibility:  MemoryVisibilityWorkspace,
		Confidence:  1,
	}
	if err := validateMemoryProposalPolicy(request, false); !errors.Is(err, ErrMemoryPolicyRejected) {
		t.Fatalf("unapproved identity error = %v", err)
	}
	request.AdminApproved = true
	if err := validateMemoryProposalPolicy(request, false); err != nil {
		t.Fatalf("approved identity error = %v", err)
	}
}

func TestMemoryDigestIsStableAndDoesNotExposeContent(t *testing.T) {
	t.Parallel()
	digest := memoryContentDigest("durable fact")
	if !strings.HasPrefix(digest, "sha256:") || strings.Contains(digest, "durable fact") || len(digest) != 71 {
		t.Fatalf("digest = %q", digest)
	}
}

func TestValidateMemorySupersedeContract(t *testing.T) {
	t.Parallel()
	targetID := uuid.New()
	valid := MemoryProposalRequest{
		Action: memoryruntime.MutationActionUpsert, ExpectedRevision: 0,
		SupersedesID: &targetID, ExpectedSupersededRevision: 3,
	}
	if err := validateMemorySupersedeContract(valid); err != nil {
		t.Fatalf("valid supersede error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*MemoryProposalRequest)
	}{
		{name: "nil target", mutate: func(value *MemoryProposalRequest) {
			nilID := uuid.Nil
			value.SupersedesID = &nilID
		}},
		{name: "clear", mutate: func(value *MemoryProposalRequest) { value.Action = memoryruntime.MutationActionClear }},
		{name: "existing replacement", mutate: func(value *MemoryProposalRequest) { value.ExpectedRevision = 1 }},
		{name: "missing target revision", mutate: func(value *MemoryProposalRequest) { value.ExpectedSupersededRevision = 0 }},
		{name: "revision without target", mutate: func(value *MemoryProposalRequest) { value.SupersedesID = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.mutate(&request)
			if err := validateMemorySupersedeContract(request); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("error = %v, want invalid request", err)
			}
		})
	}
}
