package workflowtest

import (
	"context"
)

type JudgeRequest struct {
	AgentID        string
	BatchID        string
	BatchItemID    string
	PromptTemplate string
	CaseSnapshot   CaseSnapshot
	RunResult      RunCaseResult
}

type JudgeResult struct {
	Status     BatchItemStatus
	Reason     string
	Suggestion string
	Confidence float64
}

type Judge interface {
	JudgeCase(ctx context.Context, req JudgeRequest) (*JudgeResult, error)
}

func runJudge(ctx context.Context, judge Judge, req JudgeRequest) *JudgeResult {
	if judge == nil {
		return &JudgeResult{
			Status:     BatchItemStatusReview,
			Reason:     "judge is not configured",
			Suggestion: "configure AI scoring and rerun, or review manually",
			Confidence: 0,
		}
	}
	result, err := judge.JudgeCase(ctx, req)
	if err != nil {
		return &JudgeResult{
			Status:     BatchItemStatusReview,
			Reason:     judgeFailureReason(err),
			Suggestion: judgeFailureSuggestion(err),
			Confidence: 0,
		}
	}
	return normalizeJudgeResult(result)
}

func judgeFailureReason(err error) string {
	if err == nil {
		return "AI 评分失败，请人工复核本次结果。"
	}
	if isModelUnavailableError(err) {
		return "AI 评分失败：" + modelUnavailableReason
	}
	return "AI 评分失败，请人工复核本次结果。"
}

func judgeFailureSuggestion(err error) string {
	if isModelUnavailableError(err) {
		return modelUnavailableAction
	}
	return "AI 评分失败，请人工复核或重新测试。"
}

func normalizeJudgeResult(result *JudgeResult) *JudgeResult {
	if result == nil {
		return &JudgeResult{
			Status:     BatchItemStatusReview,
			Reason:     "judge returned empty result",
			Suggestion: "review this result manually",
			Confidence: 0,
		}
	}
	if result.Status != BatchItemStatusPassed && result.Status != BatchItemStatusFailed && result.Status != BatchItemStatusReview {
		result.Status = BatchItemStatusReview
	}
	if result.Confidence < 0 {
		result.Confidence = 0
	}
	if result.Confidence > 1 {
		result.Confidence = 1
	}
	return result
}
