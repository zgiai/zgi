package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// TTS migrations for TTS module
var TTSMigrations = []*gormigrate.Migration{
	{
		ID: "202504200001_add_voice_to_tts_history",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE tts_audio_history ADD COLUMN voice VARCHAR(50) DEFAULT 'alloy'").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE tts_audio_history DROP COLUMN voice").Error
		},
	},
}
