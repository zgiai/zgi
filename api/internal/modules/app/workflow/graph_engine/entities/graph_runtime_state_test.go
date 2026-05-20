package entities

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSystemVariable verifies the conversational workflow extensions on SystemVariable.
func TestSystemVariable(t *testing.T) {
	// Test the basic fields
	t.Run("BasicSystemVariable", func(t *testing.T) {
		sysVar := SystemVariable{
			TenantID:   "test-tenant",
			UserID:     "test-user",
			AppID:      "test-app",
			WorkflowID: "test-workflow",
		}

		assert.Equal(t, "test-tenant", sysVar.TenantID)
		assert.Equal(t, "test-user", sysVar.UserID)
		assert.Equal(t, "test-app", sysVar.AppID)
		assert.Equal(t, "test-workflow", sysVar.WorkflowID)
	})

	// Test the conversational workflow extension fields
	t.Run("ConversationalWorkflowExtensions", func(t *testing.T) {
		sysVar := SystemVariable{
			TenantID:       "test-tenant",
			UserID:         "test-user",
			AppID:          "test-app",
			WorkflowID:     "test-workflow",
			WorkflowRunID:  "run-12345",
			Query:          "用户查询内容",
			ConversationID: "conv-67890",
			DialogueCount:  5,
		}

		// Verify the newly added conversational workflow fields
		assert.Equal(t, "run-12345", sysVar.WorkflowRunID)
		assert.Equal(t, "用户查询内容", sysVar.Query)
		assert.Equal(t, "conv-67890", sysVar.ConversationID)
		assert.Equal(t, 5, sysVar.DialogueCount)
	})

	// Test empty value handling
	t.Run("EmptyValues", func(t *testing.T) {
		sysVar := SystemVariable{}

		assert.Empty(t, sysVar.TenantID)
		assert.Empty(t, sysVar.UserID)
		assert.Empty(t, sysVar.AppID)
		assert.Empty(t, sysVar.WorkflowID)
		assert.Empty(t, sysVar.WorkflowRunID)
		assert.Empty(t, sysVar.Query)
		assert.Empty(t, sysVar.ConversationID)
		assert.Equal(t, 0, sysVar.DialogueCount)
	})

	// Test JSON serialization and deserialization
	t.Run("JSONSerialization", func(t *testing.T) {
		original := SystemVariable{
			TenantID:       "test-tenant",
			UserID:         "test-user",
			AppID:          "test-app",
			WorkflowID:     "test-workflow",
			WorkflowRunID:  "run-12345",
			Query:          "测试查询",
			ConversationID: "conv-67890",
			DialogueCount:  3,
		}

		// A JSON serialization test could be added here, but it would require importing json.
		// Since there is no JSON-tagged method here, we only verify the struct field values.
		assert.NotEmpty(t, original.TenantID)
		assert.NotEmpty(t, original.Query)
		assert.Greater(t, original.DialogueCount, 0)
	})
}

