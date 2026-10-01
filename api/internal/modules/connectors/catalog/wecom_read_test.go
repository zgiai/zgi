package catalog

import (
	"reflect"
	"testing"
)

func TestWeComReadsAreAdditiveAndOldDefinitionsRemainValid(t *testing.T) {
	old := messagingDefinition("wecom")
	latest := WeComReadDefinition()
	if err := Validate(old); err != nil {
		t.Fatal(err)
	}
	if err := Validate(latest); err != nil {
		t.Fatal(err)
	}
	if len(latest.Operations) != 10 || latest.PluginVersion == old.PluginVersion {
		t.Fatal("missing versioned operations")
	}
	if err := CheckCompatibleUpdate(old, latest); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(old.Operations, latest.Operations[:3]) {
		t.Fatal("original operation changed")
	}
	for _, op := range latest.Operations[3:] {
		if !IsWeComResourceRead(op.ID) || op.Effect != OperationEffectRead || !op.Idempotent || op.DataEgress || op.CredentialSubject != "bot" {
			t.Fatal("unsafe resource contract")
		}
	}
	for _, op := range latest.Operations {
		if op.ID != "wecom.calendar.list" && op.ID != "wecom.calendar.get" {
			continue
		}
		properties := op.InputSchema["properties"].(map[string]any)
		if properties["cursor"] != nil || properties["limit"] != nil {
			t.Fatal("invented upstream pagination")
		}
	}
	latest.PluginVersion = old.PluginVersion
	if err := Validate(latest); err == nil {
		t.Fatal("new operations reused old package version")
	}
}
