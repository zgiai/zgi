package llm

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

type stubFileDownloader struct {
	downloadFn func(ctx context.Context, fileID string) ([]byte, error)
}

type stubLLMInvoker struct{}

func (s *stubFileDownloader) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	if s.downloadFn != nil {
		return s.downloadFn(ctx, fileID)
	}
	return nil, nil
}

func (s *stubLLMInvoker) InvokeStream(ctx context.Context, accountID, appID, appType string, req *LLMInvokeRequest) (<-chan *ResultChunk, <-chan error, error) {
	resultCh := make(chan *ResultChunk, 1)
	errCh := make(chan error)

	go func() {
		defer close(resultCh)
		defer close(errCh)

		resultCh <- &ResultChunk{
			Model: req.ModelSlug,
			Delta: &ResultChunkDelta{
				Message: &PromptMessage{
					Role:    PromptMessageRoleAssistant,
					Content: "mock response",
				},
				FinishReason: "stop",
			},
		}
	}()

	return resultCh, errCh, nil
}

func setTestFileURLConfig(t *testing.T, filesURL string, serverMode string) {
	t.Helper()

	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		Server: appconfig.ServerConfig{Mode: serverMode},
		Console: appconfig.ConsoleConfig{
			APIURL: filesURL,
		},
		App: appconfig.AppConfig{
			FilesURL:  filesURL,
			SecretKey: "test-secret",
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})
}

// TestLLMNode_Run_Integration exercises the Node end-to-end without real network calls.
func TestLLMNode_Run_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// Graph runtime state with empty variable pool
	vpool := entities.NewVariablePool()
	grs := entities.NewGraphRuntimeState(vpool)

	// Minimal node data: chat mode, static message, no memory/context/files
	nodeData := NodeData{
		NodeData: base.NodeData{},
		Model: ModelConfig{
			Provider:         "deepseek",
			Name:             "deepseek-chat",
			Mode:             ModeChat,
			CompletionParams: map[string]any{"temperature": 0.7, "stop": []string{"END"}},
		},
		PromptTemplate: []NodeChatModelMessage{
			{Role: PromptMessageRoleUser, Text: "Hello", EditionType: "basic"},
		},
	}

	bns := base.NodeStruct{
		InstanceID:        "inst-1",
		NodeID:            "llm-node-1",
		TenantID:          "tenant-1",
		APPID:             "app-1",
		Graph:             nil,
		GraphRuntimeState: grs,
	}
	n := &Node{
		NodeStruct: bns,
		nodeData:   nodeData,
		invoker:    &stubLLMInvoker{},
	}

	eventChan := make(chan *shared.NodeEventCh, 16)

	if err := n.Run(ctx, eventChan); err != nil {
		// Even if provider client is not initialized, the node should not hard-fail here due to our mock
		// If it does, surface the error
		t.Fatalf("node run failed: %v", err)
	}

	// Drain events emitted during Run (channel is not closed by Run)
	events := make([]*shared.NodeEventCh, 0, 8)
ForLoop:
	for {
		select {
		case ev := <-eventChan:
			if ev == nil {
				break ForLoop
			}
			events = append(events, ev)
		default:
			break ForLoop
		}
	}

	if len(events) == 0 {
		t.Fatalf("no events emitted by node")
	}

	// Expect at least: run_started, model_invoke_completed, run_completed
	var hasStarted, hasModelCompleted, hasRunCompleted bool
	for _, ev := range events {
		switch ev.Type {
		case shared.EventTypeRunStarted:
			hasStarted = true
		case shared.EventTypeModelInvokeCompleted:
			hasModelCompleted = true
		case shared.EventTypeRunCompleted:
			hasRunCompleted = true
			for _, event := range events {
				fmt.Println(event.Data)
			}
		}
	}

	if !hasStarted {
		t.Errorf("missing EventTypeRunStarted")
	}
	if !hasModelCompleted {
		t.Errorf("missing EventTypeModelInvokeCompleted")
	}
	if !hasRunCompleted {
		t.Errorf("missing EventTypeRunCompleted")
	}
}

func TestFilterInvalidMessages_QwenSupportsImageContent(t *testing.T) {
	n := &Node{}
	modelConfig := &ModelConfigWithCredentialsEntity{
		Provider: "qwen",
		Model:    "qwen-vl-max",
	}

	messages := []PromptMessage{
		{
			Role: PromptMessageRoleUser,
			Content: []PromptMessageContent{
				{
					Type: PromptMessageContentTypeImage,
					URL:  "https://example.com/test.jpg",
				},
			},
		},
	}

	filtered := n.filterInvalidMessages(messages, modelConfig)
	if len(filtered) != 1 {
		t.Fatalf("expected one message after filtering, got %d", len(filtered))
	}

	contentList, ok := filtered[0].Content.([]PromptMessageContent)
	if !ok {
		t.Fatalf("expected multimodal content to be preserved, got %T", filtered[0].Content)
	}
	if len(contentList) != 1 {
		t.Fatalf("expected one content item after filtering, got %d", len(contentList))
	}
	if contentList[0].Type != PromptMessageContentTypeImage {
		t.Fatalf("expected image content to be preserved, got %s", contentList[0].Type)
	}
}