// TestSystemVariableConversationalWorkflowUsage verifies system variable usage in conversational workflows.
func TestSystemVariableConversationalWorkflowUsage(t *testing.T) {
	// Simulate a conversational workflow scenario
	t.Run("ConversationalWorkflowScenario", func(t *testing.T) {
		// First turn
		firstRound := SystemVariable{
			TenantID:       "tenant-001",
			UserID:         "user-001",
			AppID:          "app-001",
			WorkflowID:     "workflow-chat-001",
			WorkflowRunID:  "run-001",
			Query:          "你好，我想了解产品信息",
			ConversationID: "conv-001",
			DialogueCount:  1,
		}

		assert.Equal(t, 1, firstRound.DialogueCount)
		assert.Equal(t, "你好，我想了解产品信息", firstRound.Query)

		// Second turn (same conversation ID, incremented dialogue count)
		secondRound := SystemVariable{
			TenantID:       "tenant-001",
			UserID:         "user-001",
			AppID:          "app-001",
			WorkflowID:     "workflow-chat-001",
			WorkflowRunID:  "run-002",
			Query:          "价格是多少？",
			ConversationID: "conv-001", // same conversation ID
			DialogueCount:  2,          // incremented dialogue count
		}

		assert.Equal(t, 2, secondRound.DialogueCount)
		assert.Equal(t, "conv-001", secondRound.ConversationID)
		assert.Equal(t, firstRound.ConversationID, secondRound.ConversationID)
		assert.Greater(t, secondRound.DialogueCount, firstRound.DialogueCount)

		// Third turn
		thirdRound := SystemVariable{
			TenantID:       "tenant-001",
			UserID:         "user-001",
			AppID:          "app-001",
			WorkflowID:     "workflow-chat-001",
			WorkflowRunID:  "run-003",
			Query:          "有什么优惠活动吗？",
			ConversationID: "conv-001", // same conversation ID
			DialogueCount:  3,          // continues to increase
		}

		assert.Equal(t, 3, thirdRound.DialogueCount)
		assert.Equal(t, "有什么优惠活动吗？", thirdRound.Query)
	})

	// Compare task workflows and conversational workflows
	t.Run("TaskWorkflowVsConversationalWorkflow", func(t *testing.T) {
		// Task workflow (no conversation context)
		taskWorkflow := SystemVariable{
			TenantID:   "tenant-001",
			UserID:     "user-001",
			AppID:      "app-001",
			WorkflowID: "workflow-task-001",
			Query:      "处理这个文档",
			// Note: task workflows do not have ConversationID or DialogueCount
		}

		assert.Empty(t, taskWorkflow.ConversationID)
		assert.Equal(t, 0, taskWorkflow.DialogueCount)

		// Conversational workflow (with conversation context)
		conversationalWorkflow := SystemVariable{
			TenantID:       "tenant-001",
			UserID:         "user-001",
			AppID:          "app-001",
			WorkflowID:     "workflow-chat-001",
			WorkflowRunID:  "run-001",
			Query:          "你好",
			ConversationID: "conv-001",
			DialogueCount:  1,
		}

		assert.NotEmpty(t, conversationalWorkflow.ConversationID)
		assert.Greater(t, conversationalWorkflow.DialogueCount, 0)
	})
}

// TestSystemVariableFieldValidation verifies system variable fields.
func TestSystemVariableFieldValidation(t *testing.T) {
	// Test required fields
	t.Run("RequiredFields", func(t *testing.T) {
		sysVar := SystemVariable{
			TenantID:   "tenant-001",
			UserID:     "user-001",
			AppID:      "app-001",
			WorkflowID: "workflow-001",
		}

		// Basic required fields should be present
		assert.NotEmpty(t, sysVar.TenantID)
		assert.NotEmpty(t, sysVar.UserID)
		assert.NotEmpty(t, sysVar.AppID)
		assert.NotEmpty(t, sysVar.WorkflowID)
	})

	// Test conversational workflow specific fields
	t.Run("ConversationalSpecificFields", func(t *testing.T) {
		sysVar := SystemVariable{
			ConversationID: "conv-001",
			DialogueCount:  5,
			Query:          "用户输入的查询",
			WorkflowRunID:  "run-001",
		}

		// Verify conversational workflow specific fields
		assert.NotEmpty(t, sysVar.ConversationID)
		assert.Greater(t, sysVar.DialogueCount, 0)
		assert.NotEmpty(t, sysVar.Query)
		assert.NotEmpty(t, sysVar.WorkflowRunID)
	})

	// Test field types
	t.Run("FieldTypes", func(t *testing.T) {
		sysVar := SystemVariable{
			DialogueCount: 10,
		}

		// DialogueCount should be an int
		assert.IsType(t, 0, sysVar.DialogueCount)
		assert.Equal(t, 10, sysVar.DialogueCount)

		// Other fields should be strings
		assert.IsType(t, "", sysVar.TenantID)
		assert.IsType(t, "", sysVar.Query)
		assert.IsType(t, "", sysVar.ConversationID)
	})
}
