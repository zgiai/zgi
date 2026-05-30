package workflow

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	graphentities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type mockWorkflowFileService struct {
	getFileByIDFn  func(ctx context.Context, fileID string) (*dto.UploadFile, error)
	getFileFn      func(ctx context.Context, fileID string) (string, error)
	downloadFileFn func(ctx context.Context, fileID string) ([]byte, error)
}

func (m *mockWorkflowFileService) GetUploadConfig() *interfaces.FileUploadConfigResponse {
	return nil
}

func (m *mockWorkflowFileService) UploadFile(ctx context.Context, filename string, content []byte, mimeType string, userID, tenantID string, userRole filemodel.CreatedByRole, source *interfaces.FileSource, teamTenantID *string, isTemporary bool, isIcon bool) (*dto.UploadFile, error) {
	return nil, nil
}

func (m *mockWorkflowFileService) GetFilePreview(ctx context.Context, fileID string) (string, error) {
	return "", nil
}

func (m *mockWorkflowFileService) GetFilePreviewWithOCR(ctx context.Context, fileID string, enableOCR bool) (string, error) {
	return "", nil
}

func (m *mockWorkflowFileService) GetFile(ctx context.Context, fileID string) (string, error) {
	if m.getFileFn != nil {
		return m.getFileFn(ctx, fileID)
	}
	return "", nil
}

func (m *mockWorkflowFileService) ExtractFileWithSetting(ctx context.Context, fileID string, _ interfaces.FileExtractionSetting) (string, error) {
	return m.GetFile(ctx, fileID)
}

func (m *mockWorkflowFileService) GetSupportedFileTypes() []string {
	return nil
}

func (m *mockWorkflowFileService) IsFileSizeWithinLimit(extension string, fileSize int64) bool {
	return true
}

func (m *mockWorkflowFileService) ParseFileContent(ctx context.Context, uploadFileID string) {}

func (m *mockWorkflowFileService) GetFileByID(ctx context.Context, fileID string) (*dto.UploadFile, error) {
	if m.getFileByIDFn != nil {
		return m.getFileByIDFn(ctx, fileID)
	}
	return nil, nil
}

func (m *mockWorkflowFileService) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	if m.downloadFileFn != nil {
		return m.downloadFileFn(ctx, fileID)
	}
	return nil, nil
}

func (m *mockWorkflowFileService) ListFiles(ctx context.Context, tenantID, accountID string, req *dto.FileListRequest, visibleWorkspaceIDs []string) (*dto.FileListResponse, error) {
	return nil, nil
}

func (m *mockWorkflowFileService) ListArchivedFiles(ctx context.Context, tenantID, accountID string, req *dto.FileListRequest, visibleWorkspaceIDs []string) (*dto.FileListResponse, error) {
	return nil, nil
}

func (m *mockWorkflowFileService) GetStorageUsage(ctx context.Context, tenantID string) (int64, error) {
	return 0, nil
}

func (m *mockWorkflowFileService) DeleteFiles(ctx context.Context, fileIDs []string) error {
	return nil
}

func (m *mockWorkflowFileService) UpdateContentText(ctx context.Context, fileID string, contentText string) error {
	return nil
}

func (m *mockWorkflowFileService) CleanupExpiredTemporaryFiles(ctx context.Context, ttl time.Duration) (int64, error) {
	return 0, nil
}

func (m *mockWorkflowFileService) GetFileURL(ctx context.Context, fileID string) (string, error) {
	return "", nil
}

func TestProcessAllFileInputs_TreatsImageExtensionAsImageEvenWhenDeclaredDocument(t *testing.T) {
	getFileCalled := false
	handler := &WorkflowHandler{
		fileService: &mockWorkflowFileService{
			getFileByIDFn: func(ctx context.Context, fileID string) (*dto.UploadFile, error) {
				return &dto.UploadFile{
					ID:        fileID,
					Name:      "paper.jpg",
					Extension: "jpg",
					MimeType:  "image/jpeg",
					Size:      1024,
				}, nil
			},
			getFileFn: func(ctx context.Context, fileID string) (string, error) {
				getFileCalled = true
				return "unexpected text extraction", nil
			},
		},
	}

	inputs := map[string]interface{}{
		"query": map[string]interface{}{
			"type":            "document",
			"transfer_method": "local_file",
			"upload_file_id":  "file-1",
		},
	}

	processed := handler.processAllFileInputs(context.Background(), inputs, "tenant-1", "app-1")

	query, ok := processed["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected processed query to stay a file object, got %T", processed["query"])
	}
	if got := fmt.Sprint(query["type"]); got != "image" {
		t.Fatalf("expected processed query type to be image, got %v", got)
	}
	if getFileCalled {
		t.Fatalf("expected image input to bypass text extraction")
	}
}

