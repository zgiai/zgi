package service

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/dataset/retrieval"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestListAccessibleDatasetsUsesOrganizationScopeAndFallsBackWhenSearchMisses(t *testing.T) {
	db, mock := newKnowledgeMockDB(t)
	svc := &KnowledgeRetrievalService{db: db}

	expectAccessibleKnowledgeWorkspaces(mock, "workspace-1")
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

	expectAccessibleKnowledgeWorkspaces(mock)

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

func TestListAccessibleDatasetCandidatesOrdersSelectedIDsWithValidPostgresINClause(t *testing.T) {
	db, mock := newKnowledgeMockDB(t)
	svc := &KnowledgeRetrievalService{db: db}

	expectAccessibleKnowledgeWorkspaces(mock, "workspace-1")
	mock.ExpectQuery(`SELECT count\(\*\) FROM "datasets" WHERE workspace_id IN \(\$1\) AND provider <> 'digital_human_memory'`).
		WithArgs("workspace-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT \* FROM "datasets" WHERE workspace_id IN \(\$1\) AND provider <> 'digital_human_memory' ORDER BY CASE WHEN id IN \(\$2,\$3\) THEN 0 ELSE 1 END, LOWER\(name\) ASC, id ASC LIMIT \$4`).
		WithArgs("workspace-1", "dataset-1", "dataset-2", 24).
		WillReturnRows(datasetRows().
			AddRow("dataset-1", "org-1", "workspace-1", "Selected one", "", "vendor", false, "account-1", time.Now()).
			AddRow("dataset-2", "org-1", "workspace-1", "Selected two", "", "vendor", false, "account-1", time.Now()))

	response, err := svc.ListAccessibleDatasetCandidates(context.Background(), KnowledgeScope{
		OrganizationID: "org-1",
		AccountID:      "account-1",
	}, "", []string{"dataset-1", "dataset-2"}, true, 1, 24)
	if err != nil {
		t.Fatalf("ListAccessibleDatasetCandidates() error = %v", err)
	}
	if response.Total != 2 || len(response.KnowledgeBases) != 2 {
		t.Fatalf("response count mismatch: %#v", response)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListAccessibleDatasetsDoesNotFallbackFromDirectEmptyPermissionSnapshot(t *testing.T) {
	db, mock := newKnowledgeMockDB(t)
	svc := &KnowledgeRetrievalService{db: db}

	rows := sqlmock.NewRows([]string{"id", "role", "role_id", "permissions", "permission_source"}).
		AddRow("workspace-1", workspace_model.WorkspaceRoleAdmin, workspace_model.WorkspaceBuiltinRoleAdminID, `[]`, workspace_model.WorkspaceMemberPermissionSourceDirect)
	mock.ExpectQuery(`SELECT workspaces\.id, workspace_members\.role, workspace_members\.role_id, COALESCE\(workspace_members\.permissions::text, ''\) AS permissions, workspace_members\.permission_source FROM "workspaces" JOIN workspace_members ON workspace_members\.workspace_id = workspaces\.id WHERE workspaces\.organization_id = \$1 AND workspace_members\.account_id = \$2`).
		WillReturnRows(rows)

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

func TestListAccessibleDatasetsFallsBackFromLegacyAdminRole(t *testing.T) {
	db, mock := newKnowledgeMockDB(t)
	svc := &KnowledgeRetrievalService{db: db}

	rows := sqlmock.NewRows([]string{"id", "role", "role_id", "permissions", "permission_source"}).
		AddRow("workspace-1", workspace_model.WorkspaceRoleAdmin, nil, `[]`, "")
	mock.ExpectQuery(`SELECT workspaces\.id, workspace_members\.role, workspace_members\.role_id, COALESCE\(workspace_members\.permissions::text, ''\) AS permissions, workspace_members\.permission_source FROM "workspaces" JOIN workspace_members ON workspace_members\.workspace_id = workspaces\.id WHERE workspaces\.organization_id = \$1 AND workspace_members\.account_id = \$2`).
		WillReturnRows(rows)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "datasets"`)).
		WillReturnRows(datasetRows().AddRow("dataset-1", "org-1", "workspace-1", "Legacy", "legacy admin access", "vendor", false, "account-1", time.Now()))

	response, err := svc.ListAccessibleDatasets(context.Background(), KnowledgeScope{
		OrganizationID: "org-1",
		AccountID:      "account-1",
	}, "legacy", 10)
	if err != nil {
		t.Fatalf("ListAccessibleDatasets() error = %v", err)
	}
	if response.Status != KnowledgeListStatusSuccess {
		t.Fatalf("Status = %q, want %q", response.Status, KnowledgeListStatusSuccess)
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

func TestAccessibleKnowledgeDatasetAllowsOrganizationScopedDataset(t *testing.T) {
	db, mock := newKnowledgeMockDB(t)
	svc := &KnowledgeRetrievalService{db: db}

	expectAccessibleKnowledgeWorkspaces(mock, "workspace-1")
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

	resources, contextText, blocks, err := knowledgeResourcesAndContext(records, "\n\n", 12000, "https://api.example.test")
	if err != nil {
		t.Fatalf("knowledgeResourcesAndContext() error = %v", err)
	}
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

func TestKnowledgeResourcesAndContextNormalizesImageURLs(t *testing.T) {
	records := []scoredKnowledgeRecord{{
		DatasetID:   "dataset-1",
		DatasetName: "Herbs",
		Record: dto.HitTestingRecordResponse{
			Score: 0.91,
			Segment: dto.SegmentResponse{
				ID:         "segment-1",
				DocumentID: "document-1",
				Content:    "黄芪图：![figure](images/huangqi.png)",
				Document: dto.HitTestingDocumentResponse{
					ID:   "document-1",
					Name: "黄芪",
				},
			},
		},
	}}

	resources, contextText, _, err := knowledgeResourcesAndContext(records, "\n\n", 12000, "https://api.lingyoungai.com")
	if err != nil {
		t.Fatalf("knowledgeResourcesAndContext() error = %v", err)
	}

	want := "https://api.lingyoungai.com/images/huangqi.png"
	if !strings.Contains(resources[0].Content, want) {
		t.Fatalf("resource content missing normalized URL: %q", resources[0].Content)
	}
	if !strings.Contains(contextText, want) {
		t.Fatalf("context missing normalized URL: %q", contextText)
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

func TestRerankInBatchesProcessesEveryCandidate(t *testing.T) {
	runner := &recordingKnowledgeReranker{}
	contents := make([]string, 300)
	for index := range contents {
		contents[index] = "candidate"
	}

	scores, scored, err := rerankInBatches(context.Background(), runner, "query", contents)
	if err != nil {
		t.Fatalf("rerankInBatches() error = %v", err)
	}
	if runner.calls != 10 {
		t.Fatalf("rerank calls = %d, want 10", runner.calls)
	}
	if runner.totalDocuments != len(contents) {
		t.Fatalf("reranked documents = %d, want %d", runner.totalDocuments, len(contents))
	}
	if len(scores) != len(contents) || len(scored) != len(contents) {
		t.Fatalf("result lengths = %d/%d, want %d", len(scores), len(scored), len(contents))
	}
	for index, ok := range scored {
		if !ok {
			t.Fatalf("candidate %d was not scored", index)
		}
	}
}

func TestRerankInBatchesReportsIncompleteBatchWithoutMixingScores(t *testing.T) {
	runner := &recordingKnowledgeReranker{omitLastResultOnCall: 2}
	contents := make([]string, 40)
	for index := range contents {
		contents[index] = "candidate"
	}

	scores, scored, err := rerankInBatches(context.Background(), runner, "query", contents)
	if err != nil {
		t.Fatalf("rerankInBatches() error = %v", err)
	}
	if len(scores) != len(contents) || len(scored) != len(contents) {
		t.Fatalf("result lengths = %d/%d, want %d", len(scores), len(scored), len(contents))
	}
	for index, ok := range scored {
		if runner.omitLastResultOnCall == 2 && index == 39 {
			if ok {
				t.Fatalf("omitted candidate %d was marked scored", index)
			}
			continue
		}
		if !ok {
			t.Fatalf("candidate %d was not scored", index)
		}
	}
}

func TestDeduplicateKnowledgeRecordsKeepsHighestRetrievalScore(t *testing.T) {
	records := []scoredKnowledgeRecord{
		{
			DatasetID: "dataset-a",
			Record:    dto.HitTestingRecordResponse{Score: 0.4, Segment: dto.SegmentResponse{ID: "segment-a", Content: "Apple and Intel"}},
		},
		{
			DatasetID: "dataset-b",
			Record:    dto.HitTestingRecordResponse{Score: 0.9, Segment: dto.SegmentResponse{ID: "segment-b", Content: " apple   AND intel "}},
		},
		{
			DatasetID: "dataset-c",
			Record:    dto.HitTestingRecordResponse{Score: 0.8, Segment: dto.SegmentResponse{ID: "segment-c", Content: "A different fact"}},
		},
	}

	unique := deduplicateKnowledgeRecords(records)
	if len(unique) != 2 {
		t.Fatalf("unique records = %d, want 2", len(unique))
	}
	if unique[0].DatasetID != "dataset-b" || unique[0].Record.Score != 0.9 {
		t.Fatalf("duplicate winner = %#v, want dataset-b score 0.9", unique[0])
	}
}

func TestNormalizeRetrievalModeSupportsVectorAndBM25Alias(t *testing.T) {
	if got := normalizeRetrievalMode("vectorAndBM25"); got != RetrievalModeVector {
		t.Fatalf("normalizeRetrievalMode(vectorAndBM25) = %q, want %q", got, RetrievalModeVector)
	}
}

func TestNormalizeKnowledgeLimitCapsTopKAtTen(t *testing.T) {
	if got := normalizeKnowledgeLimit(99, defaultKnowledgeTopK, maxKnowledgeTopK); got != 10 {
		t.Fatalf("normalizeKnowledgeLimit(99) = %d, want 10", got)
	}
}

func TestCommitKnowledgeRerankScoresIsAtomicOnIncompleteResults(t *testing.T) {
	original := 0.4
	records := []scoredKnowledgeRecord{
		{Record: dto.HitTestingRecordResponse{Score: original, Segment: dto.SegmentResponse{ID: "one"}}},
		{Record: dto.HitTestingRecordResponse{Score: 0.3, Segment: dto.SegmentResponse{ID: "two"}}},
	}

	if commitKnowledgeRerankScores(records, []int{0, 1}, []float64{0.9, 0.8}, []bool{true, false}) {
		t.Fatal("commitKnowledgeRerankScores() = true, want false")
	}
	if records[0].Record.Score != original || records[0].Record.RetrievalSource != nil {
		t.Fatalf("incomplete rerank mutated records: %#v", records)
	}
}

type recordingKnowledgeReranker struct {
	calls                int
	totalDocuments       int
	omitLastResultOnCall int
}

func (r *recordingKnowledgeReranker) Rerank(_ context.Context, _ string, documents []string, _ *float64, _ *int) ([]retrieval.GatewayRerankResult, error) {
	r.calls++
	r.totalDocuments += len(documents)
	limit := len(documents)
	if r.calls == r.omitLastResultOnCall {
		limit--
	}
	results := make([]retrieval.GatewayRerankResult, 0, limit)
	for index := 0; index < limit; index++ {
		results = append(results, retrieval.GatewayRerankResult{Index: index, Score: float64(index)})
	}
	return results, nil
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

func expectAccessibleKnowledgeWorkspaces(mock sqlmock.Sqlmock, workspaceIDs ...string) {
	rows := sqlmock.NewRows([]string{"id", "role", "role_id", "permissions", "permission_source"})
	for _, workspaceID := range workspaceIDs {
		rows.AddRow(workspaceID, workspace_model.WorkspaceRoleMember, workspace_model.WorkspaceBuiltinRoleMemberID, `["knowledge_base.document.view"]`, workspace_model.WorkspaceMemberPermissionSourceDirect)
	}
	mock.ExpectQuery(`SELECT workspaces\.id, workspace_members\.role, workspace_members\.role_id, COALESCE\(workspace_members\.permissions::text, ''\) AS permissions, workspace_members\.permission_source FROM "workspaces" JOIN workspace_members ON workspace_members\.workspace_id = workspaces\.id WHERE workspaces\.organization_id = \$1 AND workspace_members\.account_id = \$2`).
		WillReturnRows(rows)
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
