package guard

import (
	"strings"

	"github.com/auxten/postgresql-parser/pkg/sql/parser"
	"github.com/auxten/postgresql-parser/pkg/sql/sem/tree"
	"github.com/auxten/postgresql-parser/pkg/walk"
)

type analysis struct {
	statements             []tree.Statement
	topLevelStatementCount int
	functions              []string
	maxJoins               int
	parseErr               error
}

func parseSQL(sql string) analysis {
	stmts, err := parser.Parse(sql)
	a := analysis{parseErr: err}
	if err != nil {
		return a
	}

	a.statements = make([]tree.Statement, 0, len(stmts))
	for _, stmt := range stmts {
		if stmt.AST != nil {
			a.topLevelStatementCount++
			a.statements = appendAnalyzedStatements(a.statements, stmt.AST)
		}
	}

	w := &walk.AstWalker{
		Fn: func(ctx interface{}, node interface{}) bool {
			out := ctx.(*analysis)
			switch n := node.(type) {
			case *tree.FuncExpr:
				out.functions = append(out.functions, strings.ToLower(n.Func.String()))
			case *tree.SelectClause:
				out.maxJoins = max(out.maxJoins, countJoins(n.From.Tables))
			case *tree.Update:
				out.maxJoins = max(out.maxJoins, countJoins(n.From))
			}
			return false
		},
	}
	_, _ = w.Walk(stmts, &a)
	return a
}

func appendAnalyzedStatements(out []tree.Statement, stmt tree.Statement) []tree.Statement {
	if stmt == nil {
		return out
	}
	out = append(out, stmt)
	switch s := stmt.(type) {
	case *tree.Explain:
		out = appendAnalyzedStatements(out, s.Statement)
	case *tree.Select:
		out = appendWithStatements(out, s.With)
	case *tree.Update:
		out = appendWithStatements(out, s.With)
	case *tree.Delete:
		out = appendWithStatements(out, s.With)
	case *tree.Insert:
		out = appendWithStatements(out, s.With)
	}
	return out
}

func appendWithStatements(out []tree.Statement, with *tree.With) []tree.Statement {
	if with == nil {
		return out
	}
	for _, cte := range with.CTEList {
		if cte != nil {
			out = appendAnalyzedStatements(out, cte.Stmt)
		}
	}
	return out
}

func countJoins(exprs tree.TableExprs) int {
	total := 0
	for _, expr := range exprs {
		total += countJoinExpr(expr)
	}
	return total
}

func countJoinExpr(expr tree.TableExpr) int {
	switch n := expr.(type) {
	case *tree.AliasedTableExpr:
		if nested, ok := n.Expr.(tree.TableExpr); ok {
			return countJoinExpr(nested)
		}
	case *tree.JoinTableExpr:
		return 1 + countJoinExpr(n.Left) + countJoinExpr(n.Right)
	case *tree.ParenTableExpr:
		return countJoinExpr(n.Expr)
	}
	return 0
}

func hasNonTrailingSemicolon(sql string) bool {
	inSingle := false
	inDouble := false
	inLineComment := false
	inBlockComment := false
	dollarQuote := ""
	escaped := false

	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		next := byte(0)
		if i+1 < len(sql) {
			next = sql[i+1]
		}

		if inLineComment {
			if ch == '\n' || ch == '\r' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if dollarQuote != "" {
			if strings.HasPrefix(sql[i:], dollarQuote) {
				i += len(dollarQuote) - 1
				dollarQuote = ""
			}
			continue
		}
		if inSingle {
			if ch == '\'' && next == '\'' {
				i++
				continue
			}
			if ch == '\'' && !escaped {
				inSingle = false
			}
			escaped = ch == '\\' && !escaped
			continue
		}
		if inDouble {
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		if ch == '-' && next == '-' {
			inLineComment = true
			i++
			continue
		}
		if ch == '/' && next == '*' {
			inBlockComment = true
			i++
			continue
		}
		if ch == '$' {
			if tag := readDollarQuoteTag(sql[i:]); tag != "" {
				dollarQuote = tag
				i += len(tag) - 1
				continue
			}
		}
		switch ch {
		case '\'':
			inSingle = true
			escaped = false
		case '"':
			inDouble = true
		case ';':
			return hasExecutableTail(sql[i+1:])
		}
	}
	return false
}

func hasExecutableTail(sql string) bool {
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		next := byte(0)
		if i+1 < len(sql) {
			next = sql[i+1]
		}
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' {
			continue
		}
		if ch == '-' && next == '-' {
			i += 2
			for i < len(sql) && sql[i] != '\n' && sql[i] != '\r' {
				i++
			}
			i--
			continue
		}
		if ch == '/' && next == '*' {
			i += 2
			for i+1 < len(sql) && !(sql[i] == '*' && sql[i+1] == '/') {
				i++
			}
			if i+1 >= len(sql) {
				return false
			}
			i++
			continue
		}
		return true
	}
	return false
}

func readDollarQuoteTag(sql string) string {
	if len(sql) < 2 || sql[0] != '$' {
		return ""
	}
	for i := 1; i < len(sql); i++ {
		ch := sql[i]
		if ch == '$' {
			return sql[:i+1]
		}
		if !(ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || i > 1 && ch >= '0' && ch <= '9') {
			return ""
		}
	}
	return ""
}