func TestProcessVisionFiles_OpenAIUsesImageMimeEvenWhenDeclaredAsDocument(t *testing.T) {
	n := &Node{}
	modelConfig := &ModelConfigWithCredentialsEntity{
		Provider: "openai",
		Model:    "gpt-4o",
	}

	workflowFile := file.NewFile(
		"tenant-1",
		file.FileTypeDocument,
		file.FileTransferMethodRemoteURL,
		file.WithRemoteURL("https://example.com/test.jpg"),
		file.WithMimeType("image/jpeg"),
	)

	messages := []PromptMessage{
		{
			Role:    PromptMessageRoleUser,
			Content: "请分析这张图片",
		},
	}

	processed, autoInjected, err := n.processVisionFiles(messages, []any{workflowFile}, true, ImageDetailHigh)
	if err != nil {
		t.Fatalf("processVisionFiles returned error: %v", err)
	}
	if autoInjected {
		t.Fatalf("expected autoInjected=false when explicit user prompt already exists")
	}

	filtered := n.filterInvalidMessages(processed, modelConfig)
	if len(filtered) != 1 {
		t.Fatalf("expected one message after filtering, got %d", len(filtered))
	}

	contentList, ok := filtered[0].Content.([]PromptMessageContent)
	if !ok {
		t.Fatalf("expected multimodal content to be preserved, got %T", filtered[0].Content)
	}
	if len(contentList) != 2 {
		t.Fatalf("expected image and text content after filtering, got %d items", len(contentList))
	}
	if contentList[0].Type != PromptMessageContentTypeImage {
		t.Fatalf("expected first content item to be image, got %s", contentList[0].Type)
	}
	if contentList[1].Type != PromptMessageContentTypeText {
		t.Fatalf("expected second content item to be text, got %s", contentList[1].Type)
	}
}

func TestParseLLMNodeDataFromConfig_PreservesVisionSelector(t *testing.T) {
	config := map[string]any{
		"id": "llm-node",
		"data": map[string]any{
			"title": "LLM",
			"type":  "llm",
			"model": map[string]any{
				"provider":          "openai",
				"name":              "gpt-4o",
				"mode":              "chat",
				"completion_params": map[string]any{},
			},
			"prompt_template": []map[string]any{
				{
					"role": "system",
					"text": "system",
				},
			},
			"vision": map[string]any{
				"enabled": true,
				"configs": map[string]any{
					"detail":            "high",
					"variable_selector": []string{"start-node", "query"},
				},
			},
		},
	}

	nodeData, nodeID, err := parseLLMNodeDataFromConfig(config)
	if err != nil {
		t.Fatalf("parseLLMNodeDataFromConfig returned error: %v", err)
	}
	if nodeID != "llm-node" {
		t.Fatalf("expected node id llm-node, got %s", nodeID)
	}
	if !nodeData.Vision.Enabled {
		t.Fatalf("expected vision to stay enabled")
	}
	if len(nodeData.Vision.Configs.VariableSelector) != 2 {
		t.Fatalf("expected 2 vision selector segments, got %d", len(nodeData.Vision.Configs.VariableSelector))
	}
	if nodeData.Vision.Configs.VariableSelector[0] != "start-node" || nodeData.Vision.Configs.VariableSelector[1] != "query" {
		t.Fatalf("unexpected vision selector: %#v", nodeData.Vision.Configs.VariableSelector)
	}
}

func TestHandleChatModelTemplate_BasicMessageReadsNestedIterationField(t *testing.T) {
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"iter_1", "item"}, map[string]any{
		"number": 5,
		"type":   "选择题",
	})

	node := &Node{}
	messages := []NodeChatModelMessage{
		{
			Role:        PromptMessageRoleUser,
			Text:        "数量：{{#iter_1.item.number#}}",
			EditionType: "basic",
		},
	}

	promptMessages, err := node.handleChatModelTemplate(
		messages,
		"",
		nil,
		variablePool,
		ImageDetailAuto,
	)
	if err != nil {
		t.Fatalf("handleChatModelTemplate() error = %v", err)
	}

	if len(promptMessages) != 1 {
		t.Fatalf("len(promptMessages) = %d, want 1", len(promptMessages))
	}

	content, ok := promptMessages[0].Content.(string)
	if !ok {
		t.Fatalf("prompt message content type = %T, want string", promptMessages[0].Content)
	}

	if content != "数量：5" {
		t.Fatalf("content = %q, want %q", content, "数量：5")
	}
}

