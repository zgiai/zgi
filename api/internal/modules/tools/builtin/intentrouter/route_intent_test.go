package intentrouter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestRouteIntentToolReturnsStablePayload(t *testing.T) {
	messages, err := NewRouteIntentTool("tenant-1").Invoke(
		context.Background(),
		"user-1",
		map[string]interface{}{
			"user_input":            "Export the current report as a Word document.",
			"intent_id":             "file_generation.docx",
			"task_type":             "file_generation",
			"subtype":               "docx",
			"confidence":            0.94,
			"recommended_action":    "call_skill",
			"recommended_skill_id":  "file-generator",
			"recommended_tool_name": "generate_docx",
			"routing_hints": map[string]interface{}{
				"requires_file_generation": true,
			},
			"evidence":           []interface{}{"User explicitly asked to export as a Word document."},
			"normalized_request": "Generate a DOCX file from the current report.",
		},
		nil,
		nil,
		nil,
	)

	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, tools.ToolInvokeMessageTypeJSON, messages[0].Type)

	payload := messages[0].Data
	require.Equal(t, "file_generation.docx", payload["intent_id"])
	require.Equal(t, "file_generation", payload["task_type"])
	require.Equal(t, "call_skill", payload["recommended_action"])
	require.Equal(t, "file-generator", payload["recommended_skill_id"])
	require.Equal(t, "generate_docx", payload["recommended_tool_name"])
	require.Equal(t, 0.94, payload["confidence"])
	require.Contains(t, payload, "missing_info")
	require.Contains(t, payload, "routing_hints")
}

func TestBuildIntentRouteAcceptsJSONStrings(t *testing.T) {
	payload, err := buildIntentRoute(map[string]interface{}{
		"user_input":         "Use this CSV to find trends.",
		"intent_id":          "data_analysis.trend",
		"task_type":          "data_analysis",
		"confidence":         0.78,
		"recommended_action": "answer_directly",
		"routing_hints":      `{"uses_uploaded_files":true}`,
		"uploaded_files":     `[{"filename":"scores.csv","mime_type":"text/csv"}]`,
		"evidence":           `["User asks to find trends in a CSV file."]`,
		"normalized_request": "Analyze the uploaded CSV for trends.",
	})

	require.NoError(t, err)
	require.Equal(t, "data_analysis", payload["task_type"])
	require.Len(t, payload["uploaded_files"], 1)
}

func TestBuildIntentRouteRejectsInvalidConfidence(t *testing.T) {
	_, err := buildIntentRoute(validRouteParams(map[string]interface{}{
		"confidence": 1.2,
	}))

	require.Error(t, err)
	require.Contains(t, err.Error(), "confidence must be between 0 and 1")
}

func TestBuildIntentRouteRejectsClarificationWithoutMissingInfo(t *testing.T) {
	_, err := buildIntentRoute(validRouteParams(map[string]interface{}{
		"recommended_action": "request_user_input",
	}))

	require.Error(t, err)
	require.Contains(t, err.Error(), "missing_info is required")
}

func TestBuildIntentRouteRejectsUnsupportedRoutingHint(t *testing.T) {
	_, err := buildIntentRoute(validRouteParams(map[string]interface{}{
		"routing_hints": map[string]interface{}{
			"unknown_hint": true,
		},
	}))

	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported routing_hints key")
}

func TestBuildIntentRouteRejectsCallSkillWithoutSkillID(t *testing.T) {
	_, err := buildIntentRoute(validRouteParams(map[string]interface{}{
		"recommended_action": "call_skill",
	}))

	require.Error(t, err)
	require.Contains(t, err.Error(), "recommended_skill_id is required")
}

func validRouteParams(overrides map[string]interface{}) map[string]interface{} {
	params := map[string]interface{}{
		"user_input":         "What is the reimbursement policy?",
		"intent_id":          "knowledge_retrieval.policy_lookup",
		"task_type":          "knowledge_retrieval",
		"confidence":         0.9,
		"recommended_action": "retrieve_knowledge",
		"routing_hints": map[string]interface{}{
			"requires_knowledge_base": true,
		},
		"evidence":           []interface{}{"User asks about a policy."},
		"normalized_request": "Retrieve policy guidance.",
	}
	for key, value := range overrides {
		params[key] = value
	}
	return params
}
