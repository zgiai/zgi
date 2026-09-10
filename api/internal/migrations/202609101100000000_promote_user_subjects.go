package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationPromoteUserSubjectsID = "202609101100000000_promote_user_subjects"

func init() {
	registerSchemaMigration(migrationPromoteUserSubjectsID, upPromoteUserSubjects, downPromoteUserSubjects)
}

func upPromoteUserSubjects(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.user_memory_subjects RENAME TO user_subjects;
		ALTER INDEX IF EXISTS public.idx_user_memory_subject_account RENAME TO idx_user_subject_account;
		ALTER INDEX IF EXISTS public.idx_user_memory_subject_issuer RENAME TO idx_user_subject_issuer;
	`)
}

func downPromoteUserSubjects(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER INDEX IF EXISTS public.idx_user_subject_issuer RENAME TO idx_user_memory_subject_issuer;
		ALTER INDEX IF EXISTS public.idx_user_subject_account RENAME TO idx_user_memory_subject_account;
		ALTER TABLE public.user_subjects RENAME TO user_memory_subjects;
	`)
}
