package sql_base

import (
	"strings"
	"testing"
)

func TestWrapMutationForAffectedRowsWrapsPlainMutations(t *testing.T) {
	cases := []string{
		`INSERT INTO "t" ("a", "b") VALUES ($1, $2), ($3, $4)`,
		`UPDATE "t" SET "a" = $1, updated_time = NOW() WHERE "id" = $2`,
		`DELETE FROM "t" WHERE "id" = ANY($1);`,
	}
	for _, query := range cases {
		wrapped, ok := wrapMutationForAffectedRows(query)
		if !ok {
			t.Fatalf("expected %q to be wrapped", query)
		}
		if !strings.HasPrefix(wrapped, "WITH zgi_affected AS (") || !strings.Contains(wrapped, "\nRETURNING 1\n)") {
			t.Fatalf("unexpected wrapper for %q: %s", query, wrapped)
		}
		if strings.Contains(wrapped, ";") {
			t.Fatalf("wrapper kept a statement terminator: %s", wrapped)
		}
	}
}

func TestWrapMutationForAffectedRowsIgnoresDelimitersInsideQuotes(t *testing.T) {
	cases := []string{
		`UPDATE "t" SET note = 'a--b' WHERE id = 1`,
		`UPDATE "t" SET note = 'x; y /* z */' WHERE id = 1`,
		`INSERT INTO "t" ("note") VALUES ('it''s; fine')`,
		`INSERT INTO "t" ("note") VALUES (E'a\'; --b')`,
		`UPDATE "odd;--name" SET "a" = $1 WHERE "id" = $2`,
		`UPDATE "t" SET note = $q$a; -- b$q$ WHERE id = $1`,
	}
	for _, query := range cases {
		if _, ok := wrapMutationForAffectedRows(query); !ok {
			t.Fatalf("expected %q to be wrapped", query)
		}
	}
}

func TestWrapMutationForAffectedRowsLeavesOtherStatementsUnchanged(t *testing.T) {
	cases := []string{
		`SELECT * FROM "t"`,
		`INSERT INTO "t" ("a") VALUES (1) RETURNING id`,
		`DELETE FROM "t" WHERE id = 1 RETURNING *`,
		`WITH x AS (SELECT 1) DELETE FROM "t" WHERE id IN (SELECT * FROM x)`,
		`DELETE FROM "t"; DELETE FROM "u"`,
		`DELETE FROM "t" -- trailing comment`,
		`DELETE FROM "t" WHERE note = 'x' /* trailing */`,
		`DELETE FROM "t" WHERE note = 'unterminated`,
		`UPDATE "t" SET note = 'a'; DELETE FROM "u"`,
		`CREATE TABLE "t" (id int)`,
		`not sql at all`,
	}
	for _, query := range cases {
		got, ok := wrapMutationForAffectedRows(query)
		if ok || got != query {
			t.Fatalf("expected %q to stay unchanged, got %q (ok=%v)", query, got, ok)
		}
	}
}

func TestParseWrappedAffectedRows(t *testing.T) {
	cases := []struct {
		value any
		want  int64
	}{
		{value: float64(3), want: 3},
		{value: "42", want: 42},
	}
	for _, tc := range cases {
		got, ok := parseWrappedAffectedRows([]map[string]interface{}{{externalAffectedRowsColumn: tc.value}})
		if !ok || got != tc.want {
			t.Fatalf("parse %v = %d (ok=%v), want %d", tc.value, got, ok, tc.want)
		}
	}
	if _, ok := parseWrappedAffectedRows(nil); ok {
		t.Fatal("expected empty result to be unreadable")
	}
}
