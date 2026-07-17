package workflowtest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTaskExpectedChecksSupportsConditions(t *testing.T) {
	checks, err := parseTaskExpectedChecks(`{
		"conditions": [
			{
				"type": "node",
				"operator": "input_contains",
				"target_id": "start",
				"target_label": "Start",
				"target_type": "start",
				"values": ["document_type"],
				"severity": "critical"
			},
			{
				"type": "output_contains",
				"operator": "contains",
				"values": ["contract amount"],
				"match_mode": "keyword"
			}
		]
	}`)

	require.NoError(t, err)
	require.True(t, checks.Useful())
	require.Len(t, checks.Conditions, 2)
	require.Equal(t, "input_contains", checks.Conditions[0].Operator)
	require.Equal(t, "ai_generated", checks.Conditions[0].Source)
	require.Equal(t, "keyword", checks.Conditions[1].MatchMode)
}

func TestTaskExpectedChecksJSONMapBackfillsConditionsFromLegacyFields(t *testing.T) {
	payload := TaskExpectedChecks{
		MustCallTools:  []string{"file_parser"},
		OutputContains: []string{"contract amount"},
		MaxLatencyMS:   30000,
	}.JSONMap()

	conditions, ok := payload["conditions"].([]TaskExpectedCheckCondition)
	require.True(t, ok)
	require.Len(t, conditions, 3)
	require.Equal(t, "capability", conditions[0].Type)
	require.Equal(t, "output_contains", conditions[1].Type)
	require.Equal(t, "latency", conditions[2].Type)
	require.Equal(t, []string{"contract amount"}, payload["output_contains"])
}

func TestNormalizeExpectedCheckConditionsDeduplicatesIDs(t *testing.T) {
	conditions := normalizeExpectedCheckConditions([]TaskExpectedCheckCondition{
		{ID: "check_no_hallucination_2", Type: "output_contains", Operator: "contains", Values: []string{"A"}},
		{ID: "check_no_hallucination_2", Type: "output_contains", Operator: "contains", Values: []string{"B"}},
	}, TaskExpectedChecks{})

	require.Len(t, conditions, 2)
	require.Equal(t, "check_no_hallucination_2", conditions[0].ID)
	require.Equal(t, "check_no_hallucination_2_2", conditions[1].ID)
}

func TestNormalizeConversationExpectedChecksDeduplicatesIDs(t *testing.T) {
	checks := normalizeConversationExpectedChecks(ConversationExpectedChecks{Conditions: []ConversationExpectedCheckCondition{
		{ID: "check_no_hallucination_2", Type: "no_hallucination", Operator: "passed", Values: []string{"不编造"}},
		{ID: "check_no_hallucination_2", Type: "no_hallucination", Operator: "passed", Values: []string{"不编造事实"}},
	}})

	require.Len(t, checks.Conditions, 2)
	require.Equal(t, "check_no_hallucination_2", checks.Conditions[0].ID)
	require.Equal(t, "check_no_hallucination_2_2", checks.Conditions[1].ID)
}
