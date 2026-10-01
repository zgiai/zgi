package credentials

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

func TestContinuationPreviewVersionCompatibility(t *testing.T) {
	cipher, err := NewStore("native", map[string]string{"native": strings.Repeat("x", 32)})
	if err != nil {
		t.Fatal(err)
	}
	protector, err := NewContinuationPayloadProtector(cipher)
	if err != nil {
		t.Fatal(err)
	}
	org := uuid.New()
	receipt := execution.RouteClaimReceipt(uuid.NewString())
	frozen := execution.FrozenConfirmationRequest{RouteReceipt: receipt, Request: execution.ExecutionRequest{OrganizationID: org, Input: map[string]any{"text": "PRIVATE-BODY"}}, Preview: &execution.ConfirmationPreview{AccountIdentity: "PRIVATE-ACCOUNT", Kind: "mail", Fields: []execution.ConfirmationPreviewField{}, Complete: true}}
	v3, err := protector.Seal(t.Context(), org, receipt, frozen)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(v3), "v3:") {
		t.Fatal("continuations must always include the frozen preview")
	}
	// Keep a v2 envelope fixture to prove old pending confirmations remain readable.
	aad, err := connectorContinuationCredentialAAD(org, receipt)
	if err != nil {
		t.Fatal(err)
	}
	legacyPayload, err := marshalConnectorContinuationPayload(frozen)
	if err != nil {
		t.Fatal(err)
	}
	legacyCiphertext, err := cipher.Protect(aad, legacyPayload)
	if err != nil {
		t.Fatal(err)
	}
	v2 := execution.ContinuationPayloadEnvelope(legacyCiphertext)
	old, err := protector.Open(t.Context(), org, receipt, v2)
	if err != nil || old.Preview != nil {
		t.Fatal("old v2 reader contract", err)
	}
	new, err := protector.Open(t.Context(), org, receipt, v3)
	if err != nil || new.Preview.AccountIdentity != "PRIVATE-ACCOUNT" {
		t.Fatal("v3 preview was not restored", err)
	}
	for _, bad := range []string{"v3:" + string(v2), strings.TrimPrefix(string(v3), "v3:"), "v4:" + string(v3)} {
		if _, err := protector.Open(t.Context(), org, receipt, execution.ContinuationPayloadEnvelope(bad)); err == nil {
			t.Fatal("version/AAD tampering accepted")
		}
	}
	if _, err := protector.Open(t.Context(), uuid.New(), receipt, v3); err == nil {
		t.Fatal("wrong organization accepted")
	}
	if _, err := protector.Open(t.Context(), org, execution.RouteClaimReceipt(uuid.NewString()), v3); err == nil {
		t.Fatal("wrong receipt accepted")
	}
	for _, envelope := range []execution.ContinuationPayloadEnvelope{v2, v3} {
		if strings.Contains(string(envelope), "PRIVATE") {
			t.Fatal("plaintext stored")
		}
	}
	raw, _ := marshalConnectorContinuationPayload(frozen)
	var value map[string]any
	_ = json.Unmarshal(raw, &value)
	value["preview"] = frozen.Preview
	modified, _ := json.Marshal(value)
	if _, err := unmarshalConnectorContinuationPayload(modified); err == nil {
		t.Fatal("strict v2 decoder accepted a preview")
	}
}
