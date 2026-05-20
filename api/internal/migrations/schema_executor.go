package migrations

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/migrations/baseline"
	"gorm.io/gorm"
)

func applySchemaFiles(tx *gorm.DB, files []baseline.File) error {
	for _, file := range files {
		for i, statement := range file.Statements {
			if err := tx.Exec(statement).Error; err != nil {
				return fmt.Errorf("execute schema file %s statement %d (%s): %w", file.Name, i+1, statementPreview(statement), err)
			}
		}
	}
	return nil
}

func statementPreview(statement string) string {
	preview := strings.Join(strings.Fields(statement), " ")
	if len(preview) <= 120 {
		return preview
	}
	return preview[:120] + "..."
}
