package migrations

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/migrations/baseline"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type CheckOptions struct {
	PostgresDSN string
}

type CheckResult struct {
	MigrationCount       int
	CheckedFiles         []string
	PostgresCheckSkipped bool
	PostgresCheckRan     bool
}

func Check(options CheckOptions) (CheckResult, error) {
	result := CheckResult{}

	if err := checkRegisteredMigrations(); err != nil {
		return result, err
	}

	files, err := checkMigrationFilenames()
	if err != nil {
		return result, err
	}
	result.CheckedFiles = files
	result.MigrationCount = len(allMigrations())

	if err := checkMigrationSourceSafety(files); err != nil {
		return result, err
	}

	if err := checkBaselineStatementSafety(); err != nil {
		return result, err
	}

	if strings.TrimSpace(options.PostgresDSN) == "" {
		result.PostgresCheckSkipped = true
		return result, nil
	}

	if err := checkFreshPostgres(options.PostgresDSN); err != nil {
		return result, err
	}
	result.PostgresCheckRan = true
	return result, nil
}

func checkRegisteredMigrations() error {
	migrations := allMigrations()
	seen := make(map[string]struct{}, len(migrations))
	for i, migration := range migrations {
		if migration == nil {
			return fmt.Errorf("migration at index %d is nil", i)
		}
		if !migrationIDPattern.MatchString(migration.ID) {
			return fmt.Errorf("migration ID %q must match public migration ID format", migration.ID)
		}
		if migration.Migrate == nil {
			return fmt.Errorf("migration %s has nil Migrate function", migration.ID)
		}
		if _, exists := seen[migration.ID]; exists {
			return fmt.Errorf("duplicate migration ID %s", migration.ID)
		}
		seen[migration.ID] = struct{}{}
		if i > 0 && migrations[i-1].ID > migration.ID {
			return fmt.Errorf("migrations must be sorted by ID: %s before %s", migrations[i-1].ID, migration.ID)
		}
	}
	return nil
}

func checkMigrationFilenames() ([]string, error) {
	root, err := migrationsDir()
	if err != nil {
		return nil, err
	}

	files, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		return nil, err
	}
	slices.Sort(files)

	allowed := map[string]struct{}{
		"check.go":               {},
		"legacy_bridge.go":       {},
		"postgres_smoke_test.go": {},
		"registry.go":            {},
		"runner.go":              {},
		"runner_test.go":         {},
		"schema_executor.go":     {},
	}
	migrationIDs := make(map[string]struct{}, len(allMigrations()))
	for _, migration := range allMigrations() {
		migrationIDs[migration.ID] = struct{}{}
		expected := filepath.Join(root, migration.ID+".go")
		if _, err := os.Stat(expected); err != nil {
			return nil, fmt.Errorf("migration %s must live in %s: %w", migration.ID, expected, err)
		}
	}

	var checked []string
	for _, file := range files {
		name := filepath.Base(file)
		if _, ok := allowed[name]; ok || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		id := strings.TrimSuffix(name, ".go")
		if _, ok := migrationIDs[id]; !ok {
			return nil, fmt.Errorf("migration file %s does not match a registered migration ID", name)
		}
		checked = append(checked, file)
	}

	return checked, nil
}

func checkMigrationSourceSafety(files []string) error {
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", file, err)
		}

		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file, data, 0)
		if err != nil {
			return fmt.Errorf("parse migration file %s: %w", file, err)
		}
		migrationID := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		inspector := newMigrationUpInspector(fset, parsed, migrationID >= migrationDataFixPolicyStartID)
		roots, err := migrationUpRoots(parsed)
		if err != nil {
			return fmt.Errorf("inspect migration file %s: %w", filepath.Base(file), err)
		}
		if len(roots) == 0 {
			return fmt.Errorf("migration file %s does not register an up migration", filepath.Base(file))
		}
		for _, root := range roots {
			if err := inspector.check(root); err != nil {
				return fmt.Errorf("migration file %s: %w", filepath.Base(file), err)
			}
		}
	}
	return nil
}

// migrationUpInspector follows only declarations reachable from the function
// registered as Migrate/up. Rollbacks are intentionally destructive and are
// protected separately by the exact-ID rollback confirmation and the schema
// builder's AllowDestructive gate.
type migrationUpInspector struct {
	fset                 *token.FileSet
	objects              map[*ast.Object]ast.Node
	visited              map[ast.Node]migrationInspectionMode
	enforceDataFixForDML bool
	inspectErr           error
}

// Historical public migrations through this repository snapshot predate the
// explicit DataFix gate and must remain stable once deployed. New migrations
// from this boundary onward must place UPDATE/DELETE work in DataFix.
const migrationDataFixPolicyStartID = "202608290000000000"

type migrationInspectionMode uint8

const (
	migrationInspectionDefault migrationInspectionMode = 1 << iota
	migrationInspectionDataFix
)

func newMigrationUpInspector(fset *token.FileSet, file *ast.File, enforceDataFixForDML bool) *migrationUpInspector {
	inspector := &migrationUpInspector{
		fset:                 fset,
		objects:              make(map[*ast.Object]ast.Node),
		visited:              make(map[ast.Node]migrationInspectionMode),
		enforceDataFixForDML: enforceDataFixForDML,
	}
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if declaration.Name.Obj != nil {
				inspector.objects[declaration.Name.Obj] = declaration
			}
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for index, name := range valueSpec.Names {
					if name.Obj == nil || len(valueSpec.Values) == 0 {
						continue
					}
					valueIndex := index
					if valueIndex >= len(valueSpec.Values) {
						valueIndex = len(valueSpec.Values) - 1
					}
					inspector.objects[name.Obj] = valueSpec.Values[valueIndex]
				}
			}
		}
	}
	return inspector
}

