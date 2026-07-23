package workflowtest

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type failingSummarizer struct {
	err error
}

func (s failingSummarizer) SummarizeBatch(ctx context.Context, req SummaryRequest) (*SummaryResult, error) {
	return nil, s.err
}

func TestRunSummarizerUsesUserFacingMessageWhenModelIsUnavailable(t *testing.T) {
	batch := Batch{FailedCount: 1, ReviewCount: 2}
	summary := runSummarizer(context.Background(), failingSummarizer{
		err: errors.New(`failed to select provider: model not found: current workspace has no enabled route for model "qwen-max"`),
	}, SummaryRequest{Batch: batch})

	require.Contains(t, summary, "AI 总结生成失败：当前默认模型不可用，请前往默认模型管理修改默认文本模型后重新测试。")
	require.Contains(t, summary, "本次测试有 1 个问题未通过，2 个问题需要复核。")
	require.NotContains(t, summary, "qwen-max")
}

func TestRunSummarizerUsesPricingMessageWhenModelPricingIsMissing(t *testing.T) {
	summary := runSummarizer(context.Background(), failingSummarizer{
		err: errors.New("all providers failed: failed to calculate credits: billing operation failed: model_pricing_not_configured"),
	}, SummaryRequest{Batch: Batch{FailedCount: 1}})

	require.Contains(t, summary, "AI 总结生成失败：当前模型未配置计费价格")
	require.NotContains(t, summary, "all providers failed")
	require.NotContains(t, summary, "model_pricing_not_configured")
}

func TestRunSummarizerDoesNotTreatProviderFailureAsMissingModel(t *testing.T) {
	summary := runSummarizer(context.Background(), failingSummarizer{
		err: errors.New("all providers failed: current user api does not support http call: upstream service error"),
	}, SummaryRequest{Batch: Batch{}})

	require.Contains(t, summary, "AI 总结生成失败，请稍后重试或人工查看明细。")
	require.NotContains(t, summary, "当前默认模型不可用")
	require.NotContains(t, summary, "all providers failed")
}

func TestRunSummarizerUsesGenericUserFacingMessageForUnknownError(t *testing.T) {
	summary := runSummarizer(context.Background(), failingSummarizer{
		err: errors.New("temporary network failure"),
	}, SummaryRequest{Batch: Batch{}})

	require.Contains(t, summary, "AI 总结生成失败，请稍后重试或人工查看明细。")
	require.NotContains(t, summary, "temporary network failure")
}
