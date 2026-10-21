package catalog

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestMessagingContractsRoundTripAndAdmission(t *testing.T) {
	definitions, err := MessagingDefinitions()
	if err != nil || len(definitions) != 3 {
		t.Fatalf("definitions: %v", err)
	}
	for _, d := range definitions {
		t.Run(d.ID, func(t *testing.T) {
			if !d.IsMessagingCredentials() || !d.UsesCredentials() || d.IsAppCredentials() {
				t.Fatal("incorrect versioned credential family")
			}
			manifest, err := BuildAgentPluginManifest([]Definition{d})
			if err != nil {
				t.Fatal(err)
			}
			loader, _ := NewPluginLoader()
			loaded, err := loader.LoadManifestPackage(context.Background(), manifest)
			if err != nil || len(loaded.Failures) != 0 || len(loaded.Definitions) != 1 {
				t.Fatalf("load: %v %+v", err, loaded.Failures)
			}
			d = loaded.Definitions[0]
			record, err := definitionAdmissionRecord(d, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			row := storedDefinitionRow{ConnectorID: d.ID, PluginID: d.PluginID, PluginVersion: d.PluginVersion, PluginDigest: d.PluginDigest, SourceDialect: d.SourceDialect, SchemaVersion: d.SourceSchemaVersion, ProviderID: d.ProviderID, DriverID: d.DriverID, Classification: string(d.Classification), Display: record.Display, KeySchema: record.KeySchema, CapabilitySummary: record.CapabilitySummary, ContractDigest: record.ContractDigest}
			restored, err := row.definition()
			if err != nil || CheckCompatibleUpdate(d, restored) != nil {
				t.Fatalf("stored contract: %v", err)
			}
			for _, mutate := range []func(*Definition){
				func(d *Definition) { d.Setup.Handler = "arbitrary-handler" },
				func(d *Definition) { d.CredentialFields[0].Name = "url" },
				func(d *Definition) { d.Setup.ProtocolVersion++ },
				func(d *Definition) { d.Setup.Methods = append(d.Setup.Methods, "execute_script") },
				func(d *Definition) { d.ExtensionSchemaVersion = "1.1.0" },
				func(d *Definition) { d.Operations[0].ToolName = "untrusted_tool" },
				func(d *Definition) { d.Operations[0].InputSchema = map[string]any{"type": "object"} },
				func(d *Definition) { d.Operations = d.Operations[1:] },
			} {
				bad := cloneDefinition(d)
				mutate(&bad)
				if Validate(bad) == nil {
					t.Fatal("untrusted credential contract accepted")
				}
			}
		})
	}
}

func TestMessagingDefinitionValuesAndClone(t *testing.T) {
	defs, err := MessagingDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		values := map[string]string{}
		for _, f := range d.CredentialFields {
			values[f.Name] = "fixture-value"
		}
		if d.ID == "weixin" {
			values["base_url"] = "https://ilinkai.weixin.qq.com"
		}
		if err := ValidateCredentialValues(d, values); err != nil {
			t.Fatalf("%s: %v", d.ID, err)
		}
		values["untrusted"] = "value"
		if ValidateCredentialValues(d, values) == nil {
			t.Fatal("unknown field accepted")
		}
		clone := cloneDefinition(d)
		clone.Setup.Intents[0] = "changed"
		if d.Setup.Intents[0] == "changed" {
			t.Fatal("mutable shared intents")
		}
		data, _ := json.Marshal(d)
		if len(data) == 0 {
			t.Fatal("empty contract")
		}
	}
}
