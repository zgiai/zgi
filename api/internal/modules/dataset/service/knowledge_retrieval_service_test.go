package service

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
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

	datasets, err := svc.ListAccessibleDatasets(context.Background(), KnowledgeScope{
		OrganizationID: "org-1",
		AccountID:      "account-1",
	}, "看看系统中的知识库里的大纲", 10)
	if err != nil {
		t.Fatalf("ListAccessibleDatasets() error = %v", err)
	}
	if len(datasets) != 1 {
		t.Fatalf("len(datasets) = %d, want 1: %#v", len(datasets), datasets)
	}
	if datasets[0].DatasetID != "dataset-1" {
		t.Fatalf("DatasetID = %q, want %q", datasets[0].DatasetID, "dataset-1")
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
	})
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
