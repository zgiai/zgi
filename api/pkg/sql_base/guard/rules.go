package guard

import (
	"fmt"
	"strings"

	"github.com/auxten/postgresql-parser/pkg/sql/sem/tree"
)

func checkParseError(_ string, policy Policy, a analysis) (Reason, bool) {
	if a.parseErr == nil || policy.AllowParseFailure {
		return Reason{}, false
	}
	return Reason{
		Code:    ReasonParseError,
		Message: "SQL could not be parsed",
		Detail:  a.parseErr.Error(),
	}, true
}

func checkMultiStatement(sql string, policy Policy, a analysis) (Reason, bool) {
	if policy.AllowMultiStmt {
		return Reason{}, false
	}
	if len(a.statements) > 1 || hasNonTrailingSemicolon(sql) {
		return Reason{
			Code:    ReasonMultiStatement,
			Message: "multiple SQL statements are not allowed",
		}, true
	}
	return Reason{}, false
}

func checkBlockStatement(_ string, policy Policy, a analysis) (Reason, bool) {
	if a.parseErr != nil {
		return Reason{}, false
	}
	blocked := makeSet(policy.BlockStatements)
	for _, stmt := range a.statements {
		tag := strings.ToLower(stmt.StatementTag())
		verb := tag
		if idx := strings.IndexByte(verb, ' '); idx >= 0 {
			verb = verb[:idx]
		}
		if _, ok := blocked[tag]; ok {
			return blockStatementReason(tag), true
		}
		if _, ok := blocked[verb]; ok {
			return blockStatementReason(tag), true
		}
	}
	return Reason{}, false
}

func checkBlockFunction(_ string, policy Policy, a analysis) (Reason, bool) {
	if a.parseErr != nil {
		return Reason{}, false
	}
	blocked := makeSet(policy.BlockFunctions)
	for _, fn := range a.functions {
		name := lastNamePart(fn)
		if _, ok := blocked[fn]; ok {
			return blockFunctionReason(fn), true
		}
		if _, ok := blocked[name]; ok {
			return blockFunctionReason(name), true
		}
	}
	return Reason{}, false
}

func checkRequireWhere(_ string, policy Policy, a analysis) (Reason, bool) {
	if !policy.RequireWhere || a.parseErr != nil {
		return Reason{}, false
	}
	for _, stmt := range a.statements {
		switch s := stmt.(type) {
		case *tree.Update:
			if s.Where == nil {
				return Reason{
					Code:    ReasonRequireWhere,
					Message: "UPDATE statements require a WHERE clause",
					Detail:  "UPDATE",
				}, true
			}
		case *tree.Delete:
			if s.Where == nil {
				return Reason{
					Code:    ReasonRequireWhere,
					Message: "DELETE statements require a WHERE clause",
					Detail:  "DELETE",
				}, true
			}
		}
	}
	return Reason{}, false
}

func checkReadonly(_ string, policy Policy, a analysis) (Reason, bool) {
	if !policy.Readonly || a.parseErr != nil {
		return Reason{}, false
	}
	for _, stmt := range a.statements {
		if tree.CanWriteData(stmt) || tree.CanModifySchema(stmt) {
			return Reason{
				Code:    ReasonReadonly,
				Message: "readonly mode allows SELECT-only SQL",
				Detail:  stmt.StatementTag(),
			}, true
		}
	}
	return Reason{}, false
}

func checkMaxJoinDepth(_ string, policy Policy, a analysis) (Reason, bool) {
	if policy.MaxJoinDepth <= 0 || a.parseErr != nil {
		return Reason{}, false
	}
	if a.maxJoins > policy.MaxJoinDepth {
		return Reason{
			Code:    ReasonMaxJoinDepth,
			Message: "SQL exceeds maximum join depth",
			Detail:  fmt.Sprintf("joins=%d max=%d", a.maxJoins, policy.MaxJoinDepth),
		}, true
	}
	return Reason{}, false
}

func blockStatementReason(statement string) Reason {
	return Reason{
		Code:    ReasonBlockStatement,
		Message: "SQL statement type is blocked",
		Detail:  statement,
	}
}

func blockFunctionReason(function string) Reason {
	return Reason{
		Code:    ReasonBlockFunction,
		Message: "SQL function is blocked",
		Detail:  function,
	}
}

func makeSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}

func lastNamePart(value string) string {
	value = strings.Trim(value, `"`)
	parts := strings.Split(value, ".")
	return strings.Trim(parts[len(parts)-1], `"`)
}
