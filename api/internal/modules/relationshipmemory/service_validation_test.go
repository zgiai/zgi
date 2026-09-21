package relationshipmemory

import (
	"context"
	"errors"
	"testing"
)

func TestValidateMutationRequiresEvidenceForModelAndAutomaticWrites(t *testing.T) {
	contract := ConsumerMemoryContract{
		Enabled:        true,
		DefinitionMode: "dynamic",
		AllowedCategories: []string{
			"profile", "preference", "constraint", "relationship",
		},
	}
	base := PutItemRequest{Key: "answer_style", Category: "preference", Content: "Prefer concise answers.", Contract: contract}

	automatic := base
	automatic.Metadata.SourceKind = SourceAutomatic
	if err := validateMutation(automatic); !errors.Is(err, ErrPolicyRejected) {
		t.Fatalf("automatic write without evidence error = %v", err)
	}

	model := base
	model.Metadata.ActorType = "model"
	if err := validateMutation(model); !errors.Is(err, ErrPolicyRejected) {
		t.Fatalf("model write without evidence error = %v", err)
	}

	model.Metadata.EvidenceFromUserInput = true
	if err := validateMutation(model); err != nil {
		t.Fatalf("evidence-backed model write rejected: %v", err)
	}
}

func TestPlainContentCodecFailsClosedOnUnknownEncoding(t *testing.T) {
	codec := PlainContentCodec{}
	if _, err := codec.Decode(context.Background(), EncodedContent{Payload: "ciphertext", Encoding: "aes-gcm"}); err == nil {
		t.Fatal("plain codec accepted an encrypted payload")
	}
}

func TestValidateMutationEnforcesConsumerDefinitions(t *testing.T) {
	agent := PutItemRequest{
		Key: "unknown", Content: "value",
		Contract: ConsumerMemoryContract{Enabled: true, DefinitionMode: "slots", Definitions: []MemoryDefinition{{Key: "profile", Enabled: true, MaxChars: 20}}},
	}
	if err := validateMutation(agent); !errors.Is(err, ErrPolicyRejected) {
		t.Fatalf("unknown Agent slot error = %v", err)
	}

	digitalHuman := PutItemRequest{
		Key: "project", Category: "project", Content: "Project details",
		Contract: ConsumerMemoryContract{Enabled: true, DefinitionMode: "dynamic", AllowedCategories: []string{"profile", "preference", "constraint", "relationship"}},
	}
	if err := validateMutation(digitalHuman); !errors.Is(err, ErrPolicyRejected) {
		t.Fatalf("business-scoped DigitalHuman category error = %v", err)
	}
}