func TestHydrateInputs_FillsEmptyFileURLsFromSourceURL(t *testing.T) {
	executor := &WorkflowExecutor{
		fileService: &mockWorkflowFileService{
			getFileByIDFn: func(ctx context.Context, fileID string) (*dto.UploadFile, error) {
				return &dto.UploadFile{
					ID:        fileID,
					Name:      "paper.jpg",
					Extension: "jpg",
					MimeType:  "image/jpeg",
					Size:      1024,
					SourceURL: "https://example.com/files/file-1.jpg",
				}, nil
			},
		},
	}

	inputs := map[string]interface{}{
		"query": map[string]interface{}{
			"type":            "image",
			"transfer_method": "local_file",
			"upload_file_id":  "file-1",
			"url":             "",
			"remote_url":      "",
		},
	}

	hydrated, err := executor.HydrateInputs(context.Background(), inputs)
	if err != nil {
		t.Fatalf("HydrateInputs returned error: %v", err)
	}

	query, ok := hydrated["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected hydrated query to be file map, got %T", hydrated["query"])
	}
	if got := fmt.Sprint(query["url"]); got != "https://example.com/files/file-1.jpg" {
		t.Fatalf("expected hydrated url to be filled from source url, got %q", got)
	}
	if got := fmt.Sprint(query["remote_url"]); got != "https://example.com/files/file-1.jpg" {
		t.Fatalf("expected hydrated remote_url to be filled from source url, got %q", got)
	}
}

func TestApplyProcessedInputs_ReplacesOriginalRequestInputs(t *testing.T) {
	req := &dto.DraftWorkflowRunRequest{
		Inputs: map[string]interface{}{
			"query": map[string]interface{}{
				"type": "document",
			},
		},
	}

	processedInputs := map[string]interface{}{
		"query": map[string]interface{}{
			"type": "image",
			"url":  "https://example.com/files/file-1.jpg",
		},
	}

	applyProcessedInputs(req, processedInputs)

	query, ok := req.Inputs["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected request input query to be a file map, got %T", req.Inputs["query"])
	}
	if got := fmt.Sprint(query["type"]); got != "image" {
		t.Fatalf("expected processed inputs to replace original type, got %q", got)
	}
	if got := fmt.Sprint(query["url"]); got != "https://example.com/files/file-1.jpg" {
		t.Fatalf("expected processed inputs to replace original url, got %q", got)
	}
}

func TestGetNodeInputs_UsesNestedSelectorForLLMContext(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"answer", "payload"}, map[string]any{
		"text": "nested context",
	})

	inputs := handler.getNodeInputs(
		"llm-1",
		"llm",
		map[string]interface{}{
			"context": map[string]interface{}{
				"enabled": true,
				"variable_selector": []interface{}{
					map[string]interface{}{
						"variable":       "context",
						"value_selector": []interface{}{"answer", "payload", "text"},
					},
				},
			},
		},
		map[string]interface{}{},
		map[string]interface{}{},
		variablePool,
	)

	if got := inputs["context"]; got != "nested context" {
		t.Fatalf("inputs[context] = %#v, want %q", got, "nested context")
	}
}

func TestGetNodeInputs_UsesNestedSelectorForEndOutputs(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"answer", "payload"}, map[string]any{
		"text": "nested end output",
	})

	inputs := handler.getNodeInputs(
		"end-1",
		"end",
		map[string]interface{}{
			"outputs": []interface{}{
				map[string]interface{}{
					"variable":       "result",
					"value_selector": []interface{}{"answer", "payload", "text"},
				},
			},
		},
		map[string]interface{}{},
		map[string]interface{}{},
		variablePool,
	)

	if got := inputs["result"]; got != "nested end output" {
		t.Fatalf("inputs[result] = %#v, want %q", got, "nested end output")
	}
}

