package channelruntime

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm/schema"
)

func TestDeliveryColumnMappingsMatchMigration(t *testing.T) {
	parsed, err := schema.Parse(&Delivery{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse() error = %v", err)
	}

	for fieldName, wantColumn := range map[string]string{
		"Kind":        "delivery_kind",
		"DeliveryKey": "delivery_key",
		"Sequence":    "sequence",
	} {
		field := parsed.LookUpField(fieldName)
		if field == nil {
			t.Fatalf("field %q was not found", fieldName)
		}
		if field.DBName != wantColumn {
			t.Fatalf("field %q DBName = %q, want %q", fieldName, field.DBName, wantColumn)
		}
	}
}

func TestInstallationCredentialColumnMappings(t *testing.T) {
	parsed, err := schema.Parse(&Installation{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse() error = %v", err)
	}

	for fieldName, wantColumn := range map[string]string{
		"ConnectorID":                 "connector_id",
		"ConnectionOwnerID":           "connection_owner_id",
		"CredentialSource":            "credential_source",
		"ConnectionRevision":          "connection_revision",
		"ConnectionCredentialVersion": "connection_credential_version",
	} {
		field := parsed.LookUpField(fieldName)
		if field == nil {
			t.Fatalf("field %q was not found", fieldName)
		}
		if field.DBName != wantColumn {
			t.Fatalf("field %q DBName = %q, want %q", fieldName, field.DBName, wantColumn)
		}
	}

	// A pre-existing installation row has no credential source; the column
	// default keeps it readable as a legacy embedded installation.
	source := parsed.LookUpField("CredentialSource")
	if source.DefaultValue != string(CredentialSourceLegacy) {
		t.Fatalf("CredentialSource default = %q, want %q", source.DefaultValue, CredentialSourceLegacy)
	}
}

func TestCredentialSourceContractValues(t *testing.T) {
	for source, want := range map[CredentialSource]string{
		CredentialSourceEmbedded:           "embedded",
		CredentialSourcePersonalConnection: "personal_connection",
		CredentialSourceLegacy:             "legacy",
	} {
		if string(source) != want {
			t.Fatalf("credential source = %q, want %q", source, want)
		}
	}
}

func TestInstallationJSONHidesConnectionOwner(t *testing.T) {
	ownerID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	installation := Installation{
		ID:                          uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
		OrganizationID:              uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"),
		WorkspaceID:                 uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd"),
		Provider:                    "feishu",
		ConnectorID:                 "feishu",
		ConnectionID:                uuid.MustParse("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"),
		ConnectionOwnerID:           ownerID,
		CredentialSource:            CredentialSourcePersonalConnection,
		ConnectionRevision:          4,
		ConnectionCredentialVersion: 7,
		Name:                        "support",
		Status:                      InstallationStatusActive,
	}

	payload, err := json.Marshal(installation)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	raw := string(payload)
	if strings.Contains(raw, "connection_owner_id") || strings.Contains(raw, ownerID.String()) {
		t.Fatalf("installation JSON exposed owner-scoped identity: %s", raw)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for key, want := range map[string]any{
		"connector_id":                  "feishu",
		"credential_source":             "personal_connection",
		"connection_revision":           float64(4),
		"connection_credential_version": float64(7),
	} {
		got, ok := decoded[key]
		if !ok {
			t.Fatalf("installation JSON missing %q: %s", key, raw)
		}
		if got != want {
			t.Fatalf("installation JSON %q = %v, want %v", key, got, want)
		}
	}
}

func TestInstallationLegacyEmbeddedJSONStaysCompatible(t *testing.T) {
	installationID := uuid.MustParse("12345678-1234-1234-1234-123456789abc")
	installation := Installation{
		ID:             installationID,
		OrganizationID: uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"),
		WorkspaceID:    uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd"),
		Provider:       "feishu",
		ConnectionID:   installationID,
		Name:           "legacy",
		Status:         InstallationStatusActive,
	}

	payload, err := json.Marshal(installation)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	// An installation created before credential source tracking has an empty
	// in-memory source and must stay loadable instead of defaulting in code.
	if source, ok := decoded["credential_source"]; !ok || source != "" {
		t.Fatalf("legacy credential_source = %v, present = %v; want empty", source, ok)
	}
	for _, key := range []string{"connector_id", "connection_revision", "connection_credential_version"} {
		if _, ok := decoded[key]; ok {
			t.Fatalf("legacy installation JSON should omit %q: %s", key, string(payload))
		}
	}
	if decoded["connection_id"] != installationID.String() {
		t.Fatalf("legacy connection_id = %v, want %s", decoded["connection_id"], installationID)
	}
}

func TestResolvedChannelCredentialDestroyClearsOwnerScopedLease(t *testing.T) {
	credential := &ResolvedChannelCredential{
		Values:   map[string]string{"app_secret": "secret"},
		Version:  3,
		Revision: 9,
		Source:   CredentialSourcePersonalConnection,
	}

	credential.Destroy()

	if len(credential.Values) != 0 {
		t.Fatalf("Destroy() left credential values: %v", credential.Values)
	}
	if credential.Version != 0 || credential.Revision != 0 || credential.Source != "" {
		t.Fatalf("Destroy() = version %d, revision %d, source %q; want zero values", credential.Version, credential.Revision, credential.Source)
	}
}

func TestResolvedChannelCredentialDestroyNilIsSafe(t *testing.T) {
	var credential *ResolvedChannelCredential
	credential.Destroy()
}
