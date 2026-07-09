package workflowtest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTaskExpectedChecksNormalizesFlexibleShapes(t *testing.T) {
	checks, err := parseTaskExpectedChecks(`{
		"must_visit_nodes": {"1": "文档解析", "2": "摘要生成"},
		"must_call_tools": "file_parser",
		"output_contains": ["甲方", "乙方", "金额"],
		"max_latency_ms": "30000"
	}`)

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"1: 文档解析", "2: 摘要生成"}, checks.MustVisitNodes)
	require.Equal(t, []string{"file_parser"}, checks.MustCallTools)
	require.Equal(t, []string{"甲方", "乙方", "金额"}, checks.OutputContains)
	require.Equal(t, 30000, checks.MaxLatencyMS)
	require.True(t, checks.Useful())
}

func TestExpectedChecksFromInputDetectsUsefulChecks(t *testing.T) {
	checks := expectedChecksFromInput(JSONMap{
		"output_contains": []interface{}{"甲方", "乙方"},
	})

	require.True(t, checks.Useful())
	require.Equal(t, []string{"甲方", "乙方"}, checks.OutputContains)
}
