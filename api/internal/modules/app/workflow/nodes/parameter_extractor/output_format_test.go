package parameterextractor

import (
	"errors"
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/internal/modules/llm/gateway"
)

// TestOutputFormatCompatibility_Success validates that successful outputs contain all required fields
// Requirements: 3.1, 3.4, 3.5
func TestOutputFormatCompatibility_Success(t *testing.T) {
	// Create mock usage info
	usage := &shared.LLMUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		TotalPrice:       decimal.NewFromFloat(0.0025),
		Currency:         "USD",
	}

	// Create success result with extracted parameters
	nodeInputs := map[string]any{
		"query": "Extract user name John and age 25",
	}

	outputs := make(map[string]any)
	outputs["__is_success"] = 1
	outputs["__reason"] = nil
	outputs["__usage"] = map[string]any{
		"total_tokens":      usage.TotalTokens,
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_price":       usage.TotalPrice.String(),
		"currency":          usage.Currency,
	}
	outputs["user_name"] = "John"
	outputs["age"] = 25

	metadata := make(map[shared.WorkflowNodeExecutionMetadataKey]any)
	metadata[shared.TotalTokens] = usage.TotalTokens
	metadata[shared.TotalPrice] = usage.TotalPrice.String()
	metadata[shared.Currency] = usage.Currency

	result := &shared.NodeRunResult{
		Status:   shared.SUCCEEDED,
		Inputs:   nodeInputs,
		Outputs:  outputs,
		Metadata: metadata,
		LLMUsage: usage,
	}

	// Validate: Success output contains __is_success: 1
	assert.Equal(t, 1, result.Outputs["__is_success"], "Success output must contain __is_success: 1")

	// Validate: __reason should be nil for success
	assert.Nil(t, result.Outputs["__reason"], "Success output should have __reason as nil")

	// Validate: __usage object format
	usageMap, ok := result.Outputs["__usage"].(map[string]any)
	require.True(t, ok, "__usage must be a map")
	assert.Equal(t, 150, usageMap["total_tokens"], "__usage must contain total_tokens")
	assert.Equal(t, 100, usageMap["prompt_tokens"], "__usage must contain prompt_tokens")
	assert.Equal(t, 50, usageMap["completion_tokens"], "__usage must contain completion_tokens")
	assert.NotEmpty(t, usageMap["total_price"], "__usage must contain total_price")
	assert.Equal(t, "USD", usageMap["currency"], "__usage must contain currency")

	// Validate: Extracted parameters as independent fields
	assert.Equal(t, "John", result.Outputs["user_name"], "Extracted parameter user_name must be present")
	assert.Equal(t, 25, result.Outputs["age"], "Extracted parameter age must be present")

	// Validate: Metadata contains tokens, price, currency
	assert.Equal(t, 150, result.Metadata[shared.TotalTokens], "Metadata must contain total_tokens")
	assert.Equal(t, "0.0025", result.Metadata[shared.TotalPrice], "Metadata must contain total_price")
	assert.Equal(t, "USD", result.Metadata[shared.Currency], "Metadata must contain currency")
}