func TestProcessConversationalSystemVariables_PreservesWrappedSystemVariables(t *testing.T) {
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"sys", "query"}, "hello world")
	variablePool.Add([]string{"sys", "conversation_id"}, "conv-123")

	node := &Node{}
	got := node.processConversationalSystemVariables(
		"bare=#sys.query# wrapped={{#sys.query#}} conv={{#sys.conversation_id#}}",
		variablePool,
	)

	want := "bare=hello world wrapped={{#sys.query#}} conv={{#sys.conversation_id#}}"
	if got != want {
		t.Fatalf("processConversationalSystemVariables() = %q, want %q", got, want)
	}
}

func TestHandleChatModelTemplate_BasicMessageResolvesBareAndWrappedSysQuery(t *testing.T) {
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"sys", "query"}, "hello world")

	node := &Node{}
	messages := []NodeChatModelMessage{
		{
			Role:        PromptMessageRoleUser,
			Text:        "bare=#sys.query# wrapped={{#sys.query#}}",
			EditionType: "basic",
		},
	}

	promptMessages, err := node.handleChatModelTemplate(
		messages,
		"",
		nil,
		variablePool,
		ImageDetailAuto,
	)
	if err != nil {
		t.Fatalf("handleChatModelTemplate() error = %v", err)
	}

	if len(promptMessages) != 1 {
		t.Fatalf("len(promptMessages) = %d, want 1", len(promptMessages))
	}

	content, ok := promptMessages[0].Content.(string)
	if !ok {
		t.Fatalf("prompt message content type = %T, want string", promptMessages[0].Content)
	}

	want := "bare=hello world wrapped=hello world"
	if content != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}

func TestAddCurrentQueryForCompletion_PreservesWrappedSysQueryTemplate(t *testing.T) {
	node := &Node{}
	promptMessages := []PromptMessage{
		{
			Role:    PromptMessageRoleUser,
			Content: "bare=#sys.query# wrapped={{#sys.query#}}",
		},
	}

	if err := node.addCurrentQueryForCompletion("hello world", promptMessages); err != nil {
		t.Fatalf("addCurrentQueryForCompletion() error = %v", err)
	}

	content, ok := promptMessages[0].Content.(string)
	if !ok {
		t.Fatalf("prompt message content type = %T, want string", promptMessages[0].Content)
	}

	want := "bare=hello world wrapped={{#sys.query#}}"
	if content != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}

func TestInsertHistoryIntoPrompt_ReplacesWrappedHistoriesPlaceholder(t *testing.T) {
	node := &Node{}
	promptMessages := []PromptMessage{
		{
			Role:    PromptMessageRoleUser,
			Content: "<histories>{{#histories#}}</histories>\nAssistant:",
		},
	}

	if err := node.insertHistoryIntoPrompt(promptMessages, "user: hi\nassistant: hello"); err != nil {
		t.Fatalf("insertHistoryIntoPrompt() error = %v", err)
	}

	content, ok := promptMessages[0].Content.(string)
	if !ok {
		t.Fatalf("prompt message content type = %T, want string", promptMessages[0].Content)
	}

	want := "<histories>user: hi\nassistant: hello</histories>\nAssistant:"
	if content != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}

func TestReplaceContextPlaceholder_ReplacesWrappedAndLegacySyntax(t *testing.T) {
	got := replaceContextPlaceholder("wrapped={{#context#}} legacy={#context#}", "ctx")
	want := "wrapped=ctx legacy=ctx"
	if got != want {
		t.Fatalf("replaceContextPlaceholder() = %q, want %q", got, want)
	}
}

func TestFetchMemory_NewNodeDisabledIgnoresProvidedHistory(t *testing.T) {
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"sys", "conversation_history"}, []map[string]interface{}{
		{"role": "user", "content": "old question"},
		{"role": "assistant", "content": "old answer"},
	})

	node := &Node{
		nodeData: NodeData{
			ConversationHistory: &ConversationHistoryConfig{
				Enabled:           false,
				HistoryWindowSize: 3,
			},
		},
	}

	memory, memoryConfig := node.fetchMemory(context.Background(), variablePool, "", nil, nil)
	if len(memory.Messages) != 0 {
		t.Fatalf("memory messages = %d, want 0", len(memory.Messages))
	}
	if !memory.HistoryExplicitlyProvided {
		t.Fatal("HistoryExplicitlyProvided = false, want true")
	}
	if memoryConfig.Window.Enabled {
		t.Fatal("memory window enabled = true, want false")
	}
}

func TestFetchMemory_LegacyNodeUsesExplicitProvidedHistory(t *testing.T) {
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"sys", "conversation_history"}, []map[string]interface{}{
		{"role": "user", "content": "old question"},
		{"role": "assistant", "content": "old answer"},
	})

	node := &Node{}
	memory, _ := node.fetchMemory(context.Background(), variablePool, "", nil, nil)
	if len(memory.Messages) != 2 {
		t.Fatalf("memory messages = %d, want 2", len(memory.Messages))
	}
	if memory.Messages[0].Role != PromptMessageRoleUser || memory.Messages[0].Content != "old question" {
		t.Fatalf("first message = %#v", memory.Messages[0])
	}
	if memory.Messages[1].Role != PromptMessageRoleAssistant || memory.Messages[1].Content != "old answer" {
		t.Fatalf("second message = %#v", memory.Messages[1])
	}
}

