package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// A person's automatic memory with an Agent now follows the Agent: it is on
// unless the person turns it off. Relationships created before this change
// started off, so automatic memory never ran even where the Agent and the
// deployment allowed it. This turns it on once for relationships whose
// settings the person never changed (still at their first revision); a
// choice someone made stays as it is.
func init() {
	registerSchemaMigration("202610261600000000_agent_auto_capture_follows_agent", upAgentAutoCaptureFollowsAgent, downAgentAutoCaptureFollowsAgent)
}

func upAgentAutoCaptureFollowsAgent(schema *mschema.Builder) error {
	return schema.DataFix("turn on automatic memory for untouched Agent relationships", func(db *gorm.DB) error {
		return db.Exec(`
			UPDATE public.user_memory_relationships
			SET auto_capture_enabled = TRUE, settings_revision = settings_revision + 1, updated_at = NOW()
			WHERE consumer_type = 'agent' AND auto_capture_enabled = FALSE AND memory_enabled = TRUE
				AND settings_revision = 1`).Error
	})
}

// The relationships this turned on cannot be told apart from ones people
// turned on themselves afterwards.
func downAgentAutoCaptureFollowsAgent(*mschema.Builder) error {
	return nil
}
