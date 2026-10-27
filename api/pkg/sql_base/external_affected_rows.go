package sql_base

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/auxten/postgresql-parser/pkg/sql/sem/tree"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlanalysis"
)

// externalAffectedRowsColumn is the column the counting wrapper returns.
const externalAffectedRowsColumn = "zgi_affected_rows"

// wrapMutationForAffectedRows rewrites a single INSERT/UPDATE/DELETE without a
// RETURNING clause into a data-modifying CTE that returns the number of
// written rows. postgres-meta only returns result rows, never the command
// tag, so this is the only way to learn how many rows a plain mutation wrote.
//
// The rewrite is applied only when it is provably safe; otherwise ok is false
// and the statement runs unchanged:
//   - exactly one statement that parses as INSERT, UPDATE or DELETE;
//   - no WITH clause (a data-modifying WITH cannot be nested);
//   - no RETURNING clause (the returned rows already give the count);
//   - no comments or inner semicolons, so appending RETURNING to the text
//     cannot land inside a comment or after a statement terminator.
func wrapMutationForAffectedRows(query string) (string, bool) {
	text := strings.TrimRight(strings.TrimSpace(query), "; \t\r\n")
	if text == "" || strings.Contains(text, ";") || strings.Contains(text, "--") || strings.Contains(text, "/*") {
		return query, false
	}
	analysis := sqlanalysis.Analyze(text)
	if !analysis.Parsed() || analysis.StatementCount != 1 || len(analysis.Statements) == 0 || analysis.HasCTE {
		return query, false
	}
	var returning tree.ReturningClause
	switch stmt := analysis.Statements[0].(type) {
	case *tree.Insert:
		returning = stmt.Returning
	case *tree.Update:
		returning = stmt.Returning
	case *tree.Delete:
		returning = stmt.Returning
	default:
		return query, false
	}
	if returning == nil || tree.HasReturningClause(returning) {
		return query, false
	}
	return "WITH zgi_affected AS (\n" + text + "\nRETURNING 1\n) SELECT count(*)::bigint AS " +
		externalAffectedRowsColumn + " FROM zgi_affected", true
}

// parseWrappedAffectedRows reads the count produced by the counting wrapper.
func parseWrappedAffectedRows(rows []map[string]interface{}) (int64, bool) {
	if len(rows) != 1 {
		return 0, false
	}
	switch v := rows[0][externalAffectedRowsColumn].(type) {
	case float64:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	case string:
		// node-postgres serializes bigint as a string.
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
