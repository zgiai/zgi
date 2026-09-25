package sqlanalysis

import (
	"strings"

	"github.com/auxten/postgresql-parser/pkg/sql/parser"
	"github.com/auxten/postgresql-parser/pkg/sql/sem/tree"
	"github.com/auxten/postgresql-parser/pkg/walk"
)

// Analyze parses SQL and returns a deterministic structural description.
func Analyze(sql string) Analysis {
	parsed, err := parser.Parse(sql)
	if err != nil {
		return Analysis{Operation: OperationUnknown, ParseError: err.Error()}
	}

	a := Analysis{ReadOnly: true}
	for _, stmt := range parsed {
		if stmt.AST == nil {
			continue
		}
		a.StatementCount++
		a.Statements = appendAnalyzedStatements(a.Statements, stmt.AST)
		tag := strings.ToLower(stmt.AST.StatementTag())
		a.StatementTags = append(a.StatementTags, tag)
		op := classify(stmt.AST)
		a.Operation = mergeOperation(a.Operation, op)
		markStatementFeatures(&a, stmt.AST)
	}
	if a.StatementCount == 0 {
		a.Operation = OperationUnknown
		a.ReadOnly = false
		return a
	}
	for _, stmt := range a.Statements {
		if tree.CanWriteData(stmt) || tree.CanModifySchema(stmt) {
			a.ReadOnly = false
			break
		}
	}
	walkReferences(parsed, &a)
	walkAliasScopes(parsed, &a)
	walkDerivedScopes(parsed, &a)
	return a
}

type columnCollector struct {
	refs map[string]ColumnReference
}

func (v *columnCollector) VisitPre(expr tree.Expr) (bool, tree.Expr) {
	// A nested query owns a separate alias scope and is analysed independently.
	if _, ok := expr.(*tree.Subquery); ok {
		return false, expr
	}
	if ref, ok := columnReferenceFromExpr(expr); ok {
		key := strings.ToLower(ref.Qualifier + "." + ref.Name)
		v.refs[key] = ref
	}
	return true, expr
}

func (v *columnCollector) VisitPost(expr tree.Expr) tree.Expr { return expr }

func columnReferenceFromExpr(expr tree.Expr) (ColumnReference, bool) {
	switch n := expr.(type) {
	case *tree.ColumnItem:
		ref := ColumnReference{Name: n.Column()}
		if n.TableName != nil && n.TableName.NumParts > 0 {
			ref.Qualifier = n.TableName.Parts[0]
		}
		return ref, ref.Name != ""
	case *tree.UnresolvedName:
		if n.Star || n.NumParts == 0 || n.Parts[0] == "" {
			return ColumnReference{}, false
		}
		ref := ColumnReference{Name: n.Parts[0]}
		if n.NumParts > 1 {
			ref.Qualifier = n.Parts[1]
		}
		return ref, true
	default:
		return ColumnReference{}, false
	}
}

func collectExprColumns(expr tree.Expr, dst map[string]ColumnReference) {
	if expr == nil {
		return
	}
	collector := &columnCollector{refs: dst}
	_, _ = tree.WalkExpr(collector, expr)
}

func walkAliasScopes(stmts parser.Statements, a *Analysis) {
	allowed := make(map[string]ColumnReference)
	restricted := make(map[string]ColumnReference)
	w := &walk.AstWalker{Fn: func(_ interface{}, node interface{}) bool {
		selectNode, ok := node.(*tree.Select)
		if !ok {
			return false
		}
		clause := selectClause(selectNode.Select)
		if clause == nil {
			return false
		}
		aliases := make(map[string]struct{})
		for _, item := range clause.Exprs {
			if alias := strings.ToLower(string(item.As)); alias != "" {
				aliases[alias] = struct{}{}
			}
			collectExprColumns(item.Expr, restricted)
		}
		if clause.Where != nil {
			collectExprColumns(clause.Where.Expr, restricted)
		}
		if clause.Having != nil {
			collectExprColumns(clause.Having.Expr, restricted)
		}
		collectJoinColumns(clause.From.Tables, restricted)
		for _, expr := range clause.GroupBy {
			collectAllowedAlias(expr, aliases, allowed)
		}
		for _, order := range selectNode.OrderBy {
			if order != nil && order.OrderType == tree.OrderByColumn {
				collectAllowedAlias(order.Expr, aliases, allowed)
			}
		}
		return false
	}}
	_, _ = w.Walk(stmts, nil)
	for _, ref := range allowed {
		a.OutputAliasReferences = append(a.OutputAliasReferences, ref)
	}
	for _, ref := range restricted {
		a.RestrictedReferences = append(a.RestrictedReferences, ref)
	}
}