func TestFetchMemory_LegacyFallbackDisabledDoesNotLoadHistory(t *testing.T) {
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"sys", "conversation_id"}, uuid.NewString())

	node := &Node{
		NodeStruct: base.NodeStruct{
			GraphConfig: map[string]any{
				"features": map[string]interface{}{
					"conversation_history": map[string]interface{}{
						"enabled":             false,
						"history_window_size": float64(3),
					},
				},
			},
		},
	}

	memory, _ := node.fetchMemory(context.Background(), variablePool, "", nil, nil)
	if len(memory.Messages) != 0 {
		t.Fatalf("memory messages = %d, want 0", len(memory.Messages))
	}
}

func TestFetchMemory_LegacyFallbackZeroWindowDoesNotLoadHistory(t *testing.T) {
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"sys", "conversation_id"}, uuid.NewString())

	node := &Node{
		NodeStruct: base.NodeStruct{
			GraphConfig: map[string]any{
				"features": map[string]interface{}{
					"conversation_history": map[string]interface{}{
						"enabled":             true,
						"history_window_size": float64(0),
					},
				},
			},
		},
	}

	memory, _ := node.fetchMemory(context.Background(), variablePool, "", nil, nil)
	if len(memory.Messages) != 0 {
		t.Fatalf("memory messages = %d, want 0", len(memory.Messages))
	}
}

func TestTokenBufferMemory_DoesNotImplicitlyLoadConversationHistory(t *testing.T) {
	memory := NewTokenBufferMemory(
		map[string]any{
			"messages": []any{
				map[string]any{"role": "user", "content": "old question"},
			},
		},
		nil,
		4000,
		"",
		"",
	)

	messages := memory.GetHistoryPromptMessages(4000, -1)
	if len(messages) != 0 {
		t.Fatalf("history messages = %d, want 0", len(messages))
	}
}

func TestPromptMessagesFromAgentMessagesExpandsRounds(t *testing.T) {
	records := []*conversation.AgentMessage{
		{Query: "q1", Answer: "a1"},
		{Query: "q2", Answer: "a2"},
	}

	messages := promptMessagesFromAgentMessages(records)
	if len(messages) != 4 {
		t.Fatalf("messages = %d, want 4", len(messages))
	}
	if messages[0].Role != PromptMessageRoleUser || messages[0].Content != "q1" {
		t.Fatalf("message[0] = %#v", messages[0])
	}
	if messages[1].Role != PromptMessageRoleAssistant || messages[1].Content != "a1" {
		t.Fatalf("message[1] = %#v", messages[1])
	}
	if messages[2].Role != PromptMessageRoleUser || messages[2].Content != "q2" {
		t.Fatalf("message[2] = %#v", messages[2])
	}
	if messages[3].Role != PromptMessageRoleAssistant || messages[3].Content != "a2" {
		t.Fatalf("message[3] = %#v", messages[3])
	}
}

func TestFetchPromptMessages_LegacyOrderWithoutPromptLayout(t *testing.T) {
	node := &Node{}
	variablePool := entities.NewVariablePool()
	memory := explicitTestMemory([]PromptMessage{
		{Role: PromptMessageRoleUser, Content: "history question"},
		{Role: PromptMessageRoleAssistant, Content: "history answer"},
	})

	messages := []NodeChatModelMessage{
		{Role: PromptMessageRoleSystem, Text: "system", EditionType: "basic"},
		{Role: PromptMessageRoleUser, Text: "user template", EditionType: "basic"},
	}

	promptMessages, _, _, err := node.fetchPromptMessages(
		"current query",
		nil,
		"",
		memory,
		&ModelConfigWithCredentialsEntity{},
		messages,
		testMemoryConfig(false, 0),
		false,
		ImageDetailAuto,
		variablePool,
		nil,
	)
	if err != nil {
		t.Fatalf("fetchPromptMessages() error = %v", err)
	}

	assertPromptMessageSequence(t, promptMessages, []promptMessageExpectation{
		{role: PromptMessageRoleSystem, content: "system"},
		{role: PromptMessageRoleUser, content: "history question"},
		{role: PromptMessageRoleAssistant, content: "history answer"},
		{role: PromptMessageRoleUser, content: "user template"},
	})
}

type promptMessageExpectation struct {
	role    PromptMessageRole
	content string
}

func explicitTestMemory(messages []PromptMessage) *TokenBufferMemory {
	return &TokenBufferMemory{
		Messages:                  messages,
		HistoryExplicitlyProvided: true,
	}
}