func TestGetNodeInputs_EndDoesNotInjectSystemInputs(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"answer", "text"}, "done")

	inputs := handler.getNodeInputs(
		"end-1",
		"end",
		map[string]interface{}{
			"outputs": []interface{}{
				map[string]interface{}{
					"variable":       "result",
					"value_selector": []interface{}{"answer", "text"},
				},
			},
		},
		map[string]interface{}{
			"sys.user_id":     "u1",
			"sys.workflow_id": "workflow-1",
			"tenant_id":       "tenant-1",
		},
		map[string]interface{}{},
		variablePool,
	)

	filtered := FilterFrontendInputs("end", inputs)
	if got := filtered["result"]; got != "done" {
		t.Fatalf("result = %#v, want done", got)
	}
	for _, key := range []string{"sys.user_id", "sys.workflow_id", "tenant_id"} {
		if _, exists := filtered[key]; exists {
			t.Fatalf("%s should not be exposed in end node inputs: %#v", key, filtered)
		}
	}
}

func TestGetNodeInputs_HTTPRequestBuildsFrontendSnapshot(t *testing.T) {
	handler := &WorkflowHandler{}

	inputs := handler.getNodeInputs(
		"http-1",
		"http-request",
		map[string]interface{}{
			"url":    "http://baidu.com",
			"method": "GET",
			"headers": []interface{}{
				map[string]interface{}{"key": "X-Test", "value": "ok"},
			},
			"params": "q: zgi",
			"body": map[string]interface{}{
				"type": "none",
			},
			"authorization": map[string]interface{}{
				"type": "no-auth",
			},
			"retry_config": map[string]interface{}{"max_times": 3},
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{"query": "hello"},
		graphentities.NewVariablePool(),
	)

	filtered := FilterFrontendInputs("http-request", inputs)
	if len(filtered) != 5 {
		t.Fatalf("filtered http inputs len = %d, want 5: %#v", len(filtered), filtered)
	}
	if got := filtered["url"]; got != "http://baidu.com" {
		t.Fatalf("filtered url = %#v, want http://baidu.com", got)
	}
	if got := filtered["method"]; got != "GET" {
		t.Fatalf("filtered method = %#v, want GET", got)
	}
	header, ok := filtered["header"].(map[string]interface{})
	if !ok || header["X-Test"] != "ok" {
		t.Fatalf("filtered header = %#v, want X-Test", filtered["header"])
	}
	param, ok := filtered["param"].(map[string]interface{})
	if !ok || param["q"] != "zgi" {
		t.Fatalf("filtered param = %#v, want q", filtered["param"])
	}
	if _, exists := filtered["body"]; exists {
		t.Fatalf("filtered body should be removed: %#v", filtered)
	}
	if filtered["auth"] != nil {
		t.Fatalf("filtered auth = %#v, want nil", filtered["auth"])
	}
	if _, exists := filtered["retry_config"]; exists {
		t.Fatalf("retry_config should be removed: %#v", filtered)
	}
	if _, exists := filtered["sys.user_id"]; exists {
		t.Fatalf("sys.user_id should be removed: %#v", filtered)
	}
}

func TestGetNodeInputs_LoopBuildsFrontendSnapshot(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"start", "limit"}, 5)

	inputs := handler.getNodeInputs(
		"loop-1",
		"loop",
		map[string]interface{}{
			"loop_count": 3,
			"loop_variables": []interface{}{
				map[string]interface{}{
					"label":      "i",
					"var_type":   "integer",
					"value_type": "constant",
					"value":      0,
				},
				map[string]interface{}{
					"label":      "limit",
					"var_type":   "integer",
					"value_type": "variable",
					"value":      []interface{}{"start", "limit"},
				},
			},
			"break_conditions": []interface{}{
				map[string]interface{}{"variable_selector": []interface{}{"loop-1", "i"}, "comparison_operator": ">="},
			},
			"parallel_nums": 4,
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{"query": "hello"},
		variablePool,
	)

	filtered := FilterFrontendInputs("loop", inputs)
	if len(filtered) != 3 {
		t.Fatalf("filtered loop inputs len = %d, want 3: %#v", len(filtered), filtered)
	}
	if got := filtered["loop_count"]; got != 3 {
		t.Fatalf("filtered loop_count = %#v, want 3", got)
	}
	loopVariables, ok := filtered["loop_variables"].(map[string]interface{})
	if !ok {
		t.Fatalf("filtered loop_variables = %T, want map[string]interface{}", filtered["loop_variables"])
	}
	if got := loopVariables["i"]; got != 0 {
		t.Fatalf("loop variable i = %#v, want 0", got)
	}
	if got := fmt.Sprint(loopVariables["limit"]); got != "5" {
		t.Fatalf("loop variable limit = %#v, want 5", got)
	}
	if _, exists := filtered["parallel_nums"]; exists {
		t.Fatalf("parallel_nums should be removed: %#v", filtered)
	}
}

