package migrations

import (
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// assertMilliPointSchema verifies every credit column really stores milli-points
// in an integer column, which is what the whole charging/reporting chain expects
// (millipoints.Value writes the raw int64).
//
// The fractional-points layout (numeric(20,3)) was abandoned: migration
// 202609171652571815_retire_internal_credit_units is retired and
// 202610011200000000_restore_milli_point_storage converts an already-converted
// database back. This assertion makes a database that is still fractional fail
// loudly at start-up instead of charging and reporting 1000x wrong amounts.
//
// It is a no-op on non-PostgreSQL dialectors (the information_schema probe is
// PostgreSQL-specific) and skips columns whose table or column does not exist,
// so it stays safe on a partially migrated database.
func assertMilliPointSchema(db *gorm.DB) error {
	if db == nil || db.Dialector == nil || db.Dialector.Name() != "postgres" {
		return nil
	}

	type columnRow struct {
		TableName  string `gorm:"column:table_name"`
		ColumnName string `gorm:"column:column_name"`
		DataType   string `gorm:"column:data_type"`
	}

	var rows []columnRow
	if err := db.Raw(`
		SELECT table_name, column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public'
	`).Scan(&rows).Error; err != nil {
		return fmt.Errorf("read information_schema.columns: %w", err)
	}

	present := make(map[string]string, len(rows))
	for _, row := range rows {
		present[row.TableName+"."+row.ColumnName] = strings.ToLower(strings.TrimSpace(row.DataType))
	}

	var offenders []string
	for _, target := range preRetiredCreditColumns() {
		for _, column := range target.columns {
			dataType, ok := present[target.table+"."+column]
			if !ok {
				continue
			}
			switch dataType {
			case "bigint", "integer", "smallint":
				// Integer storage is the milli-point layout.
			default:
				offenders = append(offenders, fmt.Sprintf("%s.%s is %s", target.table, column, dataType))
			}
		}
	}
	if len(offenders) == 0 {
		return nil
	}

	sort.Strings(offenders)
	return fmt.Errorf(
		"credit columns must store milli-points in BIGINT, but %d column(s) use a non-integer type: %s. "+
			"Every amount read or written through those columns would be off by 1000x. "+
			"Run the migration command to apply 202610011200000000_restore_milli_point_storage",
		len(offenders),
		strings.Join(offenders, "; "),
	)
}
