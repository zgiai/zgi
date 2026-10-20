package socialops

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestCurrentConfigurationSaveLoadUsesStableScopedCAS(t *testing.T) {
	service, scope, _, _ := fixture(t)
	configuration := validConfiguration("x-social", RequirementOptional)
	configuration.Scope = scope
	configuration.Revision = 999

	saved, record, err := service.SaveCurrentConfiguration(t.Context(), scope, 0, configuration)
	if err != nil {
		t.Fatalf("SaveCurrentConfiguration(create): %v", err)
	}
	if saved.Revision != 1 || record.Revision != 1 || record.Kind != CurrentConfigurationRecordKind || record.Key != CurrentConfigurationRecordKey {
		t.Fatalf("created configuration = %#v, record = %#v", saved, record)
	}
	loaded, loadedRecord, err := service.LoadCurrentConfiguration(t.Context(), scope)
	if err != nil || loaded.Revision != 1 || loaded.Scope != scope || loadedRecord.ID != record.ID {
		t.Fatalf("LoadCurrentConfiguration() = %#v, %#v, %v", loaded, loadedRecord, err)
	}

	configuration.Product.Name = "Acme Analytics 2"
	updated, updatedRecord, err := service.SaveCurrentConfiguration(t.Context(), scope, record.Revision, configuration)
	if err != nil || updated.Revision != 2 || updatedRecord.Revision != 2 {
		t.Fatalf("SaveCurrentConfiguration(update) = %#v, %#v, %v", updated, updatedRecord, err)
	}
	if _, _, err := service.SaveCurrentConfiguration(t.Context(), scope, record.Revision, configuration); err == nil {
		t.Fatal("stale current configuration CAS succeeded")
	}
	other := scope
	other.OwnerID = uuid.New()
	if _, _, err := service.LoadCurrentConfiguration(t.Context(), other); err == nil {
		t.Fatal("cross-owner current configuration loaded")
	}
}

func TestCurrentConfigurationLoadRejectsArtifactRevisionMismatchAndUnknownFields(t *testing.T) {
	service, scope, _, _ := fixture(t)
	configuration := validConfiguration("x-social", RequirementOptional)
	configuration.Scope = scope
	configuration.Revision = 9
	data, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-1], []byte(`,"caller_authorized":true}`)...)
	artifact, err := service.PutArtifact(context.Background(), scope, "application/json", data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveRecord(t.Context(), scope, CurrentConfigurationRecordKind, CurrentConfigurationRecordKey, 0, artifact.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.LoadCurrentConfiguration(t.Context(), scope); err == nil {
		t.Fatal("configuration with caller authority field loaded")
	}
}
