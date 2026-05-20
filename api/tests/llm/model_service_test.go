package llm_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

// TestModelViewHasIsAvailableField tests that ModelView includes is_available field
func TestModelViewHasIsAvailableField(t *testing.T) {
	view := model.ModelView{
		ID:          uuid.New(),
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		ModelName:   "GPT-4o Mini",
		IsEnabled:   true,
		IsAvailable: true,
	}

	// Verify IsAvailable field exists and is set correctly
	assert.True(t, view.IsAvailable)
	assert.True(t, view.IsEnabled)
}

// TestModelViewIsAvailableFalseByDefault tests that is_available defaults to false
func TestModelViewIsAvailableFalseByDefault(t *testing.T) {
	view := model.ModelView{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "gpt-4o",
		IsEnabled: true,
		// IsAvailable not set - should be false
	}

	assert.False(t, view.IsAvailable)
}

// TestModelViewIsAvailableIndependentOfIsEnabled tests that is_available is independent of is_enabled
func TestModelViewIsAvailableIndependentOfIsEnabled(t *testing.T) {
	// Model enabled but no official channel available
	view1 := model.ModelView{
		IsEnabled:   true,
		IsAvailable: false,
	}
	assert.True(t, view1.IsEnabled)
	assert.False(t, view1.IsAvailable)

	// Model disabled but has official channel available
	view2 := model.ModelView{
		IsEnabled:   false,
		IsAvailable: true,
	}
	assert.False(t, view2.IsEnabled)
	assert.True(t, view2.IsAvailable)
}
