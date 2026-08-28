package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestWriteAgentContextPromptDumpUsesPerRequestMetricsAndSequentialFiles(t *testing.T) {
	directory := t.TempDir()
	request := &adapter.ChatRequest{
		Provider:  "openai",
		Model:     "gpt-5",
		MaxTokens: intPtr(16000),
		Messages: []adapter.Message{
			{Role: "system", Content: "system text"},
			{Role: "user", Content: "This session is being continued from a previous conversation that ran out of context.\n\nSummary:\nsummary text"},
			{Role: "user", Content: "old question"},
			{Role: "assistant", Content: "old answer"},
			{Role: "user", Content: "current question"},
		},
		Tools: []adapter.Tool{{Type: "function", Function: adapter.Function{Name: "read_file", Description: "Read a file", Parameters: map[string]interface{}{"type": "object"}}}},
	}
	decision := &contextmgr.Decision{
		AgentRunID:         "run-1",
		APIRound:           4,
		RequestType:        contextmgr.RequestTypeMain,
		Action:             contextmgr.DecisionSemanticCompact,
		FixedRequestTokens: 18104,
		CompressibleTokens: 75330,
		FinalPromptTokens:  93434,
		BeforeTokens:       230000,
		AfterTokens:        93434,
		Estimator:          "o200k_base",
		EstimateScale:      1,
		Budget: contextmgr.Budget{
			ModelContextWindow:        1_000_000,
			ConfiguredAgentWindowK:    256,
			AgentContextWindow:        256_000,
			AgentContextWindowClamped: true,
			PromptBudget:              240_000,
			SoftLimit:                 223_000,
			HardLimit:                 240_000,
			TargetTokens:              144_000,
			ContextPressure:           float64(93434) / 240000,
		},
	}

	first, err := writeAgentContextPromptDump(directory, time.Date(2026, 8, 13, 15, 30, 0, 0, time.Local), "agent_tool_loop", 3, decision, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := writeAgentContextPromptDump(directory, time.Now(), "main", 5, decision, request)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(first) != "000001.txt" || filepath.Base(second) != "000002.txt" {
		t.Fatalf("dump files = %q, %q", first, second)
	}
	encoded, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	content := string(encoded)
	for _, expected := range []string{
		"AgentRunID: run-1", "API Round: 4", "请求类型: main", "模型: gpt-5", "模型物理上下文窗口: 1000000",
		"配置 Agent 工作窗口(k): 256", "有效 Agent 工作窗口 Token: 256000", "PromptBudget: 240000",
		"固定请求 Token: 18104", "可压缩区 Token: 75330", "最终 Prompt Token: 93434", "总上下文压力: 38.93%",
		"SoftLimit: 223000", "HardLimit: 240000", "TargetTokens: 144000", "本次决策: semantic_compact",
		"【最终完整 ChatRequest】", `"provider": "openai"`, `"messages": [`, "system text", "summary text",
		"old question", "old answer", "current question", `"tools": [`, `"name": "read_file"`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("dump missing %q:\n%s", expected, content)
		}
	}
	for _, removed := range []string{
		"历史压力", "历史预算 Token", "摘要阈值",
		"【1. 系统提示词｜固定部分】", "【2. 历史摘要｜历史部分】",
		"【3. 原始历史对话与 Agent transcript｜历史部分】", "【4. 补充上下文｜固定部分】",
		"【5. 当前用户问题｜固定部分】", "【6. 工具定义｜固定部分】", "【最终实际 Messages】",
	} {
		if strings.Contains(content, removed) {
			t.Fatalf("dump retained removed metric %q:\n%s", removed, content)
		}
	}
}

func TestRenderAgentContextPromptDumpOnlyContainsMetricsAndCompleteRequest(t *testing.T) {
	content, err := renderAgentContextPromptDump(time.Now(), "main", 1, nil, &adapter.ChatRequest{Model: "test-model", Messages: []adapter.Message{{Role: "user", Content: "question"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"模型: test-model", "本次决策: none", "【最终完整 ChatRequest】", `"content": "question"`} {
		if !strings.Contains(content, expected) {
			t.Fatalf("dump missing %q:\n%s", expected, content)
		}
	}
	if strings.Count(content, "【") != 1 {
		t.Fatalf("dump contains extra sections:\n%s", content)
	}
}

func TestContextPromptDumpIsExplicitlyEnabled(t *testing.T) {
	previousConfig := config.GlobalConfig
	t.Cleanup(func() {
		config.GlobalConfig = previousConfig
	})

	config.GlobalConfig = nil
	if contextPromptDumpEnabled() {
		t.Fatal("prompt dump must default to disabled")
	}
	config.GlobalConfig = &config.Config{
		ChatRuntime: config.ChatRuntimeConfig{ContextPromptDumpEnabled: true},
	}
	if !contextPromptDumpEnabled() {
		t.Fatal("prompt dump was not enabled")
	}
}

func intPtr(value int) *int { return &value }