func walkDerivedScopes(stmts parser.Statements, a *Analysis) {
	resolved := make(map[string]ColumnReference)
	w := &walk.AstWalker{Fn: func(_ interface{}, node interface{}) bool {
		if _, ok := node.(*tree.Subquery); ok {
			a.HasSubquery = true
			return false
		}
		selectNode, ok := node.(*tree.Select)
		if !ok {
			return false
		}
		ctes := cteOutputColumns(selectNode.With)
		resolveDerivedReferences(selectNode, ctes, resolved)
		if selectNode.With != nil {
			for _, cte := range selectNode.With.CTEList {
				if cteSelect, ok := cte.Stmt.(*tree.Select); ok {
					resolveDerivedReferences(cteSelect, ctes, resolved)
				}
			}
		}
		return false
	}}
	_, _ = w.Walk(stmts, nil)
	for _, ref := range resolved {
		a.DerivedColumnReferences = append(a.DerivedColumnReferences, ref)
	}
}

func resolveDerivedReferences(selectNode *tree.Select, ctes map[string]map[string]struct{}, resolved map[string]ColumnReference) {
	clause := selectClause(selectNode.Select)
	if clause == nil {
		return
	}
	sources := make(map[string]map[string]struct{})
	collectDerivedSources(clause.From.Tables, ctes, sources)
	if len(sources) == 0 {
		return
	}
	refs := make(map[string]ColumnReference)
	for _, item := range clause.Exprs {
		collectExprColumns(item.Expr, refs)
	}
	if clause.Where != nil {
		collectExprColumns(clause.Where.Expr, refs)
	}
	if clause.Having != nil {
		collectExprColumns(clause.Having.Expr, refs)
	}
	for _, expr := range clause.GroupBy {
		collectExprColumns(expr, refs)
	}
	for _, order := range selectNode.OrderBy {
		if order != nil {
			collectExprColumns(order.Expr, refs)
		}
	}
	collectJoinColumns(clause.From.Tables, refs)
	for _, ref := range refs {
		if derivedSourceContains(sources, ref) {
			resolved[strings.ToLower(ref.Qualifier+"."+ref.Name)] = ref
		}
	}
}

func cteOutputColumns(with *tree.With) map[string]map[string]struct{} {
	result := make(map[string]map[string]struct{})
	if with == nil {
		return result
	}
	for _, cte := range with.CTEList {
		if cte == nil {
			continue
		}
		name := strings.ToLower(string(cte.Name.Alias))
		if name == "" {
			continue
		}
		if len(cte.Name.Cols) > 0 {
			columns := make(map[string]struct{}, len(cte.Name.Cols))
			for _, column := range cte.Name.Cols {
				columns[strings.ToLower(string(column))] = struct{}{}
			}
			result[name] = columns
			continue
		}
		result[name] = statementOutputColumns(cte.Stmt)
	}
	return result
}

func statementOutputColumns(stmt tree.Statement) map[string]struct{} {
	selectNode, ok := stmt.(*tree.Select)
	if !ok {
		return nil
	}
	return selectOutputColumns(selectNode.Select)
}

func selectOutputColumns(stmt tree.SelectStatement) map[string]struct{} {
	clause := selectClause(stmt)
	if clause == nil {
		return nil
	}
	columns := make(map[string]struct{}, len(clause.Exprs))
	for _, item := range clause.Exprs {
		name := strings.ToLower(string(item.As))
		if name == "" {
			if ref, ok := columnReferenceFromExpr(item.Expr); ok {
				name = strings.ToLower(ref.Name)
			}
		}
		if name != "" {
			columns[name] = struct{}{}
		}
	}
	return columns
}

