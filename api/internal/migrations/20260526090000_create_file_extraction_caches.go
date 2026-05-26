package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260526090000FileExtractionCachesID = "20260526090000_create_file_extraction_caches"

func init() {
	registerSchemaMigration(
		migration20260526090000FileExtractionCachesID,
		upCreateFileExtractionCaches,
		downCreateFileExtractionCaches,
	)
}

func upCreateFileExtractionCaches(schema *mschema.Builder) error {
	return schema.Create("file_extraction_caches", func(table *mschema.Blueprint) {
		table.ID()
		table.String("file_id").NotNull()
		table.String("cache_key").NotNull()
		table.Text("content").NotNull()
		table.String("source").NotNull()
		table.TimestampsTz()
		table.Index("idx_file_extraction_caches_file", "file_id")
		table.Unique("idx_file_extraction_caches_file_key", "file_id", "cache_key")
		table.Foreign("fk_file_extraction_caches_file", []string{"file_id"}, "upload_files", []string{"id"}).CascadeOnDelete()
	})
}

func downCreateFileExtractionCaches(schema *mschema.Builder) error {
	return schema.DropIfExists("file_extraction_caches")
}
