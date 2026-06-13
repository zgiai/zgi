package guard

import "testing"

func TestCheckMultiStatementAndDrop(t *testing.T) {
	result := Check("SELECT 1; DROP TABLE users", DefaultPolicy())
	assertReason(t, result, ReasonMultiStatement)
	assertReason(t, result, ReasonBlockStatement)
}

func TestCheckAllowsTrailingSemicolon(t *testing.T) {
	result := Check("SELECT 1;", DefaultPolicy())
	if result.Verdict != VerdictAllow {
		t.Fatalf("verdict = %s, reasons = %#v", result.Verdict, result.Reasons)
	}
}

func TestCheckIgnoresSemicolonInDollarQuotedString(t *testing.T) {
	result := Check("SELECT $$a;b$$;", DefaultPolicy())
	if result.Verdict != VerdictAllow {
		t.Fatalf("verdict = %s, reasons = %#v", result.Verdict, result.Reasons)
	}
}

func TestCheckScansMultiStatementAfterParseFailure(t *testing.T) {
	policy := DefaultPolicy()
	policy.AllowParseFailure = true
	result := Check("SELECT @@@; DROP TABLE users", policy)
	assertReason(t, result, ReasonMultiStatement)
}

func TestCheckBlockFunction(t *testing.T) {
	result := Check("SELECT pg_read_file('/etc/passwd')", DefaultPolicy())
	assertReason(t, result, ReasonBlockFunction)
}

func TestCheckRequireWhere(t *testing.T) {
	result := Check("UPDATE users SET name = 'x'", DefaultPolicy())
	assertReason(t, result, ReasonRequireWhere)
}

func TestCheckReadonlyBlocksWrites(t *testing.T) {
	policy := DefaultPolicy()
	policy.Readonly = true
	for _, sql := range []string{
		"INSERT INTO users(id) VALUES (1)",
		"UPDATE users SET name = 'x' WHERE id = 1",
		"DELETE FROM users WHERE id = 1",
	} {
		result := Check(sql, policy)
		assertReason(t, result, ReasonReadonly)
	}
}

func TestCheckMaxJoinDepth(t *testing.T) {
	policy := DefaultPolicy()
	policy.MaxJoinDepth = 1
	result := Check("SELECT * FROM a JOIN b ON a.id = b.id JOIN c ON b.id = c.id", policy)
	assertReason(t, result, ReasonMaxJoinDepth)
}

func TestWarnModeDoesNotDenyAction(t *testing.T) {
	policy := DefaultPolicy()
	policy.Mode = ModeWarn
	result := Check("DROP TABLE users", policy)
	if result.Action != ActionAllow {
		t.Fatalf("action = %s, want %s", result.Action, ActionAllow)
	}
	if result.Verdict != VerdictDeny {
		t.Fatalf("verdict = %s, want %s", result.Verdict, VerdictDeny)
	}
}

func TestEnforceModeDeniesAction(t *testing.T) {
	policy := DefaultPolicy()
	policy.Mode = ModeEnforce
	result := Check("DROP TABLE users", policy)
	if result.Action != ActionDeny {
		t.Fatalf("action = %s, want %s", result.Action, ActionDeny)
	}
}

func assertReason(t *testing.T, result Result, code ReasonCode) {
	t.Helper()
	for _, reason := range result.Reasons {
		if reason.Code == code {
			return
		}
	}
	t.Fatalf("reason %s not found in %#v", code, result.Reasons)
}