func TestGetNodeInputs_IterationBuildsFrontendSnapshot(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"start", "filelist"}, []interface{}{"a", "b"})
	variablePool.Add([]string{"llm", "text"}, "done")

	inputs := handler.getNodeInputs(
		"iter-1",
		"iteration",
		map[string]interface{}{
			"iterator_selector": []interface{}{"start", "filelist"},
			"output_selector":   []interface{}{"llm", "text"},
			"is_parallel":       true,
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{},
		variablePool,
	)

	filtered := FilterFrontendInputs("iteration", inputs)
	if len(filtered) != 2 {
		t.Fatalf("filtered iteration inputs len = %d, want 2: %#v", len(filtered), filtered)
	}
	if got := fmt.Sprint(filtered["filelist"]); got != "[a b]" {
		t.Fatalf("filelist = %#v, want [a b]", filtered["filelist"])
	}
	if got := filtered["text"]; got != "done" {
		t.Fatalf("text = %#v, want done", got)
	}
	if _, exists := filtered["is_parallel"]; exists {
		t.Fatalf("is_parallel should be removed: %#v", filtered)
	}
	for _, key := range []string{"iterator_selector", "iterator_value", "output_selector"} {
		if _, exists := filtered[key]; exists {
			t.Fatalf("%s should be removed: %#v", key, filtered)
		}
	}
}