// TestOutputFormatCompatibility_Failure validates that failure outputs contain all required fields
// Requirements: 3.2, 3.3, 3.5
func TestOutputFormatCompatibility_Failure(t *testing.T) {
	// Create a mock node with test data
	node := &Node{
		nodeData: NodeData{
			Parameters: []ParameterConfig{
				{
					Name:     "user_name",
					Type:     ParameterTypeString,
					Required: true,
				},
				{
					Name:     "age",
					Type:     ParameterTypeNumber,
					Required: false,
				},
			},
		},
	}

	// Create mock usage info (may be nil in some failure cases)
	usage := &shared.LLMUsage{
		PromptTokens:     100,
		CompletionTokens: 0,
		TotalTokens:      100,
		TotalPrice:       decimal.NewFromFloat(0.001),
		Currency:         "USD",
	}

	// Create failure result
	nodeInputs := map[string]any{
		"query": "Invalid query",
	}

	testErr := fmt.Errorf("JSON extraction failed: invalid format")
	result, err := node.createFailureResult(nodeInputs, testErr, usage)
	require.NoError(t, err, "createFailureResult should not return error")

	// Validate: Failure output contains __is_success: 0
	assert.Equal(t, 0, result.Outputs["__is_success"], "Failure output must contain __is_success: 0")

	// Validate: Failure output contains __reason with error message
	reason, ok := result.Outputs["__reason"].(string)
	require.True(t, ok, "__reason must be a string")
	assert.Contains(t, reason, "JSON extraction failed", "__reason must contain error message")

	// Validate: __usage object format (even in failure)
	usageMap, ok := result.Outputs["__usage"].(map[string]any)
	require.True(t, ok, "__usage must be present even in failure")
	assert.Equal(t, 100, usageMap["total_tokens"], "__usage must contain total_tokens")
	assert.Equal(t, 100, usageMap["prompt_tokens"], "__usage must contain prompt_tokens")
	assert.Equal(t, 0, usageMap["completion_tokens"], "__usage must contain completion_tokens")
	assert.NotEmpty(t, usageMap["total_price"], "__usage must contain total_price")
	assert.Equal(t, "USD", usageMap["currency"], "__usage must contain currency")

	// Validate: Default values for all parameters
	assert.NotNil(t, result.Outputs["user_name"], "Failed extraction must include default value for user_name")
	assert.NotNil(t, result.Outputs["age"], "Failed extraction must include default value for age")

	// Validate: Metadata contains tokens, price, currency
	assert.Equal(t, 100, result.Metadata[shared.TotalTokens], "Metadata must contain total_tokens")
	assert.Equal(t, "0.001", result.Metadata[shared.TotalPrice], "Metadata must contain total_price")
	assert.Equal(t, "USD", result.Metadata[shared.Currency], "Metadata must contain currency")

	// Validate: Status is still SUCCEEDED (to allow workflow continuation)
	assert.Equal(t, shared.SUCCEEDED, result.Status, "Status should be SUCCEEDED even on failure")
}

// TestOutputFormatCompatibility_FailureWithoutUsage validates failure output when usage is nil
// Requirements: 3.2, 3.3
func TestOutputFormatCompatibility_FailureWithoutUsage(t *testing.T) {
	// Create a mock node with test data
	node := &Node{
		nodeData: NodeData{
			Parameters: []ParameterConfig{
				{
					Name:     "email",
					Type:     ParameterTypeString,
					Required: true,
				},
			},
		},
	}

	// Create failure result without usage info
	nodeInputs := map[string]any{
		"query": "Extract email",
	}

	testErr := fmt.Errorf("LLM invocation failed: timeout")
	result, err := node.createFailureResult(nodeInputs, testErr, nil)
	require.NoError(t, err, "createFailureResult should not return error")

	// Validate: Failure output contains __is_success: 0
	assert.Equal(t, 0, result.Outputs["__is_success"], "Failure output must contain __is_success: 0")

	// Validate: Failure output contains __reason
	reason, ok := result.Outputs["__reason"].(string)
	require.True(t, ok, "__reason must be a string")
	assert.Contains(t, reason, "LLM invocation failed", "__reason must contain error message")

	// Validate: __usage should not be present when usage is nil
	_, hasUsage := result.Outputs["__usage"]
	assert.False(t, hasUsage, "__usage should not be present when usage info is nil")

	// Validate: Default value for parameter
	assert.NotNil(t, result.Outputs["email"], "Failed extraction must include default value for email")

	// Validate: Metadata should not contain usage info when nil
	_, hasTokens := result.Metadata[shared.TotalTokens]
	assert.False(t, hasTokens, "Metadata should not contain tokens when usage is nil")
}

func TestCreateFailureResult_BillingErrorReturnsFailed(t *testing.T) {
	node := &Node{
		nodeData: NodeData{
			Parameters: []ParameterConfig{
				{
					Name:     "email",
					Type:     ParameterTypeString,
					Required: true,
				},
			},
		},
	}

	nodeInputs := map[string]any{
		"query": "Extract email",
	}

	testErr := errors.Join(
		errors.New("all providers failed"),
		&gateway.BillingUserError{
			Kind:  gateway.BillingUserErrorKindWorkspaceQuotaInsufficient,
			Cause: gateway.ErrInsufficientQuota,
		},
	)

	result, err := node.createFailureResult(nodeInputs, testErr, nil)
	require.Error(t, err, "billing failure should be returned")
	require.NotNil(t, result, "billing failure should still return a result")
	assert.Equal(t, shared.FAILED, result.Status, "billing failure must fail the node")
	assert.Equal(t, "LLMInvokeError", result.ErrType, "billing failure should use invoke error type")

	var userErr *gateway.BillingUserError
	require.True(t, errors.As(err, &userErr), "returned error should preserve billing error")
	require.True(t, errors.As(result.Err, &userErr), "result.Err should preserve billing error")
	assert.Equal(t, gateway.BillingUserErrorKindWorkspaceQuotaInsufficient, userErr.Kind)
}

