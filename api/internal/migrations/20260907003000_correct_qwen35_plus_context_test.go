package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCorrectQwen35PlusContext(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE llm_models (
		name TEXT PRIMARY KEY,
		context_window INTEGER,
		max_output_tokens INTEGER,
		updated_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO llm_models (name, context_window, max_output_tokens) VALUES
		('qwen3.5-plus', 100000, 4096),
		('qwen3.5-plus-2026-02-15', 100000, 8192),
		('qwen3.5-397b-a17b', 32000, 8192)`).Error)

	require.NoError(t, upCorrectQwen35PlusContext(db))

	type limits struct {
		Name            string
		ContextWindow   int
		MaxOutputTokens int
	}
	var rows []limits
	require.NoError(t, db.Table("llm_models").Order("name").Find(&rows).Error)
	byModel := make(map[string]limits, len(rows))
	for _, row := range rows {
		byModel[row.Name] = row
	}
	require.Equal(t, 1000000, byModel["qwen3.5-plus"].ContextWindow)
	require.Equal(t, 65536, byModel["qwen3.5-plus"].MaxOutputTokens)
	require.Equal(t, 1000000, byModel["qwen3.5-plus-2026-02-15"].ContextWindow)
	require.Equal(t, 8192, byModel["qwen3.5-plus-2026-02-15"].MaxOutputTokens)
	require.Equal(t, 32000, byModel["qwen3.5-397b-a17b"].ContextWindow)
}