func testMemoryConfig(windowEnabled bool, windowSize int) *MemoryConfig {
	return &MemoryConfig{
		RolePrefix: RolePrefix{
			User:      "Human",
			Assistant: "Assistant",
		},
		Window: WindowConfig{
			Enabled: windowEnabled,
			Size:    windowSize,
		},
	}
}

func assertPromptMessageSequence(t *testing.T, got []PromptMessage, want []promptMessageExpectation) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("prompt messages length = %d, want %d: %#v", len(got), len(want), got)
	}

	for i := range want {
		if got[i].Role != want[i].role {
			t.Fatalf("prompt message[%d].Role = %q, want %q", i, got[i].Role, want[i].role)
		}
		content, ok := got[i].Content.(string)
		if !ok {
			t.Fatalf("prompt message[%d].Content type = %T, want string", i, got[i].Content)
		}
		if content != want[i].content {
			t.Fatalf("prompt message[%d].Content = %q, want %q", i, content, want[i].content)
		}
	}
}

func TestFetchPromptMessages_WithPromptLayoutPlacesHistoryAfterSystem(t *testing.T) {
	node := &Node{
		nodeData: NodeData{
			PromptLayout: &PromptLayout{
				Version: 1,
				Items: []PromptLayoutItem{
					{Type: PromptLayoutItemGroup, GroupID: "current-user"},
					{Type: PromptLayoutItemHistory, ID: "conversation_history"},
				},
			},
		},
	}
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"sys", "query"}, "current query")
	memory := explicitTestMemory([]PromptMessage{
		{Role: PromptMessageRoleUser, Content: "history question"},
		{Role: PromptMessageRoleAssistant, Content: "history answer"},
	})

	messages := []NodeChatModelMessage{
		{Role: PromptMessageRoleSystem, Text: "system", EditionType: "basic"},
		{
			Role:        PromptMessageRoleUser,
			Text:        "{{#sys.query#}}",
			GroupID:     "current-user",
			GroupKind:   PromptGroupKindCurrentUser,
			EditionType: "basic",
		},
	}

	promptMessages, _, _, err := node.fetchPromptMessages(
		"current query",
		nil,
		"",
		memory,
		&ModelConfigWithCredentialsEntity{},
		messages,
		testMemoryConfig(false, 0),
		false,
		ImageDetailAuto,
		variablePool,
		nil,
	)
	if err != nil {
		t.Fatalf("fetchPromptMessages() error = %v", err)
	}

	assertPromptMessageSequence(t, promptMessages, []promptMessageExpectation{
		{role: PromptMessageRoleSystem, content: "system"},
		{role: PromptMessageRoleUser, content: "history question"},
		{role: PromptMessageRoleAssistant, content: "history answer"},
		{role: PromptMessageRoleUser, content: "current query"},
	})
}

func TestFetchPromptMessages_WithPromptLayoutAppendsCurrentQueryWhenMissing(t *testing.T) {
	node := &Node{
		nodeData: NodeData{
			PromptLayout: &PromptLayout{
				Version: 1,
				Items:   []PromptLayoutItem{},
			},
		},
	}
	variablePool := entities.NewVariablePool()
	memory := explicitTestMemory([]PromptMessage{
		{Role: PromptMessageRoleUser, Content: "history question"},
	})

	messages := []NodeChatModelMessage{
		{Role: PromptMessageRoleSystem, Text: "system", EditionType: "basic"},
	}

	promptMessages, _, _, err := node.fetchPromptMessages(
		"current query",
		nil,
		"",
		memory,
		&ModelConfigWithCredentialsEntity{},
		messages,
		testMemoryConfig(false, 0),
		false,
		ImageDetailAuto,
		variablePool,
		nil,
	)
	if err != nil {
		t.Fatalf("fetchPromptMessages() error = %v", err)
	}

	assertPromptMessageSequence(t, promptMessages, []promptMessageExpectation{
		{role: PromptMessageRoleSystem, content: "system"},
		{role: PromptMessageRoleUser, content: "history question"},
		{role: PromptMessageRoleUser, content: "current query"},
	})
}

