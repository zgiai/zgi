package schema

import (
	"fmt"
	"strconv"
	"strings"
)

type blueprintMode int

const (
	modeCreate blueprintMode = iota
	modeAlter
)

type Blueprint struct {
	table    string
	mode     blueprintMode
	columns  []*Column
	commands []func() (string, error)
	comments []string
}

func newBlueprint(table string, mode blueprintMode) *Blueprint {
	return &Blueprint{table: table, mode: mode}
}

func (b *Blueprint) ID() *Column {
	return b.UUID("id").DefaultSQL("public.uuid_generate_v4()").NotNull().Primary()
}

func (b *Blueprint) UUID(name string) *Column {
	return b.column(name, "uuid")
}

func (b *Blueprint) String(name string, length ...int) *Column {
	size := 255
	if len(length) > 0 {
		size = length[0]
	}
	return b.column(name, fmt.Sprintf("varchar(%d)", size))
}

func (b *Blueprint) Text(name string) *Column {
	return b.column(name, "text")
}

func (b *Blueprint) Boolean(name string) *Column {
	return b.column(name, "boolean")
}

func (b *Blueprint) Integer(name string) *Column {
	return b.column(name, "integer")
}

func (b *Blueprint) BigInteger(name string) *Column {
	return b.column(name, "bigint")
}

func (b *Blueprint) Decimal(name string, precision, scale int) *Column {
	return b.column(name, fmt.Sprintf("numeric(%d,%d)", precision, scale))
}

func (b *Blueprint) JSONB(name string) *Column {
	return b.column(name, "jsonb")
}

func (b *Blueprint) TimestampTz(name string) *Column {
	return b.column(name, "timestamptz")
}

func (b *Blueprint) TimestampsTz() {
	b.TimestampTz("created_at").DefaultSQL("CURRENT_TIMESTAMP").NotNull()
	b.TimestampTz("updated_at").DefaultSQL("CURRENT_TIMESTAMP").NotNull()
}

func (b *Blueprint) SoftDeletes() {
	b.TimestampTz("deleted_at").Nullable()
}

func (b *Blueprint) RawColumn(name, definition string) *Column {
	return b.column(name, definition)
}

func (b *Blueprint) Primary(columns ...string) {
	b.constraint("", "PRIMARY KEY", columns)
}

func (b *Blueprint) Unique(name string, columns ...string) {
	b.index("UNIQUE INDEX", name, columns)
}

func (b *Blueprint) Index(name string, columns ...string) {
	b.index("INDEX", name, columns)
}

func (b *Blueprint) Foreign(name string, columns []string, refTable string, refColumns []string) *ForeignKey {
	fk := &ForeignKey{
		name:       name,
		table:      b.table,
		columns:    columns,
		refTable:   refTable,
		refColumns: refColumns,
	}
	b.commands = append(b.commands, fk.compile)
	return fk
}

func (b *Blueprint) DropColumn(name string) {
	b.commands = append(b.commands, func() (string, error) {
		if err := validateIdent(name); err != nil {
			return "", err
		}
		return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", quoteTable(b.table), quoteIdent(name)), nil
	})
}

func (b *Blueprint) RenameColumn(from, to string) {
	b.commands = append(b.commands, func() (string, error) {
		if err := validateIdent(from); err != nil {
			return "", err
		}
		if err := validateIdent(to); err != nil {
			return "", err
		}
		return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", quoteTable(b.table), quoteIdent(from), quoteIdent(to)), nil
	})
}

func (b *Blueprint) Comment(column, comment string) {
	b.commands = append(b.commands, func() (string, error) {
		if err := validateIdent(column); err != nil {
			return "", err
		}
		return fmt.Sprintf("COMMENT ON COLUMN %s.%s IS %s", quoteTable(b.table), quoteIdent(column), quoteLiteral(comment)), nil
	})
}

func (b *Blueprint) column(name, dataType string) *Column {
	column := &Column{name: name, dataType: dataType}
	b.columns = append(b.columns, column)
	return column
}

func (b *Blueprint) constraint(name, kind string, columns []string) {
	b.commands = append(b.commands, func() (string, error) {
		if err := validateIdents(columns); err != nil {
			return "", err
		}
		prefix := ""
		if name != "" {
			if err := validateIdent(name); err != nil {
				return "", err
			}
			prefix = "CONSTRAINT " + quoteIdent(name) + " "
		}
		return fmt.Sprintf("ALTER TABLE %s ADD %s%s (%s)", quoteTable(b.table), prefix, kind, quoteList(columns)), nil
	})
}

