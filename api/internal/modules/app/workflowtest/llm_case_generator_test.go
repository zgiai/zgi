package workflowtest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGeneratedCasesAcceptsJSONCodeFence(t *testing.T) {
	result, err := parseGeneratedCases("```json\n{\"cases\":[{\"content\":\"企业版支持哪些能力？\",\"question_type\":\"核心问题\"},{\"content\":\"如何接入 CRM？\",\"question_type\":\"扩展问法\"}]}\n```")

	require.NoError(t, err)
	require.Len(t, result.Cases, 2)
	require.Equal(t, CaseTypeCore, result.Cases[0].QuestionType)
	require.Equal(t, CaseTypeExtension, result.Cases[1].QuestionType)
}

func TestParseGeneratedCasesRejectsInvalidJSON(t *testing.T) {
	_, err := parseGeneratedCases("这里是非 JSON 文本")

	require.Error(t, err)
}
