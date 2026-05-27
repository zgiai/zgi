package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260526090001ID = "20260526090001_create_file_extraction_caches"

func init() {
	registerSchemaMigration(
		migration20260526090001ID,
		upCreateFileExtractionCaches,
		downCreateFileExtractionCaches,
	)
}

func upCreateFileExtractionCaches(schema *mschema.Builder) error {
	statements := []string{
		`
		CREATE TABLE IF NOT EXISTS public.file_extraction_caches (
			id uuid NOT NULL PRIMARY KEY,
			file_id uuid NOT NULL,
			cache_key character varying(255) NOT NULL,
			content text NOT NULL,
			source character varying(255) NOT NULL,
			created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
			updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
		)
		`,
		`ALTER TABLE public.file_extraction_caches ALTER COLUMN id DROP DEFAULT`,
		`
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM information_schema.columns
				WHERE table_schema = 'public'
				  AND table_name = 'file_extraction_caches'
				  AND column_name = 'file_id'
				  AND udt_name <> 'uuid'
			) THEN
				ALTER TABLE public.file_extraction_caches
					ALTER COLUMN file_id TYPE uuid USING file_id::uuid;
			END IF;
		END $$;
		`,
		`CREATE INDEX IF NOT EXISTS idx_file_extraction_caches_file ON public.file_extraction_caches (file_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_file_extraction_caches_file_key ON public.file_extraction_caches (file_id, cache_key)`,
		`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'fk_file_extraction_caches_file'
				  AND conrelid = 'public.file_extraction_caches'::regclass
			) THEN
				ALTER TABLE public.file_extraction_caches
					ADD CONSTRAINT fk_file_extraction_caches_file
					FOREIGN KEY (file_id)
					REFERENCES public.upload_files (id)
					ON DELETE CASCADE;
			END IF;
		END $$;
		`,
	}
	for _, statement := range statements {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}
	return nil
}

func downCreateFileExtractionCaches(schema *mschema.Builder) error {
	return schema.DropIfExists("file_extraction_caches")
}
