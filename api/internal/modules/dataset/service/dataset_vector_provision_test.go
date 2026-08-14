package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	quotaModel "github.com/zgiai/zgi/api/internal/modules/quota/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspaceModel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type datasetProvisionVectorDB struct {
	vectordb.VectorDB
	className  string
	properties []map[string]interface{}
	err        error
}

type datasetProvisionOrganizationService struct {
	interfaces.OrganizationService
	organization *workspaceModel.Organization
}

func (f *datasetProvisionOrganizationService) GetOrganizationByWorkspaceID(context.Context, string) (*workspaceModel.Organization, error) {
	return f.organization, nil
}

type datasetProvisionQuotaService struct {
	interfaces.QuotaService
}

func (datasetProvisionQuotaService) RecordUsageInTx(context.Context, *gorm.DB, *quotaModel.QuotaUsageHistory) error {
	return nil
}

func (f *datasetProvisionVectorDB) CreateClass(_ context.Context, className string, properties []map[string]interface{}) error {
	f.className = className
	f.properties = properties
	return f.err
}

func TestEnsureDatasetVectorClassUsesDatasetCollection(t *testing.T) {
	vectorDB := &datasetProvisionVectorDB{}
	service := &datasetService{vectorDB: vectorDB}

	datasetID := "c0bedab6-17ed-4119-ba07-b4d7ffb87adf"
	if err := service.ensureDatasetVectorClass(context.Background(), datasetID); err != nil {
		t.Fatalf("ensureDatasetVectorClass: %v", err)
	}

	if vectorDB.className != model.GenCollectionNameByID(datasetID) {
		t.Fatalf("class name=%q, want %q", vectorDB.className, model.GenCollectionNameByID(datasetID))
	}
	if len(vectorDB.properties) != 1 || vectorDB.properties[0]["name"] != "text" || vectorDB.properties[0]["tokenization"] != "gse_ch" {
		t.Fatalf("properties=%#v", vectorDB.properties)
	}
}

func TestEnsureDatasetVectorClassPropagatesProvisionFailure(t *testing.T) {
	vectorDB := &datasetProvisionVectorDB{err: errors.New("weaviate unavailable")}
	service := &datasetService{vectorDB: vectorDB}

	err := service.ensureDatasetVectorClass(context.Background(), "dataset-1")
	if err == nil || !strings.Contains(err.Error(), "provision dataset vector class") || !strings.Contains(err.Error(), "weaviate unavailable") {
		t.Fatalf("error=%v", err)
	}
}

func TestCreateDatasetProvisionsVectorClassBeforeCommit(t *testing.T) {
	db := openDatasetProvisionDB(t)
	vectorDB := &datasetProvisionVectorDB{}
	service := newDatasetProvisionService(db, vectorDB)

	dataset, err := service.CreateDataset(context.Background(), datasetProvisionRequest())
	if err != nil {
		t.Fatalf("CreateDataset: %v", err)
	}
	if dataset == nil || dataset.ID == "" {
		t.Fatalf("dataset=%+v", dataset)
	}
	if vectorDB.className != model.GenCollectionNameByID(dataset.ID) {
		t.Fatalf("class name=%q, want %q", vectorDB.className, model.GenCollectionNameByID(dataset.ID))
	}
	var datasetCount int64
	if err := db.Model(&model.Dataset{}).Count(&datasetCount).Error; err != nil || datasetCount != 1 {
		t.Fatalf("dataset count=%d err=%v", datasetCount, err)
	}
}

func TestCreateDatasetRollsBackWhenVectorClassProvisionFails(t *testing.T) {
	db := openDatasetProvisionDB(t)
	vectorDB := &datasetProvisionVectorDB{err: errors.New("weaviate unavailable")}
	service := newDatasetProvisionService(db, vectorDB)

	_, err := service.CreateDataset(context.Background(), datasetProvisionRequest())
	if err == nil || !strings.Contains(err.Error(), "weaviate unavailable") {
		t.Fatalf("CreateDataset error=%v", err)
	}
	var datasetCount int64
	if countErr := db.Model(&model.Dataset{}).Count(&datasetCount).Error; countErr != nil || datasetCount != 0 {
		t.Fatalf("dataset count=%d err=%v", datasetCount, countErr)
	}
	var ruleCount int64
	if countErr := db.Model(&model.DatasetProcessRule{}).Count(&ruleCount).Error; countErr != nil || ruleCount != 0 {
		t.Fatalf("process rule count=%d err=%v", ruleCount, countErr)
	}
}

func openDatasetProvisionDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	statements := []string{
		`CREATE TABLE datasets (
			id text PRIMARY KEY, organization_id text NOT NULL, workspace_id text, name text NOT NULL,
			description text, provider text, permission text, enable_graph_flow boolean, graph_status text,
			graph_revision integer, graph_available_revision integer, graph_projected_revision integer,
			graph_visibility_revision integer, graph_projected_visibility_revision integer,
			graph_current_run_id text, graph_progress integer, graph_error_code text, graph_error_message text,
			graph_ready_at datetime, graph_updated_at datetime, created_by text NOT NULL, created_at datetime,
			updated_by text, updated_at datetime, owner text, embedding_model text, embedding_model_provider text,
			entity_model text, entity_model_provider text, collection_binding_id text, retrieval_config text,
			icon_type text, icon text, icon_background text, process_rule text
		)`,
		`CREATE TABLE dataset_process_rules (
			id text PRIMARY KEY, dataset_id text NOT NULL, mode text NOT NULL, rules text,
			created_by text NOT NULL, created_at datetime
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create dataset test table: %v", err)
		}
	}
	return db
}

func newDatasetProvisionService(db *gorm.DB, vectorDB vectordb.VectorDB) *datasetService {
	return &datasetService{
		db:                db,
		vectorDB:          vectorDB,
		enterpriseService: &datasetProvisionOrganizationService{organization: &workspaceModel.Organization{ID: "11111111-1111-1111-1111-111111111111"}},
		quotaService:      datasetProvisionQuotaService{},
	}
}

func datasetProvisionRequest() *CreateDatasetRequest {
	return &CreateDatasetRequest{
		WorkspaceID: "22222222-2222-2222-2222-222222222222",
		Name:        "Provisioned dataset",
		CreatedBy:   "33333333-3333-3333-3333-333333333333",
	}
}
