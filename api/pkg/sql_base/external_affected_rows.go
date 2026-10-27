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
//   - no comments or semicolons outside quoted text, so appending RETURNING
//     cannot land inside a comment or after a statement terminator. Quoted
//     literals and identifiers may contain these characters.
func wrapMutationForAffectedRows(query string) (string, bool) {
	text := strings.TrimRight(strings.TrimSpace(query), "; \t\r\n")
	if text == "" || hasUnquotedDelimiter(text) {
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

// hasUnquotedDelimiter reports whether query has a statement terminator or a
// comment outside quoted text. It skips single-quoted strings (including E”
// escape strings), double-quoted identifiers and dollar-quoted strings. An
// unterminated quote counts as a delimiter so the rewrite is skipped.
func hasUnquotedDelimiter(query string) bool {
	for i := 0; i < len(query); i++ {
		switch c := query[i]; {
		case c == ';':
			return true
		case c == '-' && i+1 < len(query) && query[i+1] == '-':
			return true
		case c == '/' && i+1 < len(query) && query[i+1] == '*':
			return true
		case c == '\'':
			escapes := i > 0 && (query[i-1] == 'E' || query[i-1] == 'e') && (i < 2 || !isSQLIdentByte(query[i-2]))
			end, ok := skipQuoted(query, i, '\'', escapes)
			if !ok {
				return true
			}
			i = end
		case c == '"':
			end, ok := skipQuoted(query, i, '"', false)
			if !ok {
				return true
			}
			i = end
		case c == '$' && (i == 0 || !isSQLIdentByte(query[i-1])):
			tagEnd := i + 1
			for tagEnd < len(query) && isSQLIdentByte(query[tagEnd]) && !(tagEnd == i+1 && query[tagEnd] >= '0' && query[tagEnd] <= '9') {
				tagEnd++
			}
			if tagEnd >= len(query) || query[tagEnd] != '$' {
				// A positional parameter such as $1, not a dollar quote.
				continue
			}
			tag := query[i : tagEnd+1]
			closing := strings.Index(query[tagEnd+1:], tag)
			if closing < 0 {
				return true
			}
			i = tagEnd + closing + len(tag)
		}
	}
	return false
}

// skipQuoted returns the index of the closing quote for the quoted text that
// starts at start. A doubled quote is part of the text; with escapes, so is a
// backslash-escaped character.
func skipQuoted(query string, start int, quote byte, escapes bool) (int, bool) {
	for i := start + 1; i < len(query); i++ {
		switch query[i] {
		case '\\':
			if escapes {
				i++
			}
		case quote:
			if i+1 < len(query) && query[i+1] == quote {
				i++
				continue
			}
			return i, true
		}
	}
	return 0, false
}

func isSQLIdentByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c >= 0x80
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
