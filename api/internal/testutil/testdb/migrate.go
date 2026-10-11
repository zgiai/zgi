// Package testdb contains database fixtures for tests, not runtime migrations.
package testdb

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// AutoMigrate retains the model's columns and constraints while translating
// PostgreSQL-only timestamp and UUID defaults for an isolated SQLite fixture.
// Schema metadata belongs to this database's cache; production models and other
// database connections are not modified. PostgreSQL uses the original schema.
func AutoMigrate(db *gorm.DB, models ...any) error {
	if db.Dialector.Name() == "sqlite" {
		for _, model := range models {
			statement := &gorm.Statement{DB: db}
			if err := statement.Parse(model); err != nil {
				return err
			}
			for _, field := range statement.Schema.Fields {
				if field.GORMDataType == schema.Time {
					field.DataType = schema.Time
					field.TagSettings["TYPE"] = "datetime"
				}
				switch strings.ToLower(field.DefaultValue) {
				case "uuid_generate_v4()", "gen_random_uuid()":
					field.DefaultValue = "(lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-a' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))))"
				case "current_timestamp(0)":
					field.DefaultValue = "CURRENT_TIMESTAMP"
				}
			}
		}
	}
	return db.AutoMigrate(models...)
}
