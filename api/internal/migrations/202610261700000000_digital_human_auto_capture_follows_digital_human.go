package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// A person's automatic memory with a DigitalHuman now follows the
// DigitalHuman, as it already does with an Agent (ADR 0019): it is on unless
// the person turns it off. This turns it on once for DigitalHuman
// relationships whose settings the person never changed (still at their
// first revision); a choice someone made stays as it is.
func init() {
	registerSchemaMigration("202610261700000000_digital_human_auto_capture_follows_digital_human", upDigitalHumanAutoCaptureFollowsDigitalHuman, downDigitalHumanAutoCaptureFollowsDigitalHuman)
}

func upDigitalHumanAutoCaptureFollowsDigitalHuman(schema *mschema.Builder) error {
	return schema.DataFix("turn on automatic memory for untouched DigitalHuman relationships", func(db *gorm.DB) error {
		return db.Exec(`
			UPDATE public.user_memory_relationships
			SET auto_capture_enabled = TRUE, settings_revision = settings_revision + 1, updated_at = NOW()
			WHERE consumer_type = 'digital_human' AND auto_capture_enabled = FALSE AND memory_enabled = TRUE
				AND settings_revision = 1`).Error
	})
}

// The relationships this turned on cannot be told apart from ones people
// turned on themselves afterwards.
func downDigitalHumanAutoCaptureFollowsDigitalHuman(*mschema.Builder) error {
	return nil
}
