package catalog

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMailLegacyDigestFreeze(t *testing.T) {
	digest, err := DefinitionContractDigest(GmailDefinition())
	if err != nil || digest != "9a10ffe8439530bf48ea31e1c9d847cc0b9fa35ebf6e690624c03a041810ee8d" {
		t.Fatalf("Gmail contract changed: %s (%v)", digest, err)
	}
}

func TestMailV6ProvidersRoundTripAndIsolation(t *testing.T) {
	for _, id := range []string{"qqmail", "netease163"} {
		t.Run(id, func(t *testing.T) {
			d, ok := MailDefinition(id)
			if !ok || d.ContractVersion != 6 || Validate(d) != nil {
				t.Fatal("missing V6 mail definition")
			}
			manifest, err := BuildAgentPluginManifest([]Definition{d})
			if err != nil {
				t.Fatal(err)
			}
			loader, _ := NewPluginLoader()
			loaded, err := loader.LoadManifestPackage(context.Background(), manifest)
			if err != nil || len(loaded.Failures) != 0 || len(loaded.Definitions) != 1 {
				t.Fatalf("manifest roundtrip: %v", err)
			}
			d = loaded.Definitions[0]
			record, err := definitionAdmissionRecord(d, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			row := storedDefinitionRow{ConnectorID: d.ID, PluginID: d.PluginID, PluginVersion: d.PluginVersion, PluginDigest: d.PluginDigest, SourceDialect: d.SourceDialect, SchemaVersion: d.SourceSchemaVersion, ProviderID: d.ProviderID, DriverID: d.DriverID, Classification: string(d.Classification), Display: record.Display, KeySchema: record.KeySchema, CapabilitySummary: record.CapabilitySummary, ContractDigest: record.ContractDigest}
			restored, err := row.definition()
			if err != nil || CheckCompatibleUpdate(d, restored) != nil {
				t.Fatalf("stored roundtrip: %v", err)
			}
			for _, mutate := range []func(*Definition){
				func(d *Definition) { d.AuthMethodID = "gmail.app_password" },
				func(d *Definition) { d.ProviderID = "gmail" },
				func(d *Definition) { d.ExtensionSchemaVersion = ZGIMetadataSchemaVersion14 },
				func(d *Definition) { d.CredentialFields[1].Name = "app_password" },
				func(d *Definition) { d.Setup.Methods = append(d.Setup.Methods, "qr") },
				func(d *Definition) { d.Operations[3].Effect = OperationEffectRead },
			} {
				bad := cloneDefinition(d)
				mutate(&bad)
				if Validate(bad) == nil {
					t.Fatal("cross-provider or auth contract drift admitted")
				}
			}
			ops := MailOperationsFor(id, "read_send")
			if len(ops) != 4 {
				t.Fatal("incomplete mail operations")
			}
			for _, op := range ops {
				if !strings.HasPrefix(op, id+".") {
					t.Fatal("foreign mail operation")
				}
			}
		})
	}
}

func TestMailAuthorizationCodeProviderRules(t *testing.T) {
	for _, tc := range []struct {
		id, email, code string
		valid           bool
	}{
		{"qqmail", " Mixed@QQ.COM ", " AbcdefghijklMNOP ", true},
		{"netease163", "User@163.COM", "AbcdefghijklMNOP", true},
		{"netease163", "u@163.com", "ABCD EFGH\nIJKL\u00a0MNOP", true},
		{"qqmail", "u@qq.com", "1234567890123456", true},
		{"netease163", "u@163.com", "1234567890123456", true},
		{"qqmail", "u@163.com", "abcdefghijklmnop", false},
		{"netease163", "u@126.com", "abcdefghijklmnop", false},
		{"qqmail", "u@qq.com", "abcd efghijklmnop", true},
		{"qqmail", "u@qq.com", "abcdefghijklmnop\x00", false},
		{"qqmail", "u\u200b@qq.com", "abcdefghijklmnop", false},
	} {
		v := map[string]string{"email": tc.email, "authorization_code": tc.code}
		err := NormalizeMailValuesFor(tc.id, v)
		if (err == nil) != tc.valid {
			t.Errorf("%s valid=%t got %v", tc.id, tc.valid, err)
		}
		if tc.valid && strings.HasPrefix(strings.TrimSpace(tc.email), "Mixed") && v["email"] != "Mixed@qq.com" {
			t.Fatal("login identity changed")
		}
		if tc.valid && tc.id == "netease163" && strings.ContainsAny(tc.code, " \t\r\n\u00a0") && v["authorization_code"] != "ABCDEFGHIJKLMNOP" {
			t.Fatalf("netease authorization code separators were not normalized: %q", v["authorization_code"])
		}
	}
	for _, id := range []string{"qqmail", "netease163", "unknown"} {
		if NormalizeMailValuesFor(id, map[string]string{"email": "u@qq.com", "app_password": "abcdefghijklmnop"}) == nil {
			t.Fatal("foreign credential shape admitted")
		}
	}
}

func TestMailV5RoundTripAndRejectsAuthenticationDrift(t *testing.T) {
	d := GmailDefinition()
	if err := Validate(d); err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildAgentPluginManifest([]Definition{d})
	if err != nil {
		t.Fatal(err)
	}
	loader, _ := NewPluginLoader()
	loaded, err := loader.LoadManifestPackage(context.Background(), manifest)
	if err != nil || len(loaded.Failures) > 0 || len(loaded.Definitions) != 1 {
		t.Fatalf("mail manifest failed: %v", err)
	}
	d = loaded.Definitions[0]
	record, err := definitionAdmissionRecord(d, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	row := storedDefinitionRow{ConnectorID: d.ID, PluginID: d.PluginID, PluginVersion: d.PluginVersion, PluginDigest: d.PluginDigest, SourceDialect: d.SourceDialect, SchemaVersion: d.SourceSchemaVersion, ProviderID: d.ProviderID, DriverID: d.DriverID, Classification: string(d.Classification), Display: record.Display, KeySchema: record.KeySchema, CapabilitySummary: record.CapabilitySummary, ContractDigest: record.ContractDigest}
	restored, err := row.definition()
	if err != nil || CheckCompatibleUpdate(d, restored) != nil {
		t.Fatalf("mail stored roundtrip: %v", err)
	}
	for _, change := range []func(*Definition){func(d *Definition) { d.ContractVersion = 4; d.ExtensionSchemaVersion = ZGIMetadataSchemaVersion13 }, func(d *Definition) { d.AuthMethodID = "gmail.oauth" }, func(d *Definition) { d.CredentialFields[0].Secret = true }, func(d *Definition) { d.Setup.Methods = append(d.Setup.Methods, "qr") }, func(d *Definition) { d.Operations[3].Effect = OperationEffectRead }, func(d *Definition) { d.Operations[0].RequiredScopes = [][]string{{"gmail.readonly"}} }} {
		bad := cloneDefinition(d)
		change(&bad)
		if Validate(bad) == nil {
			t.Fatal("mail contract drift admitted")
		}
	}
}
func TestMailNormalizationPreservesLoginIdentityAndPasswordCase(t *testing.T) {
	values := map[string]string{"email": " User.Name+tag@GMAIL.COM ", "app_password": "Abcd efgh\tijkl\r\nMNOP\u00a0"}
	if err := NormalizeMailValues(values); err != nil {
		t.Fatal(err)
	}
	if values["email"] != "User.Name+tag@gmail.com" || values["app_password"] != "AbcdefghijklMNOP" {
		t.Fatal("unexpected normalization")
	}
	for _, email := range []string{"Display <user@gmail.com>", "a@gmail.com,b@gmail.com", "user@gmail.com\r\nBcc:x@example.com", "user", strings.Repeat("a", 255) + "@gmail.com"} {
		if NormalizeMailValues(map[string]string{"email": email, "app_password": "abcdefghijklmnop"}) == nil {
			t.Fatal("invalid address admitted")
		}
	}
	for _, password := range []string{"abcd\u200befghijklmnop", "abcd\x00efghijklmnop"} {
		if NormalizeMailValues(map[string]string{"email": "user@example.com", "app_password": password}) == nil {
			t.Fatal("invalid password shape admitted")
		}
	}
	for _, password := range []string{"short", "12345678901234567"} {
		if NormalizeMailValues(map[string]string{"email": "user@example.com", "app_password": password}) != nil {
			t.Fatal("provider credential shape was rejected before server verification")
		}
	}
	if NormalizeMailValues(map[string]string{"email": "user@example.com", "app_password": "1234567890123456"}) != nil {
		t.Fatal("password shape must not assert alphabetic or eligibility constraints")
	}
}
