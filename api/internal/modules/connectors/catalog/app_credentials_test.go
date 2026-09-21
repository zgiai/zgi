package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLegacyBuiltinDigestBaseline(t *testing.T) {
	data, err := os.ReadFile("testdata/builtin-contracts.json")
	if err != nil {
		t.Fatal(err)
	}
	var definitions []Definition
	if json.Unmarshal(data, &definitions) != nil {
		t.Fatal("historical contract fixture is invalid")
	}
	count := 0
	for _, definition := range definitions {
		if definition.PluginID != GitHubPluginID && definition.PluginID != ExaPluginID {
			continue
		}
		manifest, err := BuildAgentPluginManifest([]Definition{definition})
		if err != nil {
			t.Fatal(err)
		}
		digest, err := DefinitionContractDigest(definition)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string][2]string{
			"github":     {"583334e2db7735d6f922aee1f46a0de22ba6eec9797b0b750dbfc374da038b06", "c3d1e6807b6c2aab437607ba7f3c1d6850d3c67ffa425a97d4bab6e9a098422c"},
			"web-search": {"384011e113a8d1cb0952629b0dc23127b5fc36dcf47eb3438bbe47f2cc616ce7", "01597a8e615f7bd6d7a6d16c92221b57c8c7be4122a7c3d6e93cd3ff44fcc401"},
		}[definition.ID]
		if fmt.Sprintf("%x", sha256.Sum256(manifest)) != want[0] || digest != want[1] {
			t.Fatalf("legacy digest drift: %s", definition.ID)
		}
		count += len(definition.Operations)
	}
	if count != 10 {
		t.Fatalf("legacy operation count = %d", count)
	}
}

func appDefinitionFixture(t *testing.T) Definition {
	t.Helper()
	definitions, err := BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	d := cloneDefinition(definitions[0])
	d.ID, d.PluginID, d.ProviderID, d.DriverID, d.AuthMethodID = "feishu", "zgi.feishu", "feishu", "feishu-openapi-v1", "feishu.app_credentials"
	d.ExtensionSchemaVersion, d.ContractVersion = ZGIMetadataSchemaVersion11, 2
	d.SetupMethod, d.KeyFields = SetupMethodAppCredentials, nil
	d.CredentialFields = []CredentialField{
		{Name: "region", Label: "Region", Required: true, Type: "string", Enum: []string{"feishu", "lark"}, MaxLength: 6},
		{Name: "app_id", Label: "App ID", Required: true, Type: "string", MaxLength: 128},
		{Name: "app_secret", Label: "App Secret", Required: true, Secret: true, Type: "string", MaxLength: 16384},
	}
	d.Setup = &SetupDescriptor{Handler: "feishu-app-registration-v1", Methods: []string{"manual", "qr_create", "qr_update"}, TemplateID: "feishu-app-template-v1", TemplateDigest: strings.Repeat("a", 64)}
	for i := range d.Operations {
		d.Operations[i].CredentialSubject = "app"
		d.Operations[i].RequiredScopes = [][]string{{"im:chat:read"}}
	}
	return d
}

func TestAppCredentialContractAdmissionAndRoundTrip(t *testing.T) {
	d := appDefinitionFixture(t)
	manifest, err := BuildAgentPluginManifest([]Definition{d})
	if err != nil {
		t.Fatal(err)
	}
	loader, err := NewPluginLoader()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loader.LoadManifestPackage(context.Background(), manifest)
	if err != nil || len(loaded.Failures) != 0 || len(loaded.Definitions) != 1 {
		t.Fatalf("admission: %v %#v", err, loaded.Failures)
	}
	d = loaded.Definitions[0]
	record, err := definitionAdmissionRecord(d, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	row := storedDefinitionRow{ConnectorID: d.ID, PluginID: d.PluginID, PluginVersion: d.PluginVersion, PluginDigest: d.PluginDigest, SourceDialect: d.SourceDialect, SchemaVersion: d.SourceSchemaVersion, ProviderID: d.ProviderID, DriverID: d.DriverID, Classification: string(d.Classification), Display: record.Display, KeySchema: record.KeySchema, CapabilitySummary: record.CapabilitySummary, ContractDigest: record.ContractDigest}
	roundTrip, err := row.definition()
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip.ExtensionSchemaVersion != ZGIMetadataSchemaVersion11 || roundTrip.ContractVersion != 2 || CheckCompatibleUpdate(d, roundTrip) != nil {
		t.Fatal("lost versioned contract")
	}
	encoded, _ := json.Marshal(d)
	var historical Definition
	if json.Unmarshal(encoded, &historical) != nil || CheckCompatibleUpdate(d, historical) != nil {
		t.Fatal("historical contract drift")
	}
	mutated := cloneDefinition(d)
	mutated.Operations[0].RequiredScopes[0][0] = "im:message:readonly"
	if CheckCompatibleUpdate(d, mutated) == nil {
		t.Fatal("permission contract not frozen or clone not defensive")
	}
	mutated = cloneDefinition(d)
	mutated.CredentialFields[0].Enum[0] = "evil"
	if Validate(mutated) == nil {
		t.Fatal("arbitrary credential region accepted")
	}
	legacy := cloneDefinition(d)
	legacy.ExtensionSchemaVersion, legacy.ContractVersion = "", 0
	if Validate(legacy) == nil {
		t.Fatal("v1 silently accepted v1.1 fields")
	}
	bad := strings.Replace(string(manifest), `"schema_version":"1.1.0"`, `"schema_version":"1.0.0"`, 1)
	if got, _ := loader.LoadManifestPackage(context.Background(), []byte(bad)); len(got.Definitions) != 0 {
		t.Fatal("v1 manifest accepted v1.1 metadata")
	}
}

func TestAppCredentialValuesStrict(t *testing.T) {
	d := appDefinitionFixture(t)
	valid := map[string]string{"region": "feishu", "app_id": "cli_example", "app_secret": "fixture-not-a-real-secret"}
	if ValidateCredentialValues(d, valid) != nil {
		t.Fatal("valid fields rejected")
	}
	for _, mutation := range []func(map[string]string){
		func(v map[string]string) { v["region"] = "https://localhost" },
		func(v map[string]string) { v["user_token"] = "hidden" },
		func(v map[string]string) { delete(v, "app_id") },
		func(v map[string]string) { v["app_secret"] = "x\nheader" },
	} {
		candidate := map[string]string{}
		for k, v := range valid {
			candidate[k] = v
		}
		mutation(candidate)
		if ValidateCredentialValues(d, candidate) == nil {
			t.Fatal("invalid fields accepted")
		}
	}
}