func TestFetchPromptMessages_LegacyMalformedSequenceWithoutLayoutPreservesOldOrder(t *testing.T) {
	node := &Node{}
	variablePool := entities.NewVariablePool()
	memory := explicitTestMemory(nil)

	messages := []NodeChatModelMessage{
		{Role: PromptMessageRoleSystem, Text: "system", EditionType: "basic"},
		{Role: PromptMessageRoleUser, Text: "u1", EditionType: "basic"},
		{Role: PromptMessageRoleUser, Text: "u2", EditionType: "basic"},
		{Role: PromptMessageRoleAssistant, Text: "a1", EditionType: "basic"},
		{Role: PromptMessageRoleAssistant, Text: "a2", EditionType: "basic"},
		{Role: PromptMessageRoleAssistant, Text: "a3", EditionType: "basic"},
		{Role: PromptMessageRoleUser, Text: "u3", EditionType: "basic"},
	}

	promptMessages, _, _, err := node.fetchPromptMessages(
		"current query",
		nil,
		"",
		memory,
		&ModelConfigWithCredentialsEntity{},
		messages,
		testMemoryConfig(false, 0),
		false,
		ImageDetailAuto,
		variablePool,
		nil,
	)
	if err != nil {
		t.Fatalf("fetchPromptMessages() error = %v", err)
	}

	assertPromptMessageSequence(t, promptMessages, []promptMessageExpectation{
		{role: PromptMessageRoleSystem, content: "system"},
		{role: PromptMessageRoleUser, content: "u1"},
		{role: PromptMessageRoleUser, content: "u2"},
		{role: PromptMessageRoleAssistant, content: "a1"},
		{role: PromptMessageRoleAssistant, content: "a2"},
		{role: PromptMessageRoleAssistant, content: "a3"},
		{role: PromptMessageRoleUser, content: "u3"},
	})
}

func TestRenderTemplateMessage_UsesNestedTemplateVariableSelector(t *testing.T) {
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"iter_1", "item"}, map[string]any{
		"number": 5,
		"type":   "选择题",
	})

	node := &Node{}
	template := "数量：{{ count }}"
	rendered, err := node.renderTemplateMessage(
		&template,
		[]VariableSelector{
			{
				Variable:      "count",
				ValueSelector: []string{"iter_1", "item", "number"},
			},
		},
		variablePool,
	)
	if err != nil {
		t.Fatalf("renderTemplateMessage() error = %v", err)
	}

	if rendered != "数量：5" {
		t.Fatalf("rendered = %q, want %q", rendered, "数量：5")
	}
}

func TestResolveSelectorVariable_UsesNestedPathForScalarsAndFiles(t *testing.T) {
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"iter_1", "item"}, map[string]any{
		"number": 5,
		"attachment": map[string]any{
			"type":            "image",
			"transfer_method": "remote_url",
			"upload_file_id":  "file-1",
			"url":             "https://example.com/paper.jpg",
			"mime_type":       "image/jpeg",
		},
	})

	numberVar := resolveSelectorVariable(variablePool, []string{"iter_1", "item", "number"})
	if numberVar == nil {
		t.Fatalf("expected nested number variable to resolve")
	}
	if got := numberVar.ToObject(); got != 5.0 {
		t.Fatalf("numberVar.ToObject() = %#v, want 5", got)
	}

	fileVar := resolveSelectorVariable(variablePool, []string{"iter_1", "item", "attachment"})
	if fileVar == nil {
		t.Fatalf("expected nested file variable to resolve")
	}
	if fileVar.GetType() != shared.SegmentTypeFile {
		t.Fatalf("fileVar.GetType() = %s, want %s", fileVar.GetType(), shared.SegmentTypeFile)
	}
}

func TestFetchFiles_ReturnsStartNodeFileFromVariablePool(t *testing.T) {
	vpool := entities.NewVariablePool()
	vpool.Add([]string{"start-node", "query"}, map[string]any{
		"type":            "image",
		"transfer_method": "local_file",
		"upload_file_id":  "file-1",
		"filename":        "paper.jpg",
		"extension":       "jpg",
		"mime_type":       "image/jpeg",
	})

	n := &Node{
		NodeStruct: base.NodeStruct{
			GraphRuntimeState: entities.NewGraphRuntimeState(vpool),
		},
	}

	files, err := n.fetchFiles(vpool, []string{"start-node", "query"})
	if err != nil {
		t.Fatalf("fetchFiles returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one file, got %d", len(files))
	}
	if files[0] == nil {
		t.Fatalf("expected non-nil file")
	}
	if files[0].MimeType == nil || *files[0].MimeType != "image/jpeg" {
		t.Fatalf("expected image/jpeg mime type, got %#v", files[0].MimeType)
	}
}

func TestFetchFiles_UsesNestedSelector(t *testing.T) {
	vpool := entities.NewVariablePool()
	vpool.Add([]string{"start-node", "payload"}, map[string]any{
		"query": map[string]any{
			"type":            "image",
			"transfer_method": "remote_url",
			"upload_file_id":  "file-1",
			"id":              "file-1",
			"workspace_id":    "ws-1",
			"url":             "https://example.com/paper.jpg",
			"filename":        "paper.jpg",
			"extension":       "jpg",
			"mime_type":       "image/jpeg",
		},
	})

	n := &Node{
		NodeStruct: base.NodeStruct{
			GraphRuntimeState: entities.NewGraphRuntimeState(vpool),
		},
	}

	files, err := n.fetchFiles(vpool, []string{"start-node", "payload", "query"})
	if err != nil {
		t.Fatalf("fetchFiles returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one file, got %d", len(files))
	}
	if files[0] == nil {
		t.Fatalf("expected non-nil file")
	}
}