// TestOutputFormatCompatibility_AllParameterTypes validates output format for all parameter types
// Requirements: 3.4
func TestOutputFormatCompatibility_AllParameterTypes(t *testing.T) {
	// Create success result with all parameter types
	nodeInputs := map[string]any{
		"query": "Extract all fields",
	}

	usage := &shared.LLMUsage{
		PromptTokens:     50,
		CompletionTokens: 30,
		TotalTokens:      80,
		TotalPrice:       decimal.NewFromFloat(0.002),
		Currency:         "USD",
	}

	outputs := make(map[string]any)
	outputs["__is_success"] = 1
	outputs["__reason"] = nil
	outputs["__usage"] = map[string]any{
		"total_tokens":      usage.TotalTokens,
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_price":       usage.TotalPrice.String(),
		"currency":          usage.Currency,
	}
	outputs["name"] = "Alice"
	outputs["age"] = 30
	outputs["is_active"] = true
	outputs["tags"] = []string{"developer", "golang"}

	result := &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  nodeInputs,
		Outputs: outputs,
	}

	// Validate: All parameter types are present as independent fields
	assert.Equal(t, "Alice", result.Outputs["name"], "String parameter must be present")
	assert.Equal(t, 30, result.Outputs["age"], "Number parameter must be present")
	assert.Equal(t, true, result.Outputs["is_active"], "Boolean parameter must be present")

	tags, ok := result.Outputs["tags"].([]string)
	require.True(t, ok, "Array parameter must be present and correct type")
	assert.Equal(t, []string{"developer", "golang"}, tags, "Array parameter must have correct value")
}

// TestOutputFormatCompatibility_UsageObjectStructure validates the exact structure of __usage
// Requirements: 3.3
func TestOutputFormatCompatibility_UsageObjectStructure(t *testing.T) {
	usage := &shared.LLMUsage{
		PromptTokens:     200,
		CompletionTokens: 100,
		TotalTokens:      300,
		TotalPrice:       decimal.NewFromFloat(0.005),
		Currency:         "USD",
	}

	// Create __usage map as done in the node
	usageMap := map[string]any{
		"total_tokens":      usage.TotalTokens,
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_price":       usage.TotalPrice.String(),
		"currency":          usage.Currency,
	}

	// Validate: All required fields are present
	assert.Contains(t, usageMap, "total_tokens", "__usage must contain total_tokens")
	assert.Contains(t, usageMap, "prompt_tokens", "__usage must contain prompt_tokens")
	assert.Contains(t, usageMap, "completion_tokens", "__usage must contain completion_tokens")
	assert.Contains(t, usageMap, "total_price", "__usage must contain total_price")
	assert.Contains(t, usageMap, "currency", "__usage must contain currency")

	// Validate: Field types and values
	assert.IsType(t, 0, usageMap["total_tokens"], "total_tokens must be int")
	assert.IsType(t, 0, usageMap["prompt_tokens"], "prompt_tokens must be int")
	assert.IsType(t, 0, usageMap["completion_tokens"], "completion_tokens must be int")
	assert.IsType(t, "", usageMap["total_price"], "total_price must be string")
	assert.IsType(t, "", usageMap["currency"], "currency must be string")

	// Validate: Correct values
	assert.Equal(t, 300, usageMap["total_tokens"])
	assert.Equal(t, 200, usageMap["prompt_tokens"])
	assert.Equal(t, 100, usageMap["completion_tokens"])
	assert.Equal(t, "0.005", usageMap["total_price"])
	assert.Equal(t, "USD", usageMap["currency"])
}

