package agentbindings

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSkillSuspensionImpactTokenBindsAllDisabledSkills(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	req, err := normalizeSkillSuspensionImpactRequest(SkillSuspensionImpactRequest{
		OrganizationID: uuid.New(),
		ActorID:        uuid.New(),
		SkillIDs:       []string{"Custom-B", "custom-a", "custom-a"},
	})
	if err != nil {
		t.Fatalf("normalizeSkillSuspensionImpactRequest() error = %v", err)
	}
	if len(req.SkillIDs) != 2 || req.SkillIDs[0] != "custom-a" || req.SkillIDs[1] != "custom-b" {
		t.Fatalf("normalized skill ids = %#v", req.SkillIDs)
	}
	payload := skillSuspensionImpactPayload(req, []Binding{{
		AgentID:      uuid.New(),
		BindingScope: ScopeDraft,
		BindingType:  BindingTypeSkill,
		ResourceID:   "custom-a",
	}}, now.Add(ImpactTokenTTL))
	token, err := encodeSkillSuspensionImpactToken([]byte("shared-secret"), payload)
	if err != nil {
		t.Fatalf("encodeSkillSuspensionImpactToken() error = %v", err)
	}
	decoded, err := decodeSkillSuspensionImpactToken([]byte("shared-secret"), token)
	if err != nil {
		t.Fatalf("decodeSkillSuspensionImpactToken() error = %v", err)
	}
	if !skillSuspensionImpactPayloadEqual(decoded, payload) {
		t.Fatalf("decoded payload = %#v, want %#v", decoded, payload)
	}
	changed := payload
	changed.SkillIDs = append([]string(nil), payload.SkillIDs...)
	changed.SkillIDs = append(changed.SkillIDs, "custom-c")
	if skillSuspensionImpactPayloadEqual(decoded, changed) {
		t.Fatal("impact token accepted a changed disabled skill set")
	}
	if _, err := decodeSkillSuspensionImpactToken([]byte("different-secret"), token); !errors.Is(err, ErrImpactTokenInvalid) {
		t.Fatalf("decode with another secret error = %v, want ErrImpactTokenInvalid", err)
	}
}