func collectDerivedSources(exprs tree.TableExprs, ctes map[string]map[string]struct{}, dst map[string]map[string]struct{}) {
	for _, expr := range exprs {
		switch n := expr.(type) {
		case *tree.AliasedTableExpr:
			alias := strings.ToLower(string(n.As.Alias))
			switch source := n.Expr.(type) {
			case *tree.TableName:
				columns := ctes[strings.ToLower(source.Table())]
				if len(columns) > 0 {
					if alias == "" {
						alias = strings.ToLower(source.Table())
					}
					dst[alias] = columns
				}
			case *tree.Subquery:
				columns := selectOutputColumns(source.Select)
				if alias != "" && len(columns) > 0 {
					dst[alias] = columns
				}
			}
		case *tree.JoinTableExpr:
			collectDerivedSources(tree.TableExprs{n.Left, n.Right}, ctes, dst)
		case *tree.ParenTableExpr:
			collectDerivedSources(tree.TableExprs{n.Expr}, ctes, dst)
		case *tree.TableName:
			name := strings.ToLower(n.Table())
			if columns := ctes[name]; len(columns) > 0 {
				dst[name] = columns
			}
		}
	}
}

func derivedSourceContains(sources map[string]map[string]struct{}, ref ColumnReference) bool {
	name := strings.ToLower(ref.Name)
	if ref.Qualifier != "" {
		_, ok := sources[strings.ToLower(ref.Qualifier)][name]
		return ok
	}
	for _, columns := range sources {
		if _, ok := columns[name]; ok {
			return true
		}
	}
	return false
}

func selectClause(stmt tree.SelectStatement) *tree.SelectClause {
	switch n := stmt.(type) {
	case *tree.SelectClause:
		return n
	case *tree.ParenSelect:
		if n.Select != nil {
			return selectClause(n.Select.Select)
		}
	}
	return nil
}

func collectAllowedAlias(expr tree.Expr, aliases map[string]struct{}, dst map[string]ColumnReference) {
	refs := make(map[string]ColumnReference)
	collectExprColumns(expr, refs)
	for _, ref := range refs {
		if ref.Qualifier != "" {
			continue
		}
		if _, ok := aliases[strings.ToLower(ref.Name)]; ok {
			dst[strings.ToLower(ref.Name)] = ref
		}
	}
}