func TestFetchFiles_FallsBackToUserInputsWhenSelectorMissing(t *testing.T) {
	vpool := entities.NewVariablePool()
	vpool.UserInputs["query"] = map[string]any{
		"type":            "file",
		"transfer_method": "local_file",
		"upload_file_id":  "file-1",
		"filename":        "paper.jpg",
		"extension":       "jpg",
		"mime_type":       "image/jpeg",
	}

	n := &Node{
		NodeStruct: base.NodeStruct{
			GraphRuntimeState: entities.NewGraphRuntimeState(vpool),
		},
	}

	files, err := n.fetchFiles(vpool, []string{"start-node", "query"})
	if err != nil {
		t.Fatalf("fetchFiles returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one file from user input fallback, got %d", len(files))
	}
	if files[0].MimeType == nil || *files[0].MimeType != "image/jpeg" {
		t.Fatalf("expected image/jpeg mime type from user input fallback, got %#v", files[0].MimeType)
	}
	if files[0].Type != file.FileTypeImage {
		t.Fatalf("expected inferred file type image from user input fallback, got %#v", files[0].Type)
	}
}

func TestConvertEntityFileToWorkflowFile_UsesWorkspaceIDAsWorkflowFileTenantID(t *testing.T) {
	entityFile := &entities.File{
		ID:             "file-1",
		WorkspaceID:    "ws-1",
		Type:           "image",
		TransferMethod: "remote_url",
		RemoteURL:      "https://example.com/paper.jpg",
		Filename:       "paper.jpg",
		Extension:      ".jpg",
		MimeType:       "image/jpeg",
		Size:           1024,
	}

	workflowFile := convertEntityFileToWorkflowFile(entityFile)
	if workflowFile == nil {
		t.Fatalf("expected converted workflow file")
	}
	if workflowFile.TenantID != "ws-1" {
		t.Fatalf("workflowFile.TenantID = %q, want %q", workflowFile.TenantID, "ws-1")
	}
}

func TestProcessVisionFiles_AddsDefaultTextWhenNoUserPromptExists(t *testing.T) {
	n := &Node{}
	expectedPromptText := "Analyze the uploaded image or file directly. Use all visible content, including questions, answers, annotations, scores, diagrams, and layout details, to complete the task."

	workflowFile := file.NewFile(
		"tenant-1",
		file.FileTypeDocument,
		file.FileTransferMethodRemoteURL,
		file.WithRemoteURL("https://example.com/paper.jpg"),
		file.WithMimeType("image/jpeg"),
	)

	messages := []PromptMessage{
		{
			Role:    PromptMessageRoleSystem,
			Content: "你是诊断助手。",
		},
	}

	processed, autoInjected, err := n.processVisionFiles(messages, []any{workflowFile}, true, ImageDetailHigh)
	if err != nil {
		t.Fatalf("processVisionFiles returned error: %v", err)
	}
	if !autoInjected {
		t.Fatalf("expected autoInjected=true when no explicit user prompt exists")
	}

	if len(processed) != 2 {
		t.Fatalf("expected system and synthesized user message, got %d messages", len(processed))
	}
	if processed[1].Role != PromptMessageRoleUser {
		t.Fatalf("expected synthesized message role to be user, got %s", processed[1].Role)
	}

	contentList, ok := processed[1].Content.([]PromptMessageContent)
	if !ok {
		t.Fatalf("expected synthesized user content to be multimodal, got %T", processed[1].Content)
	}
	if len(contentList) != 2 {
		t.Fatalf("expected image and default text content, got %d items", len(contentList))
	}
	if contentList[0].Type != PromptMessageContentTypeImage {
		t.Fatalf("expected first content item to be image, got %s", contentList[0].Type)
	}
	if contentList[1].Type != PromptMessageContentTypeText {
		t.Fatalf("expected second content item to be text, got %s", contentList[1].Type)
	}
	if contentList[1].Data != expectedPromptText {
		t.Fatalf("expected default vision prompt text %q, got %q", expectedPromptText, contentList[1].Data)
	}
}

func TestProcessVisionFiles_UsesRemoteURLFromMapInput(t *testing.T) {
	n := &Node{}

	processed, autoInjected, err := n.processVisionFiles(
		[]PromptMessage{{Role: PromptMessageRoleSystem, Content: "你是诊断助手。"}},
		[]any{
			map[string]any{
				"type":            "image",
				"transfer_method": "remote_url",
				"remote_url":      "https://example.com/files/paper.jpg",
				"mime_type":       "image/jpeg",
			},
		},
		true,
		ImageDetailHigh,
	)
	if err != nil {
		t.Fatalf("processVisionFiles returned error: %v", err)
	}
	if !autoInjected {
		t.Fatalf("expected autoInjected=true when only system prompt exists")
	}

	contentList, ok := processed[1].Content.([]PromptMessageContent)
	if !ok {
		t.Fatalf("expected synthesized user content to be multimodal, got %T", processed[1].Content)
	}
	if got := contentList[0].URL; got != "https://example.com/files/paper.jpg" {
		t.Fatalf("expected image URL from remote_url, got %q", got)
	}
}

