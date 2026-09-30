package core

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/google/uuid"
)

// RegistrationProof is an in-process trust assertion created after an official
// QR response. It is not a DTO and cannot be provided by a browser or model.
type RegistrationProof struct {
	OrganizationID, OwnerAccountID, FlowID uuid.UUID
	ConnectorID, Identity                  string
	Values                                 map[string]string `json:"-"`
	digest                                 [32]byte
}
type registrationProofKey struct{}

func WithRegistrationProof(ctx context.Context, p RegistrationProof) context.Context {
	raw, _ := json.Marshal(p.Values)
	p.digest = sha256.Sum256(raw)
	clear(raw)
	p.Values = nil
	return context.WithValue(ctx, registrationProofKey{}, p)
}
func VerifiedRegistration(ctx context.Context, org, owner uuid.UUID, connector, identity string, values map[string]string) (uuid.UUID, bool) {
	p, ok := ctx.Value(registrationProofKey{}).(RegistrationProof)
	raw, _ := json.Marshal(values)
	digest := sha256.Sum256(raw)
	clear(raw)
	return p.FlowID, ok && p.FlowID != uuid.Nil && p.OrganizationID == org && p.OwnerAccountID == owner && p.ConnectorID == connector && p.Identity == identity && p.digest == digest
}
