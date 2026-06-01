package service

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/zgiai/zgi/api/internal/dto"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestListAccessibleDatasetsUsesOrganizationScopeAndFallsBackWhenSearchMisses(t *testing.T) {
	db, mock := newKnowledgeMockDB(t)
	svc := &KnowledgeRetrievalService{db: db}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "members"`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT workspaces\.id FROM "workspaces"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("workspace-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "datasets"`)).
		WillReturnRows(datasetRows())
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "datasets"`)).
		WillReturnRows(datasetRows().AddRow("dataset-1", "org-1", "workspace-1", "故事大纲", "各类的故事大纲", "vendor", false, "account-1", time.Now()))

	response, err := svc.ListAccessibleDatasets(context.Background(), KnowledgeScope{
		OrganizationID: "org-1",
		AccountID:      "account-1",
	}, "看看系统中的知识库里的大纲", 10)
	if err != nil {
		t.Fatalf("ListAccessibleDatasets() error = %v", err)
	}
	if response.Status != KnowledgeListStatusFallback {
		t.Fatalf("Status = %q, want %q", response.Status, KnowledgeListStatusFallback)
	}
	if !response.FallbackUsed {
		t.Fatalf("FallbackUsed = false, want true")
	}
	if response.ResultCount != 1 || len(response.KnowledgeBases) != 1 {
		t.Fatalf("response count mismatch: %#v", response)
	}
	if response.KnowledgeBases[0].DatasetID != "dataset-1" {
		t.Fatalf("DatasetID = %q, want %q", response.KnowledgeBases[0].DatasetID, "dataset-1")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListAccessibleDatasetsReturnsNoResultsWhenNoWorkspaceAccess(t *testing.T) {
	db, mock := newKnowledgeMockDB(t)
	svc := &KnowledgeRetrievalService{db: db}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "members"`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT workspaces\.id FROM "workspaces"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	response, err := svc.ListAccessibleDatasets(context.Background(), KnowledgeScope{
		OrganizationID: "org-1",
		AccountID:      "account-1",
	}, "refund", 10)
	if err != nil {
		t.Fatalf("ListAccessibleDatasets() error = %v", err)
	}
	if response.Status != KnowledgeListStatusNoResults {
		t.Fatalf("Status = %q, want %q", response.Status, KnowledgeListStatusNoResults)
	}
	if response.ResultCount != 0 || len(response.KnowledgeBases) != 0 {
		t.Fatalf("response has results: %#v", response)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestAccessibleKnowledgeDatasetAllowsOrganizationScopedDataset(t *testing.T) {
	db, mock := newKnowledgeMockDB(t)
	svc := &KnowledgeRetrievalService{db: db}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "members"`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT workspaces\.id FROM "workspaces"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("workspace-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "datasets"`)).
		WillReturnRows(datasetRows().AddRow("dataset-1", "org-1", "workspace-1", "故事大纲", "各类的故事大纲", "vendor", false, "account-1", time.Now()))

	dataset, err := svc.accessibleKnowledgeDataset(context.Background(), "dataset-1", KnowledgeScope{
		OrganizationID: "org-1",
		AccountID:      "account-1",
	}, false)
	if err != nil {
		t.Fatalf("accessibleKnowledgeDataset() error = %v", err)
	}
	if dataset.Name != "故事大纲" {
		t.Fatalf("Name = %q, want %q", dataset.Name, "故事大纲")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestAccessibleKnowledgeDatasetGrantSkipsAccountMembership(t *testing.T) {
	db, mock := newKnowledgeMockDB(t)
	svc := &KnowledgeRetrievalService{db: db}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "datasets"`)).
		WillReturnRows(datasetRows().AddRow("dataset-1", "org-1", "workspace-1", "Stories", "Story corpus", "vendor", false, "account-1", time.Now()))

	dataset, err := svc.accessibleKnowledgeDataset(context.Background(), "dataset-1", KnowledgeScope{
		OrganizationID: "org-1",
		WorkspaceID:    "workspace-1",
		AccountID:      "revoked-binder",
	}, true)
	if err != nil {
		t.Fatalf("accessibleKnowledgeDataset() error = %v", err)
	}
	if dataset.ID != "dataset-1" {
		t.Fatalf("ID = %q, want dataset-1", dataset.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestKnowledgeResourcesAndContextBuildsSourceMarkedBlocks(t *testing.T) {
	records := []scoredKnowledgeRecord{{
		DatasetID:   "dataset-1",
		DatasetName: "Product FAQ",
		Record: dto.HitTestingRecordResponse{
			Score:     0.91,
			MatchType: "hybrid",
			Segment: dto.SegmentResponse{
				ID:         "segment-1",
				DocumentID: "document-1",
				Content:    "Refunds are available within 30 days.",
				Document: dto.HitTestingDocumentResponse{
					ID:             "document-1",
					Name:           "Refund Policy",
					DataSourceType: "upload",
				},
			},
		},
	}}

	resources, contextText, blocks := knowledgeResourcesAndContext(records, "\n\n", 12000)
	if len(resources) != 1 || len(blocks) != 1 {
		t.Fatalf("resources=%d blocks=%d, want 1 each", len(resources), len(blocks))
	}
	if resources[0].Position != blocks[0].Position {
		t.Fatalf("block position = %d, want resource position %d", blocks[0].Position, resources[0].Position)
	}
	if blocks[0].Source != "Product FAQ / Refund Policy" {
		t.Fatalf("block source = %q", blocks[0].Source)
	}
	if !strings.Contains(contextText, "[1] Source: Product FAQ / Refund Policy") {
		t.Fatalf("context missing source marker: %q", contextText)
	}
	if !strings.Contains(contextText, "Score: 0.9100") {
		t.Fatalf("context missing score: %q", contextText)
	}
}

func TestRetrieveAgentKnowledgeWithoutConfiguredDatasetsReturnsNoConfig(t *testing.T) {
	db, mock := newKnowledgeMockDB(t)
	svc := &KnowledgeRetrievalService{db: db}

	mock.ExpectQuery(`SELECT .* FROM "agents_configs"`).
		WillReturnRows(sqlmock.NewRows([]string{"dataset_configs", "configs", "retriever_resource", "agent_mode"}))

	response, err := svc.RetrieveAgentKnowledge(context.Background(), KnowledgeRetrieveRequest{
		Scope: KnowledgeScope{
			AppID:     "agent-1",
			AccountID: "account-1",
		},
		Query: "refund policy",
	})
	if err != nil {
		t.Fatalf("RetrieveAgentKnowledge() error = %v", err)
	}
	if response.Status != KnowledgeRetrieveStatusNoConfig {
		t.Fatalf("Status = %q, want %q", response.Status, KnowledgeRetrieveStatusNoConfig)
	}
	if response.ResultCount != 0 || len(response.Resources) != 0 {
		t.Fatalf("response has results: %#v", response)
	}
	if len(response.Warnings) == 0 {
		t.Fatalf("Warnings empty, want no config warning")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRetrieveAgentKnowledgeWithRuntimeDatasetsSkipsLegacyConfigFallback(t *testing.T) {
	db, mock := newKnowledgeMockDB(t)
	svc := &KnowledgeRetrievalService{db: db}

	_, err := svc.RetrieveAgentKnowledge(context.Background(), KnowledgeRetrieveRequest{
		Scope: KnowledgeScope{
			AppID:          "agent-1",
			OrganizationID: "org-1",
			AccountID:      "account-1",
		},
		Query:           "refund policy",
		DatasetIDs:      []string{"runtime-dataset"},
		RetrievalConfig: map[string]interface{}{"top_k": float64(5)},
	})
	if err == nil || !strings.Contains(err.Error(), "knowledge retrieval service is not configured") {
		t.Fatalf("RetrieveAgentKnowledge() error = %v, want direct retrieval configuration error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func newKnowledgeMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return db, mock
}

func datasetRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id",
		"organization_id",
		"workspace_id",
		"name",
		"description",
		"provider",
		"enable_graph_flow",
		"created_by",
		"created_at",
	})
}
