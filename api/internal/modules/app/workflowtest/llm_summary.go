package workflowtest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
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
		AppType:            "workflow",
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
	return &SummaryResult{Summary: sanitizeSummaryText(content)}, nil
}

func sanitizeSummaryText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "**", "")
	return strings.TrimSpace(value)
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

请按以下结构输出：

测试结论：
用 1-2 句话说明本次测试覆盖情况、通过率和整体表现。重点说明当前结果是稳定、存在风险，还是需要重点优化。不要堆砌数字，数字只保留关键指标。

主要问题：
从不通过和需复核用例中归纳 1-3 个核心问题。问题应基于失败原因，而不是简单复述用例。如果失败原因不集中，请说明“问题较为分散”，并按影响较大的方向归纳。

优化建议：
针对主要问题给出可执行的优化方向。建议必须和失败原因对应，避免“优化提示词”“提升准确率”“完善知识库”这类泛泛表述单独出现。

输出要求：
1. 使用中文，语气专业、简洁，适合产品页面展示。
2. 不要输出“下轮验证”“后续测试计划”等内容。
3. 不要编造未提供的数据、场景或失败原因。
4. 如果执行异常数量大于 0，需要单独说明执行异常属于测试链路问题，不应和业务不通过混为一类。
5. 如果通过率较高且问题较少，不要强行放大风险。
6. 总字数控制在 200-350 字。
7. 输出为连续正文，但保留三个小标题：测试结论、主要问题、优化建议。
8. 不要使用 Markdown 强调符号，例如 **。`, req.Batch.Name, req.Batch.CaseCount, req.Batch.PassedCount, req.Batch.FailedCount, req.Batch.ReviewCount, string(itemsJSON))
}
