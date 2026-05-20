package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/do/v2"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/catalog/query"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/driver"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/service"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

func TestQueryExecute(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Second)
	defer cancel()

	cfg := driver.Config{
		DBHost: "localhost",
		DBPort: "5432",
		DBUser: "postgres",
		DBPass: "Abc1234",
		DBName: "postgres",
	}

	pool, err := driver.NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	injector := do.New()
	do.ProvideValue(injector, pool)
	do.Provide(injector, query.ProvideRepository)
	do.Provide(injector, service.ProvideQueryService)

	querySvc := do.MustInvoke[service.QueryService](injector)

	resp, err := querySvc.Execute(ctx, "select 1 as value", types.QueryOptions{})
	if err != nil {
		t.Fatalf("execute select literal: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %#v", resp.Error)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 row, got %d", len(resp.Data))
	}
	if got := resp.Data[0]["value"]; got != int32(1) {
		t.Fatalf("expected value = 1, got %#v", got)
	}

	resp, err = querySvc.Execute(ctx, "select $1::int as value", types.QueryOptions{
		Parameters: []any{2},
	})
	if err != nil {
		t.Fatalf("execute parameterized query: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error for parameterized query: %#v", resp.Error)
	}
	if len(resp.Data) != 1 || resp.Data[0]["value"] != int32(2) {
		t.Fatalf("expected value = 2, got %#v", resp.Data)
	}

	// Add a test case to verify UUID is fetched from the database and printed
	resp, err = querySvc.Execute(ctx, "select gen_random_uuid() as uuid_value", types.QueryOptions{})
	if err != nil {
		t.Fatalf("execute uuid query: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error for uuid query: %#v", resp.Error)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 row, got %d", len(resp.Data))
	}
	uuidValue := resp.Data[0]["uuid_value"]
	t.Logf("Generated UUID: %v", uuidValue)
}

