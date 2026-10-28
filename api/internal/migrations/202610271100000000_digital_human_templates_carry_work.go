package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// A saved DigitalHuman template carries what a DigitalHuman needs to work
// (ADR 0022): its own title beside the name in conversation, a description,
// the memory instructions, and the skills it binds. Existing templates keep
// applying their name, now as the name in conversation.
func init() {
	registerSchemaMigration("202610271100000000_digital_human_templates_carry_work", upDigitalHumanTemplatesCarryWork, downDigitalHumanTemplatesCarryWork)
}

func upDigitalHumanTemplatesCarryWork(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.digital_human_persona_templates ADD COLUMN IF NOT EXISTS identity_name varchar(80) NOT NULL DEFAULT '';
		ALTER TABLE public.digital_human_persona_templates ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '';
		ALTER TABLE public.digital_human_persona_templates ADD COLUMN IF NOT EXISTS memory_instructions text NOT NULL DEFAULT '';
		ALTER TABLE public.digital_human_persona_templates ADD COLUMN IF NOT EXISTS skill_ids jsonb NOT NULL DEFAULT '[]'::jsonb;
	`); err != nil {
		return err
	}
	return schema.DataFix("keep applying each saved template's name as the name in conversation", func(db *gorm.DB) error {
		return db.Exec(`UPDATE public.digital_human_persona_templates SET identity_name = name WHERE identity_name = ''`).Error
	})
}

// The columns stay: up migrations add them, and dropping them would lose what
// people saved.
func downDigitalHumanTemplatesCarryWork(*mschema.Builder) error {
	return nil
}
