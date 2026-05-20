package workflowtest

import (
	"context"
	"fmt"
)

type SummaryRequest struct {
	AgentID string
	Batch   Batch
	Items   []BatchItem
}

type SummaryResult struct {
	Summary string
}

type Summarizer interface {
	SummarizeBatch(ctx context.Context, req SummaryRequest) (*SummaryResult, error)
}

func runSummarizer(ctx context.Context, summarizer Summarizer, req SummaryRequest) string {
	if summarizer != nil {
		result, err := summarizer.SummarizeBatch(ctx, req)
		if err == nil && result != nil && result.Summary != "" {
			return result.Summary
		}
		if err != nil {
			return fmt.Sprintf("AI 总结生成失败：%v。%s", err, fallbackBatchSummary(req.Batch, req.Items))
		}
	}
	return fallbackBatchSummary(req.Batch, req.Items)
}

func fallbackBatchSummary(batch Batch, items []BatchItem) string {
	failed := batch.FailedCount
	review := batch.ReviewCount
	if failed == 0 && review == 0 {
		return "本次测试全部通过，暂未发现需要处理的问题。"
	}
	return fmt.Sprintf("本次测试有 %d 个问题未通过，%d 个问题需要复核。建议优先查看不通过问题的评分原因和改进建议。", failed, review)
}