func TestProcessVisionFiles_UsesSignedPreviewURLForLocalImage(t *testing.T) {
	setTestFileURLConfig(t, "https://api.zgi.im", "release")

	n := &Node{}

	workflowFile := file.NewFile(
		"tenant-1",
		file.FileTypeImage,
		file.FileTransferMethodLocalFile,
		file.WithID("file-1"),
		file.WithRelatedID("file-1"),
		file.WithMimeType("image/jpeg"),
	)

	processed, autoInjected, err := n.processVisionFiles(
		[]PromptMessage{{Role: PromptMessageRoleSystem, Content: "你是诊断助手。"}},
		[]any{workflowFile},
		true,
		ImageDetailHigh,
	)
	if err != nil {
		t.Fatalf("processVisionFiles returned error: %v", err)
	}
	if !autoInjected {
		t.Fatalf("expected autoInjected=true when only system prompt exists")
	}

	contentList, ok := processed[1].Content.([]PromptMessageContent)
	if !ok {
		t.Fatalf("expected synthesized user content to be multimodal, got %T", processed[1].Content)
	}
	if contentList[0].Base64 != "" {
		t.Fatalf("expected local image to use signed preview URL instead of inline base64")
	}
	if !strings.HasPrefix(contentList[0].URL, "https://api.zgi.im/console/api/files/file-1/file-preview?") {
		t.Fatalf("expected signed preview URL, got %q", contentList[0].URL)
	}
}

func TestProcessVisionFiles_UsesSignedPreviewURLForLocalImageFromWorkflowFileMap(t *testing.T) {
	setTestFileURLConfig(t, "https://api.zgi.im", "release")

	n := &Node{}

	workflowFile := file.NewFile(
		"tenant-1",
		file.FileTypeImage,
		file.FileTransferMethodLocalFile,
		file.WithID("file-1"),
		file.WithRelatedID("file-1"),
		file.WithMimeType("image/jpeg"),
	)

	processed, autoInjected, err := n.processVisionFiles(
		[]PromptMessage{{Role: PromptMessageRoleSystem, Content: "你是诊断助手。"}},
		[]any{workflowFile.ToDict()},
		true,
		ImageDetailHigh,
	)
	if err != nil {
		t.Fatalf("processVisionFiles returned error: %v", err)
	}
	if !autoInjected {
		t.Fatalf("expected autoInjected=true when only system prompt exists")
	}

	contentList, ok := processed[1].Content.([]PromptMessageContent)
	if !ok {
		t.Fatalf("expected synthesized user content to be multimodal, got %T", processed[1].Content)
	}
	if contentList[0].Base64 != "" {
		t.Fatalf("expected workflow file map to use signed preview URL instead of inline base64")
	}
	if !strings.HasPrefix(contentList[0].URL, "https://api.zgi.im/console/api/files/file-1/file-preview?") {
		t.Fatalf("expected signed preview URL from workflow file map, got %q", contentList[0].URL)
	}
}

func TestProcessVisionFiles_RejectsNonPublicSignedPreviewURLForLocalImage(t *testing.T) {
	setTestFileURLConfig(t, "http://localhost:2679", "release")

	n := &Node{}

	workflowFile := file.NewFile(
		"tenant-1",
		file.FileTypeImage,
		file.FileTransferMethodLocalFile,
		file.WithID("file-1"),
		file.WithRelatedID("file-1"),
		file.WithMimeType("image/jpeg"),
	)

	_, _, err := n.processVisionFiles(
		[]PromptMessage{{Role: PromptMessageRoleSystem, Content: "你是诊断助手。"}},
		[]any{workflowFile},
		true,
		ImageDetailHigh,
	)
	if err == nil {
		t.Fatalf("expected processVisionFiles to fail when FILES_URL is not public")
	}
	if !strings.Contains(err.Error(), "FILES_URL") {
		t.Fatalf("expected FILES_URL configuration error, got %v", err)
	}
}

func TestResolveFileURLFromID_UsesSignedPreviewURL(t *testing.T) {
	setTestFileURLConfig(t, "https://api.zgi.im", "release")

	n := &Node{}

	resolvedURL, err := n.resolveFileURLFromID("file-1")
	if err != nil {
		t.Fatalf("resolveFileURLFromID returned error: %v", err)
	}
	if !strings.HasPrefix(resolvedURL, "https://api.zgi.im/console/api/files/file-1/file-preview?") {
		t.Fatalf("expected signed preview URL from resolveFileURLFromID, got %q", resolvedURL)
	}
}
