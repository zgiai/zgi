package baseline

import (
	"bufio"
	"embed"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

//go:embed *.sql
var baselineFS embed.FS

func ApplySnapshot(tx *gorm.DB) error {
	for _, chunk := range Chunks() {
		if err := applyChunk(tx, chunk); err != nil {
			return err
		}
	}
	return nil
}

func applyChunk(tx *gorm.DB, chunk Chunk) error {
	content, err := baselineFS.ReadFile(chunk.File)
	if err != nil {
		return fmt.Errorf("read baseline chunk %s: %w", chunk.File, err)
	}

	statements, err := parseSQLDumpStatements(string(content))
	if err != nil {
		return fmt.Errorf("parse baseline chunk %s: %w", chunk.File, err)
	}

	for i, statement := range statements {
		if err := tx.Exec(statement).Error; err != nil {
			return fmt.Errorf(
				"execute baseline chunk %s statement %d (%s): %w",
				chunk.Name,
				i+1,
				statementPreview(statement),
				err,
			)
		}
	}

	return nil
}

func parseSQLDumpStatements(content string) ([]string, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var (
		builder    strings.Builder
		statements []string
	)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if shouldSkipDumpLine(trimmed) {
			continue
		}

		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(line)

		if strings.HasSuffix(trimmed, ";") {
			statements = append(statements, builder.String())
			builder.Reset()
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if trailing := strings.TrimSpace(builder.String()); trailing != "" {
		statements = append(statements, trailing)
	}

	return statements, nil
}

func shouldSkipDumpLine(trimmed string) bool {
	if trimmed == "" {
		return true
	}
	if strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "\\") {
		return true
	}
	if strings.HasPrefix(trimmed, "SET ") || strings.HasPrefix(trimmed, "SELECT pg_catalog.set_config") {
		return true
	}
	if strings.HasPrefix(trimmed, "CREATE EXTENSION ") || strings.HasPrefix(trimmed, "COMMENT ON EXTENSION ") {
		return true
	}
	return false
}
