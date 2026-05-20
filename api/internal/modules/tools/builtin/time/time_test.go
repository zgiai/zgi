package time_test

import (
	"context"
	"testing"
	stdtime "time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	timepkg "github.com/zgiai/zgi/api/internal/modules/tools/builtin/time"
)

func TestCurrentTimeTool_Invoke(t *testing.T) {
	// Create tool
	tool := timepkg.NewCurrentTimeTool("test-tenant")

	// Test default timezone (UTC)
	t.Run("default timezone", func(t *testing.T) {
		params := map[string]interface{}{}
		messages, err := tool.Invoke(context.Background(), "user1", params, nil, nil, nil)

		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.NotEmpty(t, messages[0].Text)

		// Parse the time to verify format
		_, err = stdtime.Parse("2006-01-02 15:04:05", messages[0].Text)
		assert.NoError(t, err)
	})

	// Test Asia/Shanghai timezone
	t.Run("Asia/Shanghai timezone", func(t *testing.T) {
		params := map[string]interface{}{
			"timezone": "Asia/Shanghai",
		}
		messages, err := tool.Invoke(context.Background(), "user1", params, nil, nil, nil)

		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.NotEmpty(t, messages[0].Text)
	})

	// Test custom format
	t.Run("custom format", func(t *testing.T) {
		params := map[string]interface{}{
			"timezone": "UTC",
			"format":   "2006/01/02",
		}
		messages, err := tool.Invoke(context.Background(), "user1", params, nil, nil, nil)

		require.NoError(t, err)
		require.Len(t, messages, 1)

		// Verify format
		_, err = stdtime.Parse("2006/01/02", messages[0].Text)
		assert.NoError(t, err)
	})

	// Test invalid timezone
	t.Run("invalid timezone", func(t *testing.T) {
		params := map[string]interface{}{
			"timezone": "Invalid/Timezone",
		}
		messages, err := tool.Invoke(context.Background(), "user1", params, nil, nil, nil)

		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Contains(t, messages[0].Text, "Invalid timezone")
	})
}

func TestTimeProvider(t *testing.T) {
	provider := timepkg.NewTimeProvider()

	t.Run("provider entity", func(t *testing.T) {
		entity := provider.GetEntity()
		assert.Equal(t, "time", entity.Identity.Name)
		assert.Equal(t, "Time Tools", entity.Identity.Label.Get("en_US"))
		assert.Len(t, entity.Tools, 2)
	})

	t.Run("get tool", func(t *testing.T) {
		tool, err := provider.GetTool("current_time")
		require.NoError(t, err)
		assert.NotNil(t, tool)
		assert.Equal(t, "current_time", tool.GetEntity().Identity.Name)
	})

	t.Run("get non-existent tool", func(t *testing.T) {
		tool, err := provider.GetTool("non_existent")
		assert.Error(t, err)
		assert.Nil(t, tool)
	})
}
