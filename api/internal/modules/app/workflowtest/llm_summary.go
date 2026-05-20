package workflowtest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

type LLMSummarizer struct {
	Client      llmclient.LLMClient
	WorkspaceID string
	AccountID   string
	Provider    string
	Model       string
}

func (s *LLMSummarizer) SummarizeBatch(ctx context.Context, req SummaryRequest) (*SummaryResult, error) {
	if s == nil || s.Client == nil {
		return nil, fmt.Errorf("llm summarizer is not configured")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	temperature := 0.2
	maxTokens := 600
	resp, err := s.Client.AppChat(timeoutCtx, &llmclient.AppContext{
		WorkspaceID:        s.WorkspaceID,
		BillingSubjectType: llmclient.BillingSubjectTypeWorkspace,
		AppID:              req.AgentID,
		AppType:            "agent",
		AccountID:          s.AccountID,
		SessionID:          req.Batch.ID,
	}, &adapter.ChatRequest{
		Provider: s.Provider,
		Model:    s.Model,
		Messages: []adapter.Message{
			{Role: "system", Content: "你是工作流自动化批量测试的总结助手。请基于单问题结果输出简洁、可执行的中文测试总结。"},
			{Role: "user", Content: buildSummaryPrompt(req)},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm summarizer returned empty response")
	}
	content, ok := resp.Choices[0].Message.Content.(string)
	if !ok || strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("llm summarizer returned empty content")
	}
	return &SummaryResult{Summary: strings.TrimSpace(content)}, nil
}

func buildSummaryPrompt(req SummaryRequest) string {
	items := make([]map[string]interface{}, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, map[string]interface{}{
			"question":         item.CaseSnapshot.Content,
			"status":           item.Status,
			"judge_reason":     item.JudgeReason,
			"judge_suggestion": item.JudgeSuggestion,
			"error":            item.Error,
		})
	}
	itemsJSON, _ := json.Marshal(items)
	return fmt.Sprintf(`批次名称：%s
问题数量：%d
通过：%d
不通过：%d
需复核：%d

单问题结果：
%s

请输出 1 段中文总结，包含整体结论、主要风险和下一步建议。`, req.Batch.Name, req.Batch.CaseCount, req.Batch.PassedCount, req.Batch.FailedCount, req.Batch.ReviewCount, string(itemsJSON))
}