func TestQuerySpecialTypes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Second)
	defer cancel()

	cfg := driver.Config{
		DBHost: "localhost",
		DBPort: "5432",
		DBUser: "postgres",
		DBPass: "Abc1234",
		DBName: "postgres",
	}

	pool, err := driver.NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	injector := do.New()
	do.ProvideValue(injector, pool)
	do.Provide(injector, query.ProvideRepository)
	do.Provide(injector, service.ProvideQueryService)

	querySvc := do.MustInvoke[service.QueryService](injector)

	// Test UUID Type
	t.Run("UUID Type", func(t *testing.T) {
		resp, err := querySvc.Execute(ctx, "SELECT gen_random_uuid() as uuid_col", types.QueryOptions{})
		if err != nil {
			t.Fatalf("execute uuid query: %v", err)
		}
		if resp.Error != nil {
			t.Fatalf("unexpected error for uuid query: %#v", resp.Error)
		}
		if len(resp.Data) != 1 {
			t.Fatalf("expected 1 row, got %d", len(resp.Data))
		}

		uuidValue := resp.Data[0]["uuid_col"]
		t.Logf("UUID value: %v, type: %T", uuidValue, uuidValue)

		// Verify that it is returned as a string rather than a byte array
		if _, ok := uuidValue.(string); !ok {
			t.Fatalf("expected UUID to be returned as string, got %T", uuidValue)
		}

		// Verify it is in valid UUID format
		if _, err := uuid.Parse(uuidValue.(string)); err != nil {
			t.Fatalf("expected valid UUID format, got %v", uuidValue)
		}
	})

	// Test Date Type
	t.Run("Date Type", func(t *testing.T) {
		resp, err := querySvc.Execute(ctx, "SELECT CURRENT_DATE as date_col", types.QueryOptions{})
		if err != nil {
			t.Fatalf("execute date query: %v", err)
		}
		if resp.Error != nil {
			t.Fatalf("unexpected error for date query: %#v", resp.Error)
		}
		if len(resp.Data) != 1 {
			t.Fatalf("expected 1 row, got %d", len(resp.Data))
		}

		dateValue := resp.Data[0]["date_col"]
		t.Logf("Date value: %v, type: %T", dateValue, dateValue)

		// Verify that it is returned as time rather than other formats
		if _, ok := dateValue.(time.Time); !ok {
			// If not time.Time, at least should be a string
			if _, ok := dateValue.(string); !ok {
				t.Fatalf("expected Date to be returned as time.Time or string, got %T", dateValue)
			}
		}
	})

	// Test Timestamp Types
	t.Run("Timestamp Types", func(t *testing.T) {
		resp, err := querySvc.Execute(ctx, "SELECT NOW() as timestamp_col, NOW() as timestamptz_col", types.QueryOptions{})
		if err != nil {
			t.Fatalf("execute timestamp query: %v", err)
		}
		if resp.Error != nil {
			t.Fatalf("unexpected error for timestamp query: %#v", resp.Error)
		}
		if len(resp.Data) != 1 {
			t.Fatalf("expected 1 row, got %d", len(resp.Data))
		}

		timestampValue := resp.Data[0]["timestamp_col"]
		timestamptzValue := resp.Data[0]["timestamptz_col"]

		t.Logf("Timestamp value: %v, type: %T", timestampValue, timestampValue)
		t.Logf("Timestamptz value: %v, type: %T", timestamptzValue, timestamptzValue)

		// Verify that it is returned as time rather than other formats
		if _, ok := timestampValue.(time.Time); !ok {
			// If not time.Time, at least should be a string
			if _, ok := timestampValue.(string); !ok {
				t.Fatalf("expected Timestamp to be returned as time.Time or string, got %T", timestampValue)
			}
		}

		if _, ok := timestamptzValue.(time.Time); !ok {
			// If not time.Time, at least should be a string
			if _, ok := timestamptzValue.(string); !ok {
				t.Fatalf("expected Timestamptz to be returned as time.Time or string, got %T", timestamptzValue)
			}
		}
	})

	// Test Array Type
	t.Run("Array Type", func(t *testing.T) {
		resp, err := querySvc.Execute(ctx, "SELECT ARRAY['apple', 'banana', 'cherry'] as text_array_col", types.QueryOptions{})
		if err != nil {
			t.Fatalf("execute array query: %v", err)
		}
		if resp.Error != nil {
			t.Fatalf("unexpected error for array query: %#v", resp.Error)
		}
		if len(resp.Data) != 1 {
			t.Fatalf("expected 1 row, got %d", len(resp.Data))
		}

		arrayValue := resp.Data[0]["text_array_col"]
		t.Logf("Array value: %v, type: %T", arrayValue, arrayValue)

		// Verify that it is returned as a string slice rather than other formats
		if _, ok := arrayValue.([]string); !ok {
			// If not a string slice, at least should be a string representation
			if _, ok := arrayValue.(string); !ok {
				t.Fatalf("expected Array to be returned as []string or string, got %T", arrayValue)
			}
		}
	})
}

func TestQueryExecuteError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := driver.Config{
		DBHost: "localhost",
		DBPort: "5432",
		DBUser: "postgres",
		DBPass: "Abc1234",
		DBName: "postgres",
	}

	pool, err := driver.NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	injector := do.New()
	do.ProvideValue(injector, pool)
	do.Provide(injector, query.ProvideRepository)
	do.Provide(injector, service.ProvideQueryService)

	querySvc := do.MustInvoke[service.QueryService](injector)

	resp, err := querySvc.Execute(ctx, "select * from table_that_does_not_exist", types.QueryOptions{})
	//resp, err := querySvc.Execute(ctx, "select * from zgi_base_sqlmeta_column_type_table_20855718", types.QueryOptions{})
	if err != nil {
		t.Fatalf("execute invalid query: %v", err)
	}
	if resp.Error == nil {
		t.Fatalf("expected error for invalid query")
	}
	if resp.Error.Code != "42P01" {
		t.Fatalf("expected code 42P01, got %s", resp.Error.Code)
	}
	if resp.Error.FormattedError == "" || !strings.Contains(resp.Error.FormattedError, "LINE") {
		t.Fatalf("expected formatted error with LINE pointer, got %q", resp.Error.FormattedError)
	}
	if resp.Error.Position == nil {
		t.Fatalf("expected error position to be set")
	}
}