func TestGetNodeInputs_VariableAssignerBuildsFrontendSnapshot(t *testing.T) {
	handler := &WorkflowHandler{}
	items := []interface{}{
		map[string]interface{}{
			"variable_selector": []interface{}{"conversation", "name"},
			"input_type":        "constant",
			"operation":         "over-write",
			"value":             "Alice",
		},
	}

	inputs := handler.getNodeInputs(
		"assigner-1",
		"assigner",
		map[string]interface{}{
			"items":             items,
			"updated_variables": map[string]interface{}{"name": "Alice"},
			"sys.user_id":       "u1",
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{},
		graphentities.NewVariablePool(),
	)

	filtered := FilterFrontendInputs("assigner", inputs)
	if len(filtered) != 1 {
		t.Fatalf("filtered assigner inputs len = %d, want 1: %#v", len(filtered), filtered)
	}
	if !reflect.DeepEqual(filtered["items"], items) {
		t.Fatalf("items = %#v, want variable operation rules", filtered["items"])
	}
	if _, exists := filtered["updated_variables"]; exists {
		t.Fatalf("updated_variables should be removed: %#v", filtered)
	}
}

func TestGetNodeInputs_CallDatabaseBuildsFrontendSnapshot(t *testing.T) {
	handler := &WorkflowHandler{}

	inputs := handler.getNodeInputs(
		"db-1",
		"call-database",
		map[string]interface{}{
			"data_source":     map[string]interface{}{"id": "ds-1", "name": "main"},
			"table_selection": []interface{}{map[string]interface{}{"name": "users"}},
			"manual_sql":      "select * from users",
			"execution":       map[string]interface{}{"timeout_seconds": 30},
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{},
		graphentities.NewVariablePool(),
	)

	filtered := FilterFrontendInputs("call-database", inputs)
	if len(filtered) != 1 {
		t.Fatalf("filtered call-database inputs len = %d, want 1: %#v", len(filtered), filtered)
	}
	if got := filtered["sql"]; got != "select * from users" {
		t.Fatalf("sql = %#v, want manual sql", got)
	}
	if _, exists := filtered["data_source"]; exists {
		t.Fatalf("data_source should be removed: %#v", filtered)
	}
	if _, exists := filtered["execution"]; exists {
		t.Fatalf("execution should be removed: %#v", filtered)
	}
	if _, exists := filtered["table_selection"]; exists {
		t.Fatalf("table_selection should be removed: %#v", filtered)
	}
	if _, exists := filtered["schema_tables"]; exists {
		t.Fatalf("schema_tables should be removed: %#v", filtered)
	}
}

func TestGetNodeInputs_CallDatabaseReadsNestedDataConfig(t *testing.T) {
	handler := &WorkflowHandler{}

	inputs := handler.getNodeInputs(
		"db-1",
		"call-database",
		map[string]interface{}{
			"data": map[string]interface{}{
				"data_source":     map[string]interface{}{"id": "ds-1", "name": "main"},
				"table_selection": []interface{}{map[string]interface{}{"name": "users"}},
				"sql":             "select * from users",
				"execution":       map[string]interface{}{"timeout_seconds": 30},
			},
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{},
		graphentities.NewVariablePool(),
	)

	filtered := FilterFrontendInputs("call-database", inputs)
	if got := filtered["sql"]; got != "select * from users" {
		t.Fatalf("sql = %#v, want nested SQL", got)
	}
	if _, exists := filtered["data_source"]; exists {
		t.Fatalf("data_source should be removed from nested data config: %#v", filtered)
	}
	if _, exists := filtered["table_selection"]; exists {
		t.Fatalf("table_selection should be removed from nested data config: %#v", filtered)
	}
	if _, exists := filtered["schema_tables"]; exists {
		t.Fatalf("schema_tables should be removed from nested data config: %#v", filtered)
	}
}

func TestGetNodeInputs_HTTPRequestReadsNestedDataConfig(t *testing.T) {
	handler := &WorkflowHandler{}

	inputs := handler.getNodeInputs(
		"http-1",
		"http-request",
		map[string]interface{}{
			"data": map[string]interface{}{
				"url":     "https://example.com",
				"method":  "POST",
				"headers": []interface{}{map[string]interface{}{"key": "X-Test", "value": "1"}},
				"params":  []interface{}{map[string]interface{}{"key": "q", "value": "cat"}},
			},
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{},
		graphentities.NewVariablePool(),
	)

	filtered := FilterFrontendInputs("http-request", inputs)
	if got := filtered["url"]; got != "https://example.com" {
		t.Fatalf("url = %#v, want nested URL", got)
	}
	if got := filtered["method"]; got != "POST" {
		t.Fatalf("method = %#v, want POST", got)
	}
	header := filtered["header"].(map[string]interface{})
	if got := header["X-Test"]; got != "1" {
		t.Fatalf("header X-Test = %#v, want 1", got)
	}
	param := filtered["param"].(map[string]interface{})
	if got := param["q"]; got != "cat" {
		t.Fatalf("param q = %#v, want cat", got)
	}
}

func TestGetNodeInputs_PromptNodesBuildFrontendSnapshots(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"start", "query"}, "show users")
	variablePool.Add([]string{"start", "subject"}, "cat")

	tests := []struct {
		name     string
		nodeType string
		nodeData map[string]interface{}
		wantKey  string
		want     interface{}
	}{
		{
			name:     "sql generator",
			nodeType: "sql-generator",
			nodeData: map[string]interface{}{
				"prompt": map[string]interface{}{
					"user": "Generate SQL for {{#start.query#}}",
				},
				"model": map[string]interface{}{"name": "hidden"},
			},
			wantKey: "prompt",
			want:    "Generate SQL for show users",
		},
		{
			name:     "image generator",
			nodeType: "image-gen",
			nodeData: map[string]interface{}{
				"prompt": "Draw {{#start.subject#}}",
				"prompt_config": map[string]interface{}{
					"template_variables": []interface{}{
						map[string]interface{}{
							"variable":       "subject",
							"value_selector": []interface{}{"start", "subject"},
						},
					},
				},
				"model": map[string]interface{}{"name": "hidden"},
			},
			wantKey: "subject",
			want:    "cat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputs := handler.getNodeInputs(
				tt.nodeType+"-1",
				tt.nodeType,
				tt.nodeData,
				map[string]interface{}{"sys.user_id": "u1"},
				map[string]interface{}{},
				variablePool,
			)

			filtered := FilterFrontendInputs(tt.nodeType, inputs)
			if tt.nodeType == "sql-generator" {
				prompt := filtered["prompt"].(map[string]interface{})
				if got := prompt["user"]; got != tt.want {
					t.Fatalf("prompt user = %#v, want %#v", got, tt.want)
				}
				if _, exists := filtered["prompt_variables"]; exists {
					t.Fatalf("sql-generator prompt_variables should be removed: %#v", filtered)
				}
			} else {
				promptVariables, ok := filtered["prompt_variables"].(map[string]interface{})
				if !ok {
					t.Fatalf("prompt_variables = %T, want map[string]interface{}", filtered["prompt_variables"])
				}
				if got := promptVariables[tt.wantKey]; got != tt.want {
					t.Fatalf("prompt variable %s = %#v, want %#v", tt.wantKey, got, tt.want)
				}
			}
			if _, exists := filtered["model"]; exists {
				t.Fatalf("model should be removed: %#v", filtered)
			}
		})
	}
}

