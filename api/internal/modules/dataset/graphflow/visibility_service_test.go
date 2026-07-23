package graphflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"gorm.io/gorm"
)

type visibilityTestRef struct {
	ID                uuid.UUID `gorm:"primaryKey"`
	OrganizationID    string
	WorkspaceID       *string
	DatasetID         string
	AssetID           uuid.UUID
	DatasetDocumentID *uuid.UUID
	RetrievalEnabled  bool
	DeletedAt         gorm.DeletedAt
	UpdatedAt         time.Time
}

func (visibilityTestRef) TableName() string {
	return "data_library_knowledge_base_asset_refs"
}

func TestVisibilityServiceIsIdempotentWithoutGraphRun(t *testing.T) {
	db := openLifecycleTestDB(t, &visibilityTestRef{})
	datasetID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	documentID := uuid.New()
	refID := uuid.New()
	workspace := workspaceID.String()
	if err := db.Create(&lifecycleTestDataset{
		ID:             datasetID.String(),
		OrganizationID: organizationID.String(),
		WorkspaceID:    workspace,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&visibilityTestRef{
		ID:                refID,
		OrganizationID:    organizationID.String(),
		WorkspaceID:       &workspace,
		DatasetID:         datasetID.String(),
		AssetID:           uuid.New(),
		DatasetDocumentID: &documentID,
		RetrievalEnabled:  true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	service := NewVisibilityService(db)
	request := VisibilityChangeRequest{
		OrganizationID:   organizationID,
		WorkspaceID:      &workspaceID,
		DatasetID:        datasetID,
		SourceRefID:      refID,
		DocumentID:       documentID,
		RetrievalEnabled: false,
	}
	revision, changed, err := service.SetDocumentRetrievalEnabled(context.Background(), request)
	if err != nil || !changed || revision != 1 {
		t.Fatalf("first visibility update: revision=%d changed=%v err=%v", revision, changed, err)
	}
	revision, changed, err = service.SetDocumentRetrievalEnabled(context.Background(), request)
	if err != nil || changed || revision != 1 {
		t.Fatalf("second visibility update: revision=%d changed=%v err=%v", revision, changed, err)
	}

	var eventCount int64
	var runCount int64
	if err := db.Model(&graphmodel.GraphOutboxEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&graphmodel.GraphFlowRun{}).Count(&runCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 || runCount != 0 {
		t.Fatalf("event count=%d run count=%d", eventCount, runCount)
	}
}

func TestVisibilityServiceRejectsStaleDocumentAndTenant(t *testing.T) {
	db := openLifecycleTestDB(t, &visibilityTestRef{})
	datasetID := uuid.New()
	organizationID := uuid.New()
	documentID := uuid.New()
	refID := uuid.New()
	if err := db.Create(&lifecycleTestDataset{ID: datasetID.String(), OrganizationID: organizationID.String()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&visibilityTestRef{
		ID:                refID,
		OrganizationID:    organizationID.String(),
		DatasetID:         datasetID.String(),
		AssetID:           uuid.New(),
		DatasetDocumentID: &documentID,
		RetrievalEnabled:  true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewVisibilityService(db)

	_, _, err := service.SetDocumentRetrievalEnabled(context.Background(), VisibilityChangeRequest{
		OrganizationID: organizationID,
		DatasetID:      datasetID,
		SourceRefID:    refID,
		DocumentID:     uuid.New(),
	})
	if !errors.Is(err, ErrStaleDocumentSnapshot) || err.Error() != "stale document snapshot" {
		t.Fatalf("unexpected stale document error: %v", err)
	}
	_, _, err = service.SetDocumentRetrievalEnabled(context.Background(), VisibilityChangeRequest{
		OrganizationID: uuid.New(),
		DatasetID:      datasetID,
		SourceRefID:    refID,
		DocumentID:     documentID,
	})
	if !errors.Is(err, ErrGraphFlowTenantScopeMismatch) || err.Error() != "graph flow tenant scope mismatch" {
		t.Fatalf("unexpected scope error: %v", err)
	}

	var stored visibilityTestRef
	if err := db.First(&stored, "id = ?", refID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.RetrievalEnabled {
		t.Fatal("cross-tenant request changed retrieval eligibility")
	}
}

func TestGraphSourceEligibilityRequiresCurrentEnabledRef(t *testing.T) {
	if graphSourceIsActive(false, true, true) {
		t.Fatal("stale ref contributed an active source")
	}
	if graphSourceIsActive(true, false, true) {
		t.Fatal("disabled ref contributed an active source")
	}
	if graphSourceIsActive(true, true, false) {
		t.Fatal("disabled document contributed an active source")
	}
	if !graphSourceIsActive(true, true, true) {
		t.Fatal("current enabled source was excluded")
	}
}
