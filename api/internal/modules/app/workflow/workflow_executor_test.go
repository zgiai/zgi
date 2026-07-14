package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	llmClient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmAdapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type mockWorkflowLLMClient struct {
	appChatStreamFn func(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error)
}

func normalizeWorkflowTestValue(t *testing.T, value any) any {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("failed to marshal test value: %v", err)
	}

	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		t.Fatalf("failed to unmarshal test value: %v", err)
	}

	return normalized
}

func TestGraphMaxConcurrencyReadsNestedGraphConfig(t *testing.T) {
	graphConfig := map[string]interface{}{
		"config": map[string]interface{}{
			"parallel_nums": float64(1),
		},
	}

	if got := graphMaxConcurrency(graphConfig); got != 1 {
		t.Fatalf("graphMaxConcurrency() = %d, want 1", got)
	}
}

func TestGraphMaxConcurrencyPrefersRootConfig(t *testing.T) {
	graphConfig := map[string]interface{}{
		"parallel_nums": 2,
		"config": map[string]interface{}{
			"parallel_nums": 1,
		},
	}

	if got := graphMaxConcurrency(graphConfig); got != 2 {
		t.Fatalf("graphMaxConcurrency() = %d, want 2", got)
	}
}

func TestWorkflowExecutorGetNodeTypeSupportsRegisteredNodeTypes(t *testing.T) {
	executor := &WorkflowExecutor{}

	tests := []struct {
		name   string
		config map[string]any
		want   shared.NodeType
	}{
		{
			name:   "call database",
			config: map[string]any{"data": map[string]any{"type": "call-database"}},
			want:   shared.CallDatabase,
		},
		{
			name:   "sql generator",
			config: map[string]any{"data": map[string]any{"type": "sql-generator"}},
			want:   shared.SQLGenerator,
		},
		{
			name:   "notification sms",
			config: map[string]any{"data": map[string]any{"type": "notification-sms"}},
			want:   shared.NotificationSMS,
		},
		{
			name:   "question answer",
			config: map[string]any{"data": map[string]any{"type": "question-answer"}},
			want:   shared.QuestionAnswer,
		},
		{
			name:   "http request legacy alias",
			config: map[string]any{"data": map[string]any{"type": "http_request"}},
			want:   shared.HTTPRequest,
		},
		{
			name:   "top level fallback",
			config: map[string]any{"type": "json-parser"},
			want:   shared.JSONParser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := executor.getNodeType(tt.config)
			if err != nil {
				t.Fatalf("getNodeType() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("getNodeType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWorkflowExecutorGetNodeTypeRejectsUnsupportedNodeType(t *testing.T) {
	executor := &WorkflowExecutor{}

	_, err := executor.getNodeType(map[string]any{
		"data": map[string]any{"type": "variable-assigner"},
	})
	if err == nil {
		t.Fatalf("getNodeType() error = nil, want unsupported node type error")
	}
	if !strings.Contains(err.Error(), `unsupported node type "variable-assigner"`) {
		t.Fatalf("getNodeType() error = %q, want unsupported node type", err.Error())
	}
}

func TestShouldSeedWorkflowNodeInput_SkipsQuestionAnswerInputs(t *testing.T) {
	for _, key := range []string{"question", "answer", "answers", "choices", "query", "question_answer_option_id"} {
		if shouldSeedWorkflowNodeInput(shared.QuestionAnswer, key) {
			t.Fatalf("shouldSeedWorkflowNodeInput(question-answer, %q) = true, want false", key)
		}
	}
}

func TestShouldSeedWorkflowNodeInput_PreservesExistingRules(t *testing.T) {
	if !shouldSeedWorkflowNodeInput(shared.Start, "sys.workflow_run_id") {
		t.Fatal("start node system input should still be seeded")
	}
	if shouldSeedWorkflowNodeInput(shared.LLM, "sys.workflow_run_id") {
		t.Fatal("non-start system workflow run id should not be seeded")
	}
	if !shouldSeedWorkflowNodeInput(shared.LLM, "question") {
		t.Fatal("non-question-answer node input should still be seeded")
	}
}

func (m *mockWorkflowLLMClient) Chat(ctx context.Context, organizationID string, req *llmAdapter.ChatRequest) (*llmAdapter.ChatResponse, error) {
	return nil, nil
}

func (m *mockWorkflowLLMClient) ChatStream(ctx context.Context, organizationID string, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error) {
	return nil, nil
}

func (m *mockWorkflowLLMClient) CreateResponse(ctx context.Context, organizationID string, req *llmAdapter.CreateResponseRequest) (*llmAdapter.CreateResponseResponse, error) {
	return nil, nil
}

func (m *mockWorkflowLLMClient) Embed(ctx context.Context, organizationID string, req *llmAdapter.EmbeddingsRequest) (*llmAdapter.EmbeddingsResponse, error) {
	return nil, nil
}

func (m *mockWorkflowLLMClient) CreateImage(ctx context.Context, organizationID string, req *llmAdapter.ImageRequest) (*llmAdapter.ImageResponse, error) {
	return nil, nil
}

func (m *mockWorkflowLLMClient) Rerank(ctx context.Context, organizationID string, req *llmAdapter.RerankRequest) (*llmAdapter.RerankResponse, error) {
	return nil, nil
}

func (m *mockWorkflowLLMClient) AppChat(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (*llmAdapter.ChatResponse, error) {
	return nil, nil
}

func (m *mockWorkflowLLMClient) AppChatStream(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error) {
	if m.appChatStreamFn != nil {
		return m.appChatStreamFn(ctx, appCtx, req)
	}
	return nil, nil
}

func (m *mockWorkflowLLMClient) AppCreateResponse(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.CreateResponseRequest) (*llmAdapter.CreateResponseResponse, error) {
	return nil, nil
}

func (m *mockWorkflowLLMClient) AppEmbed(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.EmbeddingsRequest) (*llmAdapter.EmbeddingsResponse, error) {
	return nil, nil
}

func (m *mockWorkflowLLMClient) AppCreateImage(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ImageRequest) (*llmAdapter.ImageResponse, error) {
	return nil, nil
}

func (m *mockWorkflowLLMClient) AppRerank(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.RerankRequest) (*llmAdapter.RerankResponse, error) {
	return nil, nil
}

func TestCreateVariablePoolWithVars_PreservesOrganizationID(t *testing.T) {
	executor := &WorkflowExecutor{}

	variablePool := executor.createVariablePoolWithVars(map[string]any{
		"sys.workspace_id":    "ws-1",
		"sys.organization_id": "org-1",
	}, nil, nil)

	if variablePool == nil || variablePool.SystemVariables == nil {
		t.Fatalf("variablePool or system variables is nil")
	}
	if variablePool.SystemVariables.WorkspaceID != "ws-1" {
		t.Fatalf("workspaceID = %q, want %q", variablePool.SystemVariables.WorkspaceID, "ws-1")
	}
	if variablePool.SystemVariables.OrganizationID != "org-1" {
		t.Fatalf("organizationID = %q, want %q", variablePool.SystemVariables.OrganizationID, "org-1")
	}
}

func TestCreateVariablePoolWithVars_RegistersSystemFiles(t *testing.T) {
	executor := &WorkflowExecutor{}
	files := []interface{}{
		map[string]interface{}{
			"type":            "document",
			"transfer_method": "local_file",
			"upload_file_id":  "file-1",
			"name":            "contract.docx",
		},
	}

	variablePool := executor.createVariablePoolWithVars(map[string]any{
		"sys.files": files,
	}, nil, nil)

	variable := variablePool.GetWithPath([]string{"sys", "files"})
	if variable == nil {
		t.Fatalf("expected sys.files to be registered in variable pool")
	}
	if got := variable.GetType(); got != shared.SegmentTypeArrayFile {
		t.Fatalf("sys.files type = %s, want %s", got, shared.SegmentTypeArrayFile)
	}
}

func TestCreateVariablePoolWithVars_RegistersHashFilesAsSystemFiles(t *testing.T) {
	executor := &WorkflowExecutor{}
	files := []interface{}{
		map[string]interface{}{
			"type":            "document",
			"transfer_method": "local_file",
			"upload_file_id":  "file-1",
			"name":            "contract.docx",
		},
	}

	variablePool := executor.createVariablePoolWithVars(map[string]any{
		"#files#": files,
	}, nil, nil)

	variable := variablePool.GetWithPath([]string{"sys", "files"})
	if variable == nil {
		t.Fatalf("expected #files# to be registered as sys.files in variable pool")
	}
	if got := variable.GetType(); got != shared.SegmentTypeArrayFile {
		t.Fatalf("sys.files type = %s, want %s", got, shared.SegmentTypeArrayFile)
	}
}

func TestExecuteWorkflowNodeWithCallbacks_VisionOnlyImageInputReachesGateway(t *testing.T) {
	var capturedRequest *llmAdapter.ChatRequest

	previousConfig := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		Server: appconfig.ServerConfig{Mode: "release"},
		Console: appconfig.ConsoleConfig{
			APIURL: "https://api.zgi.im",
		},
		App: appconfig.AppConfig{
			FilesURL:  "https://api.zgi.im",
			SecretKey: "test-secret",
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previousConfig
	})

	fileService := &mockWorkflowFileService{
		getFileByIDFn: func(ctx context.Context, fileID string) (*dto.UploadFile, error) {
			return &dto.UploadFile{
				ID:        fileID,
				Name:      "paper.jpg",
				Extension: "jpg",
				MimeType:  "image/jpeg",
				Size:      1024,
				SourceURL: "https://example.com/files/paper.jpg",
			}, nil
		},
	}

	executor := NewWorkflowExecutor()
	executor.fileService = fileService
	executor.SetLLMClient(&mockWorkflowLLMClient{
		appChatStreamFn: func(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error) {
			capturedRequest = req
			hasSignedPreviewURL := false
			hasPromptText := false

			for _, message := range req.Messages {
				parts, ok := message.Content.([]llmAdapter.MessageContentPart)
				if !ok {
					continue
				}
				for _, part := range parts {
					if part.Type == "image_url" && part.ImageURL != nil &&
						strings.HasPrefix(part.ImageURL.URL, "https://api.zgi.im/console/api/files/file-1/file-preview?") {
						hasSignedPreviewURL = true
					}
					if part.Type == "text" && part.Text == "Analyze the uploaded image or file directly. Use all visible content, including questions, answers, annotations, scores, diagrams, and layout details, to complete the task." {
						hasPromptText = true
					}
				}
			}

			responseText := "数据不足，无法进行诊断。"
			if hasSignedPreviewURL && hasPromptText {
				responseText = "诊断成功"
			}

			ch := make(chan llmAdapter.StreamResponse, 1)
			go func() {
				defer close(ch)
				ch <- llmAdapter.StreamResponse{
					Choices: []llmAdapter.StreamChoice{
						{Delta: llmAdapter.Message{Role: "assistant", Content: responseText}, FinishReason: "stop"},
					},
					Usage: &llmAdapter.Usage{
						PromptTokens:     10,
						CompletionTokens: 2,
						TotalTokens:      12,
					},
					Done: true,
				}
			}()
			return ch, nil
		},
	})

	rawInputs := map[string]any{
		"query": map[string]any{
			"type":            "document",
			"transfer_method": "local_file",
			"upload_file_id":  "file-1",
			"url":             "",
		},
		"sys.workspace_id":    "ws-1",
		"sys.organization_id": "org-1",
		"sys.agent_id":        "app-1",
		"sys.workflow_id":     "workflow-1",
		"sys.user_id":         "user-1",
	}

	hydratedInputs, err := executor.HydrateInputs(context.Background(), rawInputs)
	if err != nil {
		t.Fatalf("HydrateInputs returned error: %v", err)
	}

	sharedVariablePool := executor.createVariablePoolWithVars(hydratedInputs, nil, nil)
	graphConfig := map[string]any{
		"nodes": []any{
			map[string]any{
				"id": "start-node",
				"data": map[string]any{
					"type": "start",
					"variables": []any{
						map[string]any{
							"variable": "query",
							"type":     "file",
						},
					},
				},
			},
			map[string]any{
				"id": "llm-node",
				"data": map[string]any{
					"type": "llm",
					"model": map[string]any{
						"provider":          "openai",
						"name":              "gpt-4o",
						"mode":              "chat",
						"completion_params": map[string]any{},
					},
					"context": map[string]any{
						"enabled": false,
					},
					"prompt_template": []any{
						map[string]any{
							"role": "system",
							"text": "You are a diagnosis assistant.",
						},
					},
					"vision": map[string]any{
						"enabled": true,
						"configs": map[string]any{
							"detail":            "high",
							"variable_selector": []any{"start-node", "query"},
						},
					},
				},
			},
		},
		"edges": []any{
			map[string]any{
				"source": "start-node",
				"target": "llm-node",
			},
		},
	}

	startConfig := map[string]any{
		"id": "start-node",
		"data": map[string]any{
			"type": "start",
			"variables": []any{
				map[string]any{
					"variable": "query",
					"type":     "file",
				},
			},
		},
	}

	if _, err := executor.ExecuteWorkflowNodeWithCallbacks(context.Background(), "start-node", shared.Start, startConfig, hydratedInputs, sharedVariablePool, graphConfig, nil, nil); err != nil {
		t.Fatalf("start node execution failed: %v", err)
	}

	llmConfig := map[string]any{
		"id": "llm-node",
		"data": map[string]any{
			"type": "llm",
			"model": map[string]any{
				"provider":          "openai",
				"name":              "gpt-4o",
				"mode":              "chat",
				"completion_params": map[string]any{},
			},
			"context": map[string]any{
				"enabled": false,
			},
			"prompt_template": []any{
				map[string]any{
					"role": "system",
					"text": "You are a diagnosis assistant.",
				},
			},
			"vision": map[string]any{
				"enabled": true,
				"configs": map[string]any{
					"detail":            "high",
					"variable_selector": []any{"start-node", "query"},
				},
			},
		},
	}

	result, err := executor.ExecuteWorkflowNodeWithCallbacks(context.Background(), "llm-node", shared.LLM, llmConfig, map[string]any{}, sharedVariablePool, graphConfig, nil, nil)
	if err != nil {
		t.Fatalf("llm node execution failed: %v", err)
	}

	if got := result.Outputs["text"]; got != "诊断成功" {
		t.Fatalf("expected llm output to be vision success, got %v", got)
	}
	if got := result.ProcessData["resolved_file_count"]; got != 1 {
		t.Fatalf("expected resolved_file_count=1, got %v", got)
	}
	if got := result.ProcessData["auto_injected_user_prompt"]; got != true {
		t.Fatalf("expected auto_injected_user_prompt=true, got %v", got)
	}
	if got := result.ProcessData["selected_file_transport"]; got != "signed_preview_url" {
		t.Fatalf("expected selected_file_transport=signed_preview_url, got %v", got)
	}
	if got := result.ProcessData["selected_file_url_host"]; got != "api.zgi.im" {
		t.Fatalf("expected selected_file_url_host=api.zgi.im, got %v", got)
	}
	if got := result.ProcessData["selected_file_url_scheme"]; got != "https" {
		t.Fatalf("expected selected_file_url_scheme=https, got %v", got)
	}
	if got := result.ProcessData["selected_file_url_is_public"]; got != true {
		t.Fatalf("expected selected_file_url_is_public=true, got %v", got)
	}
	if got := result.ProcessData["final_prompt_contains_inline_data"]; got != false {
		t.Fatalf("expected final_prompt_contains_inline_data=false, got %v", got)
	}
	if got := result.ProcessData["resolved_model_source"]; got != "node_default" {
		t.Fatalf("expected resolved_model_source=node_default, got %v", got)
	}
	if got := result.ProcessData["resolved_model_provider"]; got != "openai" {
		t.Fatalf("expected resolved_model_provider=openai, got %v", got)
	}
	if got := result.ProcessData["resolved_model_name"]; got != "gpt-4o" {
		t.Fatalf("expected resolved_model_name=gpt-4o, got %v", got)
	}
	if got := result.Metadata[shared.WorkflowNodeExecutionMetadataKey("resolved_model_source")]; got != "node_default" {
		t.Fatalf("expected metadata resolved_model_source=node_default, got %v", got)
	}
	if capturedRequest == nil {
		t.Fatalf("expected gateway request to be captured")
	}

	gatewayRequest, ok := result.ProcessData["llm_gateway_request"].(map[string]any)
	if !ok {
		t.Fatalf("expected llm_gateway_request in process data, got %T", result.ProcessData["llm_gateway_request"])
	}
	if got := gatewayRequest["model"]; got != capturedRequest.Model {
		t.Fatalf("expected llm_gateway_request.model=%q, got %v", capturedRequest.Model, got)
	}
	if !reflect.DeepEqual(gatewayRequest["messages"], normalizeWorkflowTestValue(t, capturedRequest.Messages)) {
		t.Fatalf("expected llm_gateway_request.messages to match gateway request")
	}
	params, ok := gatewayRequest["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected llm_gateway_request.params, got %T", gatewayRequest["params"])
	}
	if !reflect.DeepEqual(params["stop"], normalizeWorkflowTestValue(t, capturedRequest.Stop)) {
		t.Fatalf("expected llm_gateway_request.params.stop to match gateway request")
	}
	if _, exists := gatewayRequest["stop"]; exists {
		t.Fatalf("llm_gateway_request should not expose top-level stop")
	}
	if _, exists := gatewayRequest["stream"]; exists {
		t.Fatalf("llm_gateway_request should not expose stream")
	}
	if _, exists := gatewayRequest["user"]; exists {
		t.Fatalf("llm_gateway_request should not expose user")
	}

	contentTypes, ok := result.ProcessData["final_prompt_content_types"].([]string)
	if !ok {
		t.Fatalf("expected final_prompt_content_types to be []string, got %T", result.ProcessData["final_prompt_content_types"])
	}
	if len(contentTypes) == 0 {
		t.Fatalf("expected non-empty final_prompt_content_types")
	}
	if contentTypes[0] == "" {
		t.Fatalf("expected populated final_prompt_content_types")
	}

	urlPresence, ok := result.ProcessData["resolved_file_urls_present"].([]bool)
	if !ok {
		t.Fatalf("expected resolved_file_urls_present to be []bool, got %T", result.ProcessData["resolved_file_urls_present"])
	}
	if len(urlPresence) != 1 || !urlPresence[0] {
		t.Fatalf("expected resolved_file_urls_present=[true], got %#v", urlPresence)
	}

	visionVar := sharedVariablePool.Get([]string{"start-node", "query"})
	if visionVar == nil {
		t.Fatalf("expected start-node query to be added to shared variable pool")
	}

	convertedFile, ok := visionVar.GetValue().(*entities.File)
	if !ok || convertedFile == nil {
		t.Fatalf("expected start-node query variable to be file-backed, got %T", visionVar.GetValue())
	}
	if convertedFile.RemoteURL == "" {
		t.Fatalf("expected hydrated file to carry remote URL")
	}
	if convertedFile.MimeType != "image/jpeg" {
		t.Fatalf("expected hydrated file mime_type image/jpeg, got %q", convertedFile.MimeType)
	}
}

func TestExecuteWorkflowNodeWithCallbacks_VisionFailureRetainsProcessData(t *testing.T) {
	previousConfig := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		Server: appconfig.ServerConfig{Mode: "release"},
		Console: appconfig.ConsoleConfig{
			APIURL: "http://localhost:2679",
		},
		App: appconfig.AppConfig{
			FilesURL:  "http://localhost:2679",
			SecretKey: "test-secret",
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previousConfig
	})

	fileService := &mockWorkflowFileService{
		getFileByIDFn: func(ctx context.Context, fileID string) (*dto.UploadFile, error) {
			return &dto.UploadFile{
				ID:        fileID,
				Name:      "paper.jpg",
				Extension: "jpg",
				MimeType:  "image/jpeg",
				Size:      1024,
			}, nil
		},
	}

	executor := NewWorkflowExecutor()
	executor.fileService = fileService
	llmInvoked := false
	executor.SetLLMClient(&mockWorkflowLLMClient{
		appChatStreamFn: func(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error) {
			llmInvoked = true
			ch := make(chan llmAdapter.StreamResponse)
			close(ch)
			return ch, nil
		},
	})

	rawInputs := map[string]any{
		"query": map[string]any{
			"type":            "document",
			"transfer_method": "local_file",
			"upload_file_id":  "file-1",
			"url":             "",
		},
		"sys.workspace_id":    "ws-1",
		"sys.organization_id": "org-1",
		"sys.agent_id":        "app-1",
		"sys.workflow_id":     "workflow-1",
		"sys.user_id":         "user-1",
	}

	hydratedInputs, err := executor.HydrateInputs(context.Background(), rawInputs)
	if err != nil {
		t.Fatalf("HydrateInputs returned error: %v", err)
	}

	sharedVariablePool := executor.createVariablePoolWithVars(hydratedInputs, nil, nil)
	graphConfig := map[string]any{
		"nodes": []any{
			map[string]any{
				"id": "start-node",
				"data": map[string]any{
					"type": "start",
					"variables": []any{
						map[string]any{
							"variable": "query",
							"type":     "file",
						},
					},
				},
			},
			map[string]any{
				"id": "llm-node",
				"data": map[string]any{
					"type": "llm",
					"model": map[string]any{
						"provider":          "openai",
						"name":              "gpt-4o",
						"mode":              "chat",
						"completion_params": map[string]any{},
					},
					"context": map[string]any{
						"enabled": false,
					},
					"prompt_template": []any{
						map[string]any{
							"role": "system",
							"text": "You are a diagnosis assistant.",
						},
					},
					"vision": map[string]any{
						"enabled": true,
						"configs": map[string]any{
							"detail":            "high",
							"variable_selector": []any{"start-node", "query"},
						},
					},
				},
			},
		},
		"edges": []any{
			map[string]any{
				"source": "start-node",
				"target": "llm-node",
			},
		},
	}

	startConfig := map[string]any{
		"id": "start-node",
		"data": map[string]any{
			"type": "start",
			"variables": []any{
				map[string]any{
					"variable": "query",
					"type":     "file",
				},
			},
		},
	}

	if _, err := executor.ExecuteWorkflowNodeWithCallbacks(context.Background(), "start-node", shared.Start, startConfig, hydratedInputs, sharedVariablePool, graphConfig, nil, nil); err != nil {
		t.Fatalf("start node execution failed: %v", err)
	}

	llmConfig := map[string]any{
		"id": "llm-node",
		"data": map[string]any{
			"type": "llm",
			"model": map[string]any{
				"provider":          "openai",
				"name":              "gpt-4o",
				"mode":              "chat",
				"completion_params": map[string]any{},
			},
			"context": map[string]any{
				"enabled": false,
			},
			"prompt_template": []any{
				map[string]any{
					"role": "system",
					"text": "You are a diagnosis assistant.",
				},
			},
			"vision": map[string]any{
				"enabled": true,
				"configs": map[string]any{
					"detail":            "high",
					"variable_selector": []any{"start-node", "query"},
				},
			},
		},
	}

	result, err := executor.ExecuteWorkflowNodeWithCallbacks(context.Background(), "llm-node", shared.LLM, llmConfig, map[string]any{}, sharedVariablePool, graphConfig, nil, nil)
	if err == nil {
		t.Fatalf("expected llm node execution to fail when FILES_URL is not public")
	}
	if result == nil {
		t.Fatalf("expected partial node result on failure")
	}
	if llmInvoked {
		t.Fatalf("expected failure before upstream LLM invocation")
	}
	if got := result.Status; got != shared.FAILED {
		t.Fatalf("expected failed status, got %s", got)
	}
	if got := result.ProcessData["failed_stage"]; got != "prompt_messages" {
		t.Fatalf("expected failed_stage=prompt_messages, got %v", got)
	}
	if got := result.ProcessData["selected_file_transport"]; got != "signed_preview_url" {
		t.Fatalf("expected selected_file_transport=signed_preview_url, got %v", got)
	}
	if got := result.ProcessData["selected_file_url_host"]; got != "localhost" {
		t.Fatalf("expected selected_file_url_host=localhost, got %v", got)
	}
	if got := result.ProcessData["selected_file_url_is_public"]; got != false {
		t.Fatalf("expected selected_file_url_is_public=false, got %v", got)
	}
	if got := result.ProcessData["final_prompt_contains_inline_data"]; got != false {
		t.Fatalf("expected final_prompt_contains_inline_data=false, got %v", got)
	}
	if _, exists := result.ProcessData["llm_gateway_request"]; exists {
		t.Fatalf("prompt preparation failure should not include llm_gateway_request")
	}
	if result.Metadata[shared.ResolvedModelSource] != "node_default" {
		t.Fatalf("expected resolved model source metadata to be preserved, got %v", result.Metadata[shared.ResolvedModelSource])
	}
	if !strings.Contains(result.ErrMsg, "FILES_URL") {
		t.Fatalf("expected error message to mention FILES_URL, got %q", result.ErrMsg)
	}
}

func TestExecuteWorkflowNodeWithCallbacks_ModelConfigOverrideUsesInputOverrideSource(t *testing.T) {
	var capturedModel string

	executor := NewWorkflowExecutor()
	executor.SetLLMClient(&mockWorkflowLLMClient{
		appChatStreamFn: func(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error) {
			capturedModel = req.Model
			ch := make(chan llmAdapter.StreamResponse, 1)
			go func() {
				defer close(ch)
				ch <- llmAdapter.StreamResponse{
					Choices: []llmAdapter.StreamChoice{
						{Delta: llmAdapter.Message{Role: "assistant", Content: "override ok"}, FinishReason: "stop"},
					},
					Usage: &llmAdapter.Usage{TotalTokens: 12},
				}
			}()
			return ch, nil
		},
	})

	inputs := map[string]any{
		"sys.user_id":         "user-1",
		"sys.agent_id":        "agent-1",
		"sys.workflow_id":     "workflow-1",
		"sys.workspace_id":    "workspace-1",
		"sys.organization_id": "org-1",
		"sys.query":           "hello",
		"model_config": map[string]any{
			"provider": "deepseek",
			"model":    "deepseek-v3",
			"mode":     "chat",
		},
	}
	variablePool := executor.createVariablePoolWithVars(inputs, nil, nil)

	graphConfig := map[string]any{
		"nodes": []any{
			map[string]any{
				"id": "llm-node",
				"data": map[string]any{
					"type": "llm",
					"model": map[string]any{
						"provider":          "openai",
						"name":              "gpt-4o",
						"mode":              "chat",
						"completion_params": map[string]any{},
					},
					"context": map[string]any{
						"enabled": false,
					},
					"prompt_template": []any{
						map[string]any{
							"role": "system",
							"text": "You are a diagnosis assistant.",
						},
						map[string]any{
							"role": "user",
							"text": "{{#sys.query#}}",
						},
					},
					"vision": map[string]any{
						"enabled": false,
					},
				},
			},
		},
	}

	llmConfig := map[string]any{
		"id": "llm-node",
		"data": map[string]any{
			"type": "llm",
			"model": map[string]any{
				"provider":          "openai",
				"name":              "gpt-4o",
				"mode":              "chat",
				"completion_params": map[string]any{},
			},
			"context": map[string]any{
				"enabled": false,
			},
			"prompt_template": []any{
				map[string]any{
					"role": "system",
					"text": "You are a diagnosis assistant.",
				},
				map[string]any{
					"role": "user",
					"text": "{{#sys.query#}}",
				},
			},
			"vision": map[string]any{
				"enabled": false,
			},
		},
	}

	result, err := executor.ExecuteWorkflowNodeWithCallbacks(context.Background(), "llm-node", shared.LLM, llmConfig, inputs, variablePool, graphConfig, nil, nil)
	if err != nil {
		t.Fatalf("llm node execution failed: %v", err)
	}

	if capturedModel != "deepseek-v3" {
		t.Fatalf("expected gateway request model deepseek-v3, got %q", capturedModel)
	}
	if got := result.ProcessData["resolved_model_source"]; got != "input_override" {
		t.Fatalf("expected resolved_model_source=input_override, got %v", got)
	}
	if got := result.ProcessData["resolved_model_provider"]; got != "deepseek" {
		t.Fatalf("expected resolved_model_provider=deepseek, got %v", got)
	}
	if got := result.ProcessData["resolved_model_name"]; got != "deepseek-v3" {
		t.Fatalf("expected resolved_model_name=deepseek-v3, got %v", got)
	}
	if got := result.Metadata[shared.WorkflowNodeExecutionMetadataKey("resolved_model_source")]; got != "input_override" {
		t.Fatalf("expected metadata resolved_model_source=input_override, got %v", got)
	}

	gatewayRequest, ok := result.ProcessData["llm_gateway_request"].(map[string]any)
	if !ok {
		t.Fatalf("expected llm_gateway_request in process data, got %T", result.ProcessData["llm_gateway_request"])
	}
	if got := gatewayRequest["model"]; got != "deepseek-v3" {
		t.Fatalf("expected llm_gateway_request.model=deepseek-v3, got %v", got)
	}
}

func TestExecuteWorkflowNodeWithCallbacks_InvokeFailureRetainsGatewayRequest(t *testing.T) {
	var capturedRequest *llmAdapter.ChatRequest

	executor := NewWorkflowExecutor()
	executor.SetLLMClient(&mockWorkflowLLMClient{
		appChatStreamFn: func(ctx context.Context, appCtx *llmClient.AppContext, req *llmAdapter.ChatRequest) (<-chan llmAdapter.StreamResponse, error) {
			capturedRequest = req
			return nil, errors.New("gateway unavailable")
		},
	})

	inputs := map[string]any{
		"sys.user_id":         "user-1",
		"sys.agent_id":        "agent-1",
		"sys.workflow_id":     "workflow-1",
		"sys.workspace_id":    "workspace-1",
		"sys.organization_id": "org-1",
		"sys.query":           "hello",
	}
	variablePool := executor.createVariablePoolWithVars(inputs, nil, nil)

	graphConfig := map[string]any{
		"nodes": []any{
			map[string]any{
				"id": "llm-node",
				"data": map[string]any{
					"type": "llm",
					"model": map[string]any{
						"provider": "openai",
						"name":     "gpt-4o",
						"mode":     "chat",
						"completion_params": map[string]any{
							"temperature": 0.3,
						},
					},
					"context": map[string]any{
						"enabled": false,
					},
					"prompt_template": []any{
						map[string]any{
							"role": "user",
							"text": "{{#sys.query#}}",
						},
					},
					"vision": map[string]any{
						"enabled": false,
					},
				},
			},
		},
	}

	llmConfig := map[string]any{
		"id":   "llm-node",
		"data": graphConfig["nodes"].([]any)[0].(map[string]any)["data"],
	}

	result, err := executor.ExecuteWorkflowNodeWithCallbacks(context.Background(), "llm-node", shared.LLM, llmConfig, inputs, variablePool, graphConfig, nil, nil)
	if err == nil {
		t.Fatalf("expected llm node execution to fail")
	}
	if result == nil {
		t.Fatalf("expected partial node result on failure")
	}
	if got := result.Status; got != shared.FAILED {
		t.Fatalf("expected failed status, got %s", got)
	}
	if got := result.ProcessData["failed_stage"]; got != "llm_invoke" {
		t.Fatalf("expected failed_stage=llm_invoke, got %v", got)
	}
	if capturedRequest == nil {
		t.Fatalf("expected gateway request to be captured before invoke failure")
	}

	gatewayRequest, ok := result.ProcessData["llm_gateway_request"].(map[string]any)
	if !ok {
		t.Fatalf("expected llm_gateway_request in process data, got %T", result.ProcessData["llm_gateway_request"])
	}
	if got := gatewayRequest["model"]; got != capturedRequest.Model {
		t.Fatalf("expected llm_gateway_request.model=%q, got %v", capturedRequest.Model, got)
	}
	if !reflect.DeepEqual(gatewayRequest["messages"], normalizeWorkflowTestValue(t, capturedRequest.Messages)) {
		t.Fatalf("expected llm_gateway_request.messages to match gateway request")
	}
	params, ok := gatewayRequest["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected llm_gateway_request.params, got %T", gatewayRequest["params"])
	}
	if got := params["temperature"]; got != 0.3 {
		t.Fatalf("expected llm_gateway_request.params.temperature=0.3, got %v", got)
	}
}