func (b *Blueprint) index(kind, name string, columns []string) {
	b.commands = append(b.commands, func() (string, error) {
		if err := validateIdent(name); err != nil {
			return "", err
		}
		if err := validateIdents(columns); err != nil {
			return "", err
		}
		return fmt.Sprintf("CREATE %s %s ON %s (%s)", kind, quoteIdent(name), quoteTable(b.table), quoteList(columns)), nil
	})
}

func (b *Blueprint) compile() ([]string, error) {
	statements := make([]string, 0, len(b.columns)+len(b.commands)+1)
	switch b.mode {
	case modeCreate:
		definitions := make([]string, 0, len(b.columns))
		for _, column := range b.columns {
			definition, err := column.compile()
			if err != nil {
				return nil, err
			}
			definitions = append(definitions, definition)
		}
		if len(definitions) == 0 {
			return nil, fmt.Errorf("create table %s has no columns", b.table)
		}
		statements = append(statements, fmt.Sprintf("CREATE TABLE %s (\n    %s\n)", quoteTable(b.table), strings.Join(definitions, ",\n    ")))
	case modeAlter:
		for _, column := range b.columns {
			definition, err := column.compile()
			if err != nil {
				return nil, err
			}
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", quoteTable(b.table), definition))
		}
	}

	for _, command := range b.commands {
		statement, err := command()
		if err != nil {
			return nil, err
		}
		statements = append(statements, statement)
	}
	return statements, nil
}

type Column struct {
	name       string
	dataType   string
	nullable   *bool
	defaultSQL string
	primary    bool
	unique     bool
}

func (c *Column) Nullable() *Column {
	value := true
	c.nullable = &value
	return c
}

func (c *Column) NotNull() *Column {
	value := false
	c.nullable = &value
	return c
}

func (c *Column) DefaultSQL(expression string) *Column {
	c.defaultSQL = expression
	return c
}

func (c *Column) Default(value any) *Column {
	switch typed := value.(type) {
	case string:
		c.defaultSQL = quoteLiteral(typed)
	case bool:
		c.defaultSQL = strconv.FormatBool(typed)
	case int:
		c.defaultSQL = strconv.Itoa(typed)
	case int64:
		c.defaultSQL = strconv.FormatInt(typed, 10)
	default:
		c.defaultSQL = fmt.Sprint(typed)
	}
	return c
}

func (c *Column) Primary() *Column {
	c.primary = true
	return c
}

func (c *Column) Unique() *Column {
	c.unique = true
	return c
}

func (c *Column) compile() (string, error) {
	if err := validateIdent(c.name); err != nil {
		return "", err
	}
	parts := []string{quoteIdent(c.name), c.dataType}
	if c.defaultSQL != "" {
		parts = append(parts, "DEFAULT "+c.defaultSQL)
	}
	if c.nullable != nil && !*c.nullable {
		parts = append(parts, "NOT NULL")
	}
	if c.primary {
		parts = append(parts, "PRIMARY KEY")
	}
	if c.unique {
		parts = append(parts, "UNIQUE")
	}
	return strings.Join(parts, " "), nil
}

type ForeignKey struct {
	name       string
	table      string
	columns    []string
	refTable   string
	refColumns []string
	onDelete   string
	onUpdate   string
}

func (f *ForeignKey) OnDelete(action string) *ForeignKey {
	f.onDelete = action
	return f
}

func (f *ForeignKey) OnUpdate(action string) *ForeignKey {
	f.onUpdate = action
	return f
}

func (f *ForeignKey) CascadeOnDelete() *ForeignKey {
	return f.OnDelete("CASCADE")
}

func (f *ForeignKey) NullOnDelete() *ForeignKey {
	return f.OnDelete("SET NULL")
}

func (f *ForeignKey) compile() (string, error) {
	if err := validateIdent(f.name); err != nil {
		return "", err
	}
	if err := validateIdents(f.columns); err != nil {
		return "", err
	}
	if err := validateIdent(f.refTable); err != nil {
		return "", err
	}
	if err := validateIdents(f.refColumns); err != nil {
		return "", err
	}

	parts := []string{
		fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			quoteTable(f.table),
			quoteIdent(f.name),
			quoteList(f.columns),
			quoteTable(f.refTable),
			quoteList(f.refColumns),
		),
	}
	if f.onUpdate != "" {
		parts = append(parts, "ON UPDATE "+f.onUpdate)
	}
	if f.onDelete != "" {
		parts = append(parts, "ON DELETE "+f.onDelete)
	}
	return strings.Join(parts, " "), nil
}

func quoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