func migrationUpRoots(file *ast.File) ([]ast.Node, error) {
	var roots []ast.Node
	var rootErr error
	ast.Inspect(file, func(node ast.Node) bool {
		if rootErr != nil {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		switch name.Name {
		case "registerSchemaMigration":
			if len(call.Args) < 2 {
				rootErr = fmt.Errorf("registerSchemaMigration is missing its up function")
				return false
			}
			roots = append(roots, call.Args[1])
		case "registerMigration":
			if len(call.Args) != 1 {
				rootErr = fmt.Errorf("registerMigration must receive one migration")
				return false
			}
			migrate, err := registeredMigrateExpression(call.Args[0])
			if err != nil {
				rootErr = err
				return false
			}
			roots = append(roots, migrate)
		}
		return true
	})
	return roots, rootErr
}

func registeredMigrateExpression(expression ast.Expr) (ast.Expr, error) {
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		expression = unary.X
	}
	composite, ok := expression.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("registerMigration must use an inline migration literal")
	}
	for _, element := range composite.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := field.Key.(*ast.Ident)
		if ok && key.Name == "Migrate" {
			return field.Value, nil
		}
	}
	return nil, fmt.Errorf("registered migration literal has no Migrate function")
}

func (i *migrationUpInspector) check(root ast.Node) error {
	i.inspect(root, false)
	return i.inspectErr
}

func (i *migrationUpInspector) inspect(root ast.Node, dataFix bool) {
	if root == nil || i.inspectErr != nil {
		return
	}
	mode := migrationInspectionDefault
	if dataFix {
		mode = migrationInspectionDataFix
	}
	if i.visited[root]&mode != 0 {
		return
	}
	i.visited[root] |= mode

	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil || i.inspectErr != nil {
			return false
		}
		switch node := node.(type) {
		case *ast.Ident:
			if declaration, ok := i.objects[node.Obj]; ok {
				i.inspect(declaration, dataFix)
			}
		case *ast.CallExpr:
			name := methodName(node.Fun)
			if name == "AllowDestructive" {
				i.inspectErr = i.forbidden(node, "AllowDestructive")
				return false
			}
			if name == "DropIfExists" || name == "DropColumn" {
				i.inspectErr = i.forbidden(node, name)
				return false
			}
			if i.enforceDataFixForDML && !dataFix &&
				(name == "UpdateRowsWhereEqual" || name == "UpdateRowsWhereNotEqual") {
				i.inspectErr = i.forbidden(node, name)
				return false
			}
			if name == "DataFix" {
				for _, argument := range node.Args {
					i.inspect(argument, true)
				}
				return false
			}
		case *ast.BasicLit:
			if node.Kind != token.STRING {
				break
			}
			value, err := strconv.Unquote(node.Value)
			if err != nil {
				i.inspectErr = fmt.Errorf("decode string literal at %s: %w", i.fset.Position(node.Pos()), err)
				return false
			}
			normalized := strings.ToUpper(strings.Join(strings.Fields(value), " "))
			for _, forbidden := range []string{"DROP TABLE", "DROP COLUMN", "DROP SCHEMA", "TRUNCATE "} {
				if strings.Contains(normalized, forbidden) {
					i.inspectErr = i.forbidden(node, forbidden)
					return false
				}
			}
			if i.enforceDataFixForDML && !dataFix {
				for _, forbidden := range []string{"DELETE FROM ", "UPDATE "} {
					if strings.Contains(normalized, forbidden) {
						i.inspectErr = i.forbidden(node, forbidden)
						return false
					}
				}
			}
		}
		return true
	})
}

func (i *migrationUpInspector) forbidden(node ast.Node, operation string) error {
	return fmt.Errorf("up migration contains forbidden operation %q at %s", operation, i.fset.Position(node.Pos()))
}

func methodName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.SelectorExpr:
		return expression.Sel.Name
	default:
		return ""
	}
}

func checkBaselineStatementSafety() error {
	for _, file := range baseline.Files {
		for _, statement := range file.Statements {
			normalized := strings.ToUpper(strings.Join(strings.Fields(statement), " "))
			for _, forbidden := range []string{
				"DROP TABLE",
				"DROP SCHEMA",
				"TRUNCATE ",
				"DELETE FROM ",
				"UPDATE ",
				"ALTER TABLE ONLY PUBLIC.MIGRATIONS",
			} {
				if strings.HasPrefix(normalized, forbidden) {
					return fmt.Errorf("baseline file %s contains forbidden statement %q: %s", file.Name, forbidden, statementPreview(statement))
				}
			}
		}
	}
	return nil
}

func checkFreshPostgres(dsn string) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("connect PostgreSQL check database: %w", err)
	}
	if err := RunWithDB(db); err != nil {
		return fmt.Errorf("run migrations on fresh PostgreSQL check database: %w", err)
	}
	return nil
}

func migrationsDir() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve migrations directory")
	}
	return filepath.Dir(filename), nil
}
