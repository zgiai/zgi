package workflowtest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseScenarioRecognitionResponseAcceptsJSONCodeFence(t *testing.T) {
	result, err := parseScenarioRecognitionResponse("```json\n{\"scenarios\":[{\"name\":\"售前咨询\",\"description\":\"产品能力咨询\"}],\"assignments\":[{\"case_id\":\"case-1\",\"scenario_name\":\"售前咨询\"}]}\n```")

	require.NoError(t, err)
	require.Len(t, result.Scenarios, 1)
	require.Equal(t, "售前咨询", result.Scenarios[0].Name)
	require.Len(t, result.Assignments, 1)
	require.Equal(t, "case-1", result.Assignments[0].CaseID)
}

func TestParseScenarioRecognitionResponseRejectsEmptyResult(t *testing.T) {
	_, err := parseScenarioRecognitionResponse(`{"scenarios":[{"name":"","description":"空名称"}]}`)

	require.Error(t, err)
	require.Contains(t, err.Error(), "scenario recognition result is empty")
}