func TestRenderTemplateTextOnlyReplacesPlaceholders(t *testing.T) {
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"start", "query"}, "actual query")

	got := renderTemplateText(
		"literal #start.query# should stay, compact={{#start.query#}}, spaced={{ #start.query# }}",
		variablePool,
	)
	want := "literal #start.query# should stay, compact=actual query, spaced=actual query"
	if got != want {
		t.Fatalf("renderTemplateText() = %q, want %q", got, want)
	}
}

func TestGetNodeInputs_SQLGeneratorReadsNestedDataConfig(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"start", "query"}, "show users")

	inputs := handler.getNodeInputs(
		"sql-generator-1",
		"sql-generator",
		map[string]interface{}{
			"data": map[string]interface{}{
				"prompt": map[string]interface{}{
					"system": "You are SQL expert",
					"user":   "Generate SQL for {{#start.query#}}",
				},
				"data_source": map[string]interface{}{
					"source": map[string]interface{}{"id": "ds-1", "name": "main"},
					"tables": []interface{}{map[string]interface{}{"name": "users"}},
				},
				"table_schema": "users(id, name)",
				"model":        map[string]interface{}{"name": "hidden"},
			},
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{},
		variablePool,
	)

	filtered := FilterFrontendInputs("sql-generator", inputs)
	prompt, ok := filtered["prompt"].(map[string]interface{})
	if !ok {
		t.Fatalf("prompt = %T, want map[string]interface{}", filtered["prompt"])
	}
	if got := prompt["user"]; got != "Generate SQL for show users" {
		t.Fatalf("prompt user = %#v, want rendered prompt", got)
	}
	if _, exists := filtered["prompt_variables"]; exists {
		t.Fatalf("prompt_variables should be removed from sql-generator inputs: %#v", filtered)
	}
	for _, key := range []string{"data_source", "schema_tables", "table_schema"} {
		if _, exists := filtered[key]; exists {
			t.Fatalf("%s should be removed from sql-generator inputs: %#v", key, filtered)
		}
	}
}

func TestGetNodeInputs_ImageGenReadsNestedDataConfig(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"start", "subject"}, "cat")

	inputs := handler.getNodeInputs(
		"image-gen-1",
		"image-gen",
		map[string]interface{}{
			"data": map[string]interface{}{
				"prompt": "Draw {{#start.subject#}}",
			},
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{},
		variablePool,
	)

	filtered := FilterFrontendInputs("image-gen", inputs)
	if got := filtered["prompt"]; got != "Draw cat" {
		t.Fatalf("prompt = %#v, want rendered nested prompt", got)
	}
	promptVariables := filtered["prompt_variables"].(map[string]interface{})
	if got := promptVariables["subject"]; got != "cat" {
		t.Fatalf("prompt variable subject = %#v, want cat", got)
	}
}