func collectJoinColumns(exprs tree.TableExprs, dst map[string]ColumnReference) {
	for _, expr := range exprs {
		switch n := expr.(type) {
		case *tree.AliasedTableExpr:
			if nested, ok := n.Expr.(tree.TableExpr); ok {
				collectJoinColumns(tree.TableExprs{nested}, dst)
			}
		case *tree.JoinTableExpr:
			collectJoinColumns(tree.TableExprs{n.Left, n.Right}, dst)
			if cond, ok := n.Cond.(*tree.OnJoinCond); ok {
				collectExprColumns(cond.Expr, dst)
			}
		case *tree.ParenTableExpr:
			collectJoinColumns(tree.TableExprs{n.Expr}, dst)
		}
	}
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
	case *tree.Insert:
		out = appendWithStatements(out, s.With)
	case *tree.Update:
		out = appendWithStatements(out, s.With)
	case *tree.Delete:
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

func walkReferences(stmts parser.Statements, a *Analysis) {
	tables := make(map[string]struct{})
	columns := make(map[string]struct{})
	functions := make(map[string]struct{})
	cteNames := make(map[string]struct{})
	a.TableAliases = make(map[string]TableReference)
	collector := &walk.AstWalker{Fn: func(ctx interface{}, node interface{}) bool {
		if cte, ok := node.(*tree.CTE); ok {
			name := strings.ToLower(string(cte.Name.Alias))
			if name != "" {
				cteNames[name] = struct{}{}
				a := ctx.(*Analysis)
				a.CTENames = append(a.CTENames, name)
			}
		}
		return false
	}}
	_, _ = collector.Walk(stmts, a)
	w := &walk.AstWalker{Fn: func(ctx interface{}, node interface{}) bool {
		out := ctx.(*Analysis)
		switch n := node.(type) {
		case *tree.AliasedTableExpr:
			if table, ok := n.Expr.(*tree.TableName); ok {
				alias := strings.ToLower(string(n.As.Alias))
				if alias != "" {
					out.TableAliases[alias] = TableReference{Schema: table.Schema(), Name: table.Table()}
				}
			}
		case *tree.TableName:
			ref := TableReference{Schema: n.Schema(), Name: n.Table()}
			if ref.Schema == "" {
				if _, virtual := cteNames[strings.ToLower(ref.Name)]; virtual {
					break
				}
			}
			key := strings.ToLower(ref.Schema + "." + ref.Name)
			if ref.Name != "" {
				if _, ok := tables[key]; !ok {
					tables[key] = struct{}{}
					out.Tables = append(out.Tables, ref)
				}
			}
		case *tree.ColumnItem:
			ref := ColumnReference{Name: n.Column()}
			if n.TableName != nil && n.TableName.NumParts > 0 {
				ref.Qualifier = n.TableName.Parts[0]
			}
			key := strings.ToLower(ref.Qualifier + "." + ref.Name)
			if _, ok := columns[key]; !ok {
				columns[key] = struct{}{}
				out.Columns = append(out.Columns, ref)
			}
		case *tree.UnresolvedName:
			if n.Star {
				out.HasWildcard = true
				break
			}
			if n.NumParts > 0 && n.Parts[0] != "" {
				ref := ColumnReference{Name: n.Parts[0]}
				if n.NumParts > 1 {
					ref.Qualifier = n.Parts[1]
				}
				key := strings.ToLower(ref.Qualifier + "." + ref.Name)
				if _, ok := columns[key]; !ok {
					columns[key] = struct{}{}
					out.Columns = append(out.Columns, ref)
				}
			}
		case *tree.FuncExpr:
			name := strings.ToLower(n.Func.String())
			if _, ok := functions[name]; !ok {
				functions[name] = struct{}{}
				out.Functions = append(out.Functions, name)
			}
		case *tree.SelectClause:
			joins := countJoins(n.From.Tables)
			out.JoinCount += joins
			if joins > out.MaxJoinCount {
				out.MaxJoinCount = joins
			}
		case *tree.Update:
			joins := countJoins(n.From)
			out.JoinCount += joins
			if joins > out.MaxJoinCount {
				out.MaxJoinCount = joins
			}
		}
		return false
	}}
	_, _ = w.Walk(stmts, a)
}

func classify(stmt tree.Statement) Operation {
	switch stmt.(type) {
	case *tree.Select:
		return OperationSelect
	case *tree.Insert:
		return OperationInsert
	case *tree.Update:
		return OperationUpdate
	case *tree.Delete:
		return OperationDelete
	case *tree.Explain:
		return OperationExplain
	}
	tag := strings.ToLower(stmt.StatementTag())
	switch {
	case strings.HasPrefix(tag, "show"):
		return OperationShow
	case strings.HasPrefix(tag, "create"), strings.HasPrefix(tag, "alter"), strings.HasPrefix(tag, "drop"), strings.HasPrefix(tag, "truncate"), strings.HasPrefix(tag, "grant"), strings.HasPrefix(tag, "revoke"):
		return OperationDDL
	default:
		return OperationOther
	}
}

func mergeOperation(current, next Operation) Operation {
	if current == "" || current == OperationUnknown {
		return next
	}
	if current == next {
		return current
	}
	return OperationMixed
}

func markStatementFeatures(a *Analysis, stmt tree.Statement) {
	switch s := stmt.(type) {
	case *tree.Select:
		a.HasCTE = a.HasCTE || s.With != nil
		a.HasLimit = a.HasLimit || s.Limit != nil
	case *tree.Insert:
		a.HasCTE = a.HasCTE || s.With != nil
	case *tree.Update:
		a.HasCTE = a.HasCTE || s.With != nil
	case *tree.Delete:
		a.HasCTE = a.HasCTE || s.With != nil
	}
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
