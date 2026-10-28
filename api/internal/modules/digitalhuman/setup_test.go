package digitalhuman

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"gorm.io/datatypes"
)

// The operations template requires its skill and a server connection that
// allows the read actions; write actions stay the operator's choice
// (ADR 0023).
func TestOpsSpecialistSetupRequirements(t *testing.T) {
	requirements := setupRequirementsFromTemplate("ops_specialist")
	if requirements.TemplateID != "ops_specialist" || len(requirements.Skills) != 1 || requirements.Skills[0] != "ops-runbook" ||
		len(requirements.Connectors) != 1 || requirements.Connectors[0].ConnectorID != "serverops_ssh" {
		t.Fatalf("requirements = %#v", requirements)
	}
	if !setupRequirementsFromTemplate("support_desk").empty() || !setupRequirementsFromTemplate("").empty() || !setupRequirementsFromTemplate("unknown").empty() {
		t.Fatal("templates without required items, and unknown ones, require nothing")
	}

	if status := evaluateSetup(requirements, nil); status.Complete || len(status.Items) != 2 || status.Items[0].Met || status.Items[1].Met {
		t.Fatalf("empty draft status = %#v", status)
	}

	skill := &ResourceBinding{BindingType: resourcebindings.BindingTypeSkill, ResourceID: "ops-runbook"}
	partial := &ResourceBinding{
		BindingType: resourcebindings.BindingTypeConnector, ParentResourceID: "serverops_ssh",
		Metadata: datatypes.JSON(`{"allowed_operation_ids":["server.system.snapshot","server.service.restart"]}`),
	}
	if status := evaluateSetup(requirements, []*ResourceBinding{skill, partial}); status.Complete || !status.Items[0].Met || status.Items[1].Met {
		t.Fatalf("a connection without every read action must not meet the need: %#v", status)
	}

	full := &ResourceBinding{
		BindingType: resourcebindings.BindingTypeConnector, ParentResourceID: "serverops_ssh",
		Metadata: datatypes.JSON(`{"allowed_operation_ids":["server.system.snapshot","server.service.status","server.journal.tail"]}`),
	}
	if status := evaluateSetup(requirements, []*ResourceBinding{skill, partial, full}); !status.Complete {
		t.Fatalf("complete draft status = %#v", status)
	}

	encoded, err := encodeSetupRequirements(requirements)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeSetupRequirements(encoded)
	if err != nil || decoded.TemplateID != "ops_specialist" || len(decoded.Connectors) != 1 {
		t.Fatalf("round trip = %#v, %v", decoded, err)
	}
	if empty, _ := encodeSetupRequirements(SetupRequirements{}); string(empty) != "{}" {
		t.Fatalf("empty requirements = %s", empty)
	}
}