func TestGetNodeInputs_LLMReadsNestedDataConfig(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"start", "query"}, "hello")
	variablePool.Add([]string{"retrieval", "text"}, "context")

	inputs := handler.getNodeInputs(
		"llm-1",
		"llm",
		map[string]interface{}{
			"data": map[string]interface{}{
				"prompt": map[string]interface{}{
					"user": "Answer {{#start.query#}}",
				},
				"context": map[string]interface{}{
					"enabled":           true,
					"variable_selector": []interface{}{map[string]interface{}{"variable": "ctx", "value_selector": []interface{}{"retrieval", "text"}}},
				},
			},
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{},
		variablePool,
	)

	filtered := FilterFrontendInputs("llm", inputs)
	prompt := filtered["prompt"].(map[string]interface{})
	if got := prompt["user"]; got != "Answer hello" {
		t.Fatalf("prompt user = %#v, want rendered nested prompt", got)
	}
	if got := filtered["ctx"]; got != "context" {
		t.Fatalf("ctx = %#v, want context", got)
	}
}

func TestGetNodeInputs_LoopReadsNestedDataConfig(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"start", "limit"}, 5)

	inputs := handler.getNodeInputs(
		"loop-1",
		"loop",
		map[string]interface{}{
			"data": map[string]interface{}{
				"loop_count": 3,
				"loop_variables": []interface{}{
					map[string]interface{}{"label": "limit", "value_type": "variable", "value": []interface{}{"start", "limit"}},
				},
				"break_conditions": []interface{}{},
			},
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{},
		variablePool,
	)

	filtered := FilterFrontendInputs("loop", inputs)
	if got := filtered["loop_count"]; got != 3 {
		t.Fatalf("loop_count = %#v, want 3", got)
	}
	loopVariables := filtered["loop_variables"].(map[string]interface{})
	if got := fmt.Sprint(loopVariables["limit"]); got != "5" {
		t.Fatalf("loop variable limit = %#v, want 5", got)
	}
	if got := filtered["break_conditions"]; got == nil {
		t.Fatalf("break_conditions should be kept from nested data config: %#v", filtered)
	}
}

func TestGetNodeInputs_AnswerReadsNestedDataConfig(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"llm", "text"}, "final answer")

	inputs := handler.getNodeInputs(
		"answer-1",
		"answer",
		map[string]interface{}{
			"data": map[string]interface{}{
				"answer": "Result: {{#llm.text#}}",
			},
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{},
		variablePool,
	)

	filtered := FilterFrontendInputs("answer", inputs)
	if got := filtered["text"]; got != "final answer" {
		t.Fatalf("text = %#v, want final answer", got)
	}
	if _, exists := filtered["llm.text"]; exists {
		t.Fatalf("llm.text should use display key text: %#v", filtered)
	}
}

func TestGetNodeInputs_AnswerBuildsFrontendSnapshot(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"llm", "text"}, "final answer")

	inputs := handler.getNodeInputs(
		"answer-1",
		"answer",
		map[string]interface{}{
			"answer":       "Result: {{#llm.text#}}",
			"streaming":    true,
			"model_config": map[string]interface{}{"name": "hidden"},
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{},
		variablePool,
	)

	filtered := FilterFrontendInputs("answer", inputs)
	if len(filtered) != 1 {
		t.Fatalf("filtered answer inputs len = %d, want 1: %#v", len(filtered), filtered)
	}
	if got := filtered["text"]; got != "final answer" {
		t.Fatalf("text = %#v, want final answer", got)
	}
	if _, exists := filtered["llm.text"]; exists {
		t.Fatalf("llm.text should use display key text: %#v", filtered)
	}
}

func TestGetNodeInputs_AnswerUsesSelectorLeafVariableName(t *testing.T) {
	handler := &WorkflowHandler{}
	variablePool := graphentities.NewVariablePool()
	variablePool.Add([]string{"1779034444176_r5nt1", "body"}, "request body")

	inputs := handler.getNodeInputs(
		"answer-1",
		"answer",
		map[string]interface{}{
			"answer": "Result: {{#1779034444176_r5nt1.body#}}",
		},
		map[string]interface{}{"sys.user_id": "u1"},
		map[string]interface{}{},
		variablePool,
	)

	filtered := FilterFrontendInputs("answer", inputs)
	if got := filtered["body"]; got != "request body" {
		t.Fatalf("body = %#v, want request body", got)
	}
	if _, exists := filtered["1779034444176_r5nt1.body"]; exists {
		t.Fatalf("node-prefixed variable key should be removed: %#v", filtered)
	}
}
