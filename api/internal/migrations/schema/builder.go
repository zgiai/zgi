package schema

import (
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

var identPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Builder is a small PostgreSQL schema builder for ZGI migrations.
// It keeps migrations readable while still emitting explicit SQL.
type Builder struct {
	db                 *gorm.DB
	allowDestructive   bool
	executedStatements []string
}

func New(db *gorm.DB) *Builder {
	return &Builder{db: db}
}

func (b *Builder) AllowDestructive() *Builder {
	b.allowDestructive = true
	return b
}

func (b *Builder) Create(table string, define func(*Blueprint)) error {
	statements, err := CompileCreate(table, define)
	if err != nil {
		return err
	}
	return b.exec(statements)
}

func (b *Builder) Table(table string, define func(*Blueprint)) error {
	statements, err := CompileTable(table, define)
	if err != nil {
		return err
	}
	return b.exec(statements)
}

func (b *Builder) DropIfExists(table string) error {
	if !b.allowDestructive {
		return fmt.Errorf("drop table %s requires AllowDestructive", table)
	}
	sql, err := compileDropIfExists(table)
	if err != nil {
		return err
	}
	return b.exec([]string{sql})
}

func (b *Builder) Rename(from, to string) error {
	sql, err := compileRename(from, to)
	if err != nil {
		return err
	}
	return b.exec([]string{sql})
}

func (b *Builder) Raw(statement string) error {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return nil
	}
	return b.exec([]string{statement})
}

func (b *Builder) UpdateRowsWhereEqual(table, setColumn string, setValue any, whereColumn string, whereValue any) error {
	for _, name := range []string{table, setColumn, whereColumn} {
		if err := validateIdent(name); err != nil {
			return err
		}
	}

	statement := fmt.Sprintf(
		"UPDATE %s SET %s = ? WHERE %s = ?",
		quoteTable(table),
		quoteIdent(setColumn),
		quoteIdent(whereColumn),
	)
	b.executedStatements = append(b.executedStatements, statement)
	if err := b.db.Exec(statement, setValue, whereValue).Error; err != nil {
		return fmt.Errorf("execute data fix statement %s: %w", preview(statement), err)
	}
	return nil
}

func (b *Builder) DataFix(description string, fn func(*gorm.DB) error) error {
	description = strings.TrimSpace(description)
	if description == "" {
		return fmt.Errorf("data fix description is required")
	}
	if fn == nil {
		return fmt.Errorf("data fix %s has nil function", description)
	}

	b.executedStatements = append(b.executedStatements, "DATA FIX: "+description)
	if err := fn(b.db); err != nil {
		return fmt.Errorf("execute data fix %s: %w", description, err)
	}
	return nil
}

func (b *Builder) UpdateRowsWhereNotEqual(table, setColumn string, setValue any, whereColumn string, whereValue any) error {
	for _, name := range []string{table, setColumn, whereColumn} {
		if err := validateIdent(name); err != nil {
			return err
		}
	}

	statement := fmt.Sprintf(
		"UPDATE %s SET %s = ? WHERE %s <> ?",
		quoteTable(table),
		quoteIdent(setColumn),
		quoteIdent(whereColumn),
	)
	b.executedStatements = append(b.executedStatements, statement)
	if err := b.db.Exec(statement, setValue, whereValue).Error; err != nil {
		return fmt.Errorf("execute data fix statement %s: %w", preview(statement), err)
	}
	return nil
}

func (b *Builder) HasTable(table string) (bool, error) {
	if err := validateIdent(table); err != nil {
		return false, err
	}
	var exists bool
	err := b.db.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_name = ?
		)
	`, table).Scan(&exists).Error
	return exists, err
}

func (b *Builder) HasColumn(table, column string) (bool, error) {
	if err := validateIdent(table); err != nil {
		return false, err
	}
	if err := validateIdent(column); err != nil {
		return false, err
	}
	var exists bool
	err := b.db.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = ?
			  AND column_name = ?
		)
	`, table, column).Scan(&exists).Error
	return exists, err
}

func (b *Builder) HasIndex(table, index string) (bool, error) {
	if err := validateIdent(table); err != nil {
		return false, err
	}
	if err := validateIdent(index); err != nil {
		return false, err
	}
	var exists bool
	err := b.db.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = 'public'
			  AND tablename = ?
			  AND indexname = ?
		)
	`, table, index).Scan(&exists).Error
	return exists, err
}

func (b *Builder) WhenTableHasColumn(table, column string, fn func() error) error {
	exists, err := b.HasColumn(table, column)
	if err != nil || !exists {
		return err
	}
	return fn()
}

func (b *Builder) WhenTableDoesntHaveColumn(table, column string, fn func() error) error {
	exists, err := b.HasColumn(table, column)
	if err != nil || exists {
		return err
	}
	return fn()
}

func (b *Builder) exec(statements []string) error {
	for _, statement := range statements {
		if isDestructive(statement) && !b.allowDestructive {
			return fmt.Errorf("destructive schema statement requires AllowDestructive: %s", preview(statement))
		}
		b.executedStatements = append(b.executedStatements, statement)
		if err := b.db.Exec(statement).Error; err != nil {
			return fmt.Errorf("execute schema statement %s: %w", preview(statement), err)
		}
	}
	return nil
}

func (b *Builder) Statements() []string {
	return append([]string(nil), b.executedStatements...)
}

func CompileCreate(table string, define func(*Blueprint)) ([]string, error) {
	if err := validateIdent(table); err != nil {
		return nil, err
	}
	blueprint := newBlueprint(table, modeCreate)
	define(blueprint)
	return blueprint.compile()
}

func CompileTable(table string, define func(*Blueprint)) ([]string, error) {
	if err := validateIdent(table); err != nil {
		return nil, err
	}
	blueprint := newBlueprint(table, modeAlter)
	define(blueprint)
	return blueprint.compile()
}

func compileDropIfExists(table string) (string, error) {
	if err := validateIdent(table); err != nil {
		return "", err
	}
	return fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteTable(table)), nil
}

func compileRename(from, to string) (string, error) {
	if err := validateIdent(from); err != nil {
		return "", err
	}
	if err := validateIdent(to); err != nil {
		return "", err
	}
	return fmt.Sprintf("ALTER TABLE %s RENAME TO %s", quoteTable(from), quoteIdent(to)), nil
}

func validateIdent(name string) error {
	if !identPattern.MatchString(name) {
		return fmt.Errorf("invalid SQL identifier %q; use lower_snake_case", name)
	}
	return nil
}

func validateIdents(names []string) error {
	for _, name := range names {
		if err := validateIdent(name); err != nil {
			return err
		}
	}
	return nil
}

func quoteIdent(name string) string {
	return `"` + name + `"`
}

func quoteTable(name string) string {
	return `"public".` + quoteIdent(name)
}

func quoteList(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, quoteIdent(name))
	}
	return strings.Join(quoted, ", ")
}

func preview(statement string) string {
	preview := strings.Join(strings.Fields(statement), " ")
	if len(preview) <= 120 {
		return preview
	}
	return preview[:120] + "..."
}

func isDestructive(statement string) bool {
	normalized := strings.ToUpper(strings.Join(strings.Fields(statement), " "))
	return strings.HasPrefix(normalized, "DROP TABLE ") ||
		strings.HasPrefix(normalized, "DROP SCHEMA ") ||
		strings.HasPrefix(normalized, "TRUNCATE ") ||
		strings.HasPrefix(normalized, "DELETE FROM ") ||
		strings.HasPrefix(normalized, "UPDATE ") ||
		strings.Contains(normalized, " DROP COLUMN ")
}