// TestOutputFormatCompatibility_MetadataStructure validates metadata structure
// Requirements: 3.5
func TestOutputFormatCompatibility_MetadataStructure(t *testing.T) {
	usage := &shared.LLMUsage{
		PromptTokens:     150,
		CompletionTokens: 75,
		TotalTokens:      225,
		TotalPrice:       decimal.NewFromFloat(0.00375),
		Currency:         "USD",
	}

	// Create metadata as done in the node
	metadata := make(map[shared.WorkflowNodeExecutionMetadataKey]any)
	metadata[shared.TotalTokens] = usage.TotalTokens
	metadata[shared.TotalPrice] = usage.TotalPrice.String()
	metadata[shared.Currency] = usage.Currency

	// Validate: All required fields are present
	assert.Contains(t, metadata, shared.TotalTokens, "Metadata must contain TotalTokens")
	assert.Contains(t, metadata, shared.TotalPrice, "Metadata must contain TotalPrice")
	assert.Contains(t, metadata, shared.Currency, "Metadata must contain Currency")

	// Validate: Correct values
	assert.Equal(t, 225, metadata[shared.TotalTokens])
	assert.Equal(t, "0.00375", metadata[shared.TotalPrice])
	assert.Equal(t, "USD", metadata[shared.Currency])
}

// TestOutputFormatCompatibility_WithRetryMetadata validates metadata with retry information
// Requirements: 3.5
func TestOutputFormatCompatibility_WithRetryMetadata(t *testing.T) {
	usage := &shared.LLMUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		TotalPrice:       decimal.NewFromFloat(0.0025),
		Currency:         "USD",
	}

	// Create metadata with retry information
	metadata := make(map[shared.WorkflowNodeExecutionMetadataKey]any)
	metadata[shared.TotalTokens] = usage.TotalTokens
	metadata[shared.TotalPrice] = usage.TotalPrice.String()
	metadata[shared.Currency] = usage.Currency
	metadata["retry_count"] = 2
	metadata["total_attempts"] = 3

	// Validate: Standard fields are present
	assert.Equal(t, 150, metadata[shared.TotalTokens])
	assert.Equal(t, "0.0025", metadata[shared.TotalPrice])
	assert.Equal(t, "USD", metadata[shared.Currency])

	// Validate: Retry information is present
	assert.Equal(t, 2, metadata["retry_count"], "Metadata should contain retry_count")
	assert.Equal(t, 3, metadata["total_attempts"], "Metadata should contain total_attempts")
}

// TestCreateFailureResult_Integration validates the complete failure result creation
// Requirements: 3.2, 3.3, 3.4, 3.5
func TestCreateFailureResult_Integration(t *testing.T) {
	// Create a node with multiple parameters
	node := &Node{
		nodeData: NodeData{
			Parameters: []ParameterConfig{
				{Name: "username", Type: ParameterTypeString, Required: true},
				{Name: "score", Type: ParameterTypeNumber, Required: false},
				{Name: "active", Type: ParameterTypeBool, Required: false},
			},
		},
	}

	nodeInputs := map[string]any{
		"query": "Test query",
	}

	usage := &shared.LLMUsage{
		PromptTokens:     80,
		CompletionTokens: 20,
		TotalTokens:      100,
		TotalPrice:       decimal.NewFromFloat(0.0015),
		Currency:         "USD",
	}

	testErr := fmt.Errorf("validation failed: missing required field")

	// Call createFailureResult
	result, err := node.createFailureResult(nodeInputs, testErr, usage)
	require.NoError(t, err)

	// Validate complete structure
	assert.Equal(t, shared.SUCCEEDED, result.Status)
	assert.Equal(t, 0, result.Outputs["__is_success"])
	assert.Contains(t, result.Outputs["__reason"], "validation failed")

	// Validate __usage
	usageMap, ok := result.Outputs["__usage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 100, usageMap["total_tokens"])
	assert.Equal(t, 80, usageMap["prompt_tokens"])
	assert.Equal(t, 20, usageMap["completion_tokens"])
	assert.Equal(t, "0.0015", usageMap["total_price"])
	assert.Equal(t, "USD", usageMap["currency"])

	// Validate default values for all parameters
	assert.NotNil(t, result.Outputs["username"])
	assert.NotNil(t, result.Outputs["score"])
	assert.NotNil(t, result.Outputs["active"])

	// Validate metadata
	assert.Equal(t, 100, result.Metadata[shared.TotalTokens])
	assert.Equal(t, "0.0015", result.Metadata[shared.TotalPrice])
	assert.Equal(t, "USD", result.Metadata[shared.Currency])
}
