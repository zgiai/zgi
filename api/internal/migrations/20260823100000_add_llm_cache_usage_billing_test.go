package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddLLMCacheUsageBillingMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.llm_usage_bills.*ALTER TABLE public.llm_model_configs.*ALTER TABLE public.llm_models.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddLLMCacheUsageBilling(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS cache_read_tokens bigint NOT NULL DEFAULT 0",
		"ADD COLUMN IF NOT EXISTS cache_write_tokens bigint NOT NULL DEFAULT 0",
		"total_tokens = prompt_tokens + cache_read_tokens + cache_write_tokens + completion_tokens",
		"ADD COLUMN IF NOT EXISTS cache_read_price_override numeric(10,6)",
		"ADD COLUMN IF NOT EXISTS cache_write_price_override numeric(10,6)",
		"ADD COLUMN IF NOT EXISTS cache_read_price_configured boolean NOT NULL DEFAULT false",
		"ADD COLUMN IF NOT EXISTS cache_write_price_configured boolean NOT NULL DEFAULT false",
		"ALTER COLUMN cost_cache_read TYPE numeric(10,6)",
		"ALTER COLUMN cost_cache_write TYPE numeric(10,6)",
	} {
		if !strings.Contains(statements, want) {
			t.Fatalf("cache usage billing migration missing %q:\n%s", want, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
