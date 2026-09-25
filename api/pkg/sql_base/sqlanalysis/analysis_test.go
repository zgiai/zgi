package sqlanalysis

import "testing"

func TestAnalyzeDescribesStatementsAndReferences(t *testing.T) {
	t.Parallel()

	got := Analyze(`WITH paid AS (SELECT customer_id FROM public.orders WHERE status = 'paid')
SELECT c.name FROM public.customers AS c JOIN paid AS p ON p.customer_id = c.id`)
	if got.ParseError != "" {
		t.Fatalf("ParseError = %q, want empty", got.ParseError)
	}
	if got.StatementCount != 1 {
		t.Fatalf("StatementCount = %d, want 1", got.StatementCount)
	}
	if got.Operation != OperationSelect || !got.ReadOnly {
		t.Fatalf("Operation/ReadOnly = %q/%v, want select/true", got.Operation, got.ReadOnly)
	}
	if !got.HasCTE || got.JoinCount != 1 {
		t.Fatalf("HasCTE/JoinCount = %v/%d, want true/1", got.HasCTE, got.JoinCount)
	}
	if !containsTable(got.Tables, "public", "orders") || !containsTable(got.Tables, "public", "customers") {
		t.Fatalf("Tables = %#v, want public.orders and public.customers", got.Tables)
	}
	if containsTable(got.Tables, "", "paid") {
		t.Fatalf("Tables = %#v, CTE name must not be treated as a physical table", got.Tables)
	}
}

func TestAnalyzeRejectsInvalidSQL(t *testing.T) {
	t.Parallel()

	got := Analyze("SELECT FROM")
	if got.ParseError == "" {
		t.Fatal("ParseError is empty, want parser failure")
	}
	if got.ReadOnly {
		t.Fatal("ReadOnly = true for invalid SQL")
	}
}

func TestAnalyzeClassifiesWriteAndMultipleStatements(t *testing.T) {
	t.Parallel()

	got := Analyze("UPDATE public.users SET active = false WHERE id = 1; SELECT 1")
	if got.StatementCount != 2 {
		t.Fatalf("StatementCount = %d, want 2", got.StatementCount)
	}
	if got.Operation != OperationMixed || got.ReadOnly {
		t.Fatalf("Operation/ReadOnly = %q/%v, want mixed/false", got.Operation, got.ReadOnly)
	}
}

func containsTable(tables []TableReference, schema, name string) bool {
	for _, table := range tables {
		if table.Schema == schema && table.Name == name {
			return true
		}
	}
	return false
}
