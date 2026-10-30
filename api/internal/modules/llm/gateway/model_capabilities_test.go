package gateway

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openCapabilityPostgresMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm: %v", err)
	}
	return db, mock, func() { _ = sqlDB.Close() }
}

func TestModelCapabilitiesUnionsLiveRows(t *testing.T) {
	db, mock, cleanup := openCapabilityPostgresMockDB(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{"id", "name", "structured_output", "json_mode", "function_calling", "vision"}).
		AddRow(uuid.NewString(), "deepseek-v4-pro", false, true, true, false).
		AddRow(uuid.NewString(), "deepseek-v4-pro", true, false, false, false)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "llm_models"`)).
		WithArgs("deepseek-v4-pro", true).
		WillReturnRows(rows)

	service := &llmGatewayServiceImpl{db: db}
	caps, err := service.ModelCapabilities(context.Background(), "deepseek-v4-pro")
	if err != nil {
		t.Fatalf("ModelCapabilities() error = %v", err)
	}
	if !caps.Known {
		t.Fatal("Known = false, want true")
	}
	if !caps.StructuredOutput || !caps.JSONMode || !caps.ToolCall || caps.Vision {
		t.Fatalf("capabilities = %+v, want the union of both rows", caps)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestModelCapabilitiesReadsRerankFromUseCases(t *testing.T) {
	for _, tc := range []struct {
		name             string
		useCases         []interface{}
		rerank, declared bool
	}{
		{"rerank declared", []interface{}{"{rerank}", "{text_chat}"}, true, true},
		{"chat only", []interface{}{"{text_chat}", "{}"}, false, true},
		{"legacy rows without use cases", []interface{}{"{}", "{}"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, cleanup := openCapabilityPostgresMockDB(t)
			defer cleanup()

			rows := sqlmock.NewRows([]string{"id", "name", "use_cases"})
			for _, useCases := range tc.useCases {
				rows.AddRow(uuid.NewString(), "qwen3-rerank", useCases)
			}
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "llm_models"`)).
				WithArgs("qwen3-rerank", true).
				WillReturnRows(rows)

			service := &llmGatewayServiceImpl{db: db}
			caps, err := service.ModelCapabilities(context.Background(), "qwen3-rerank")
			if err != nil {
				t.Fatalf("ModelCapabilities() error = %v", err)
			}
			if !caps.Known || caps.Rerank != tc.rerank || caps.UseCasesDeclared != tc.declared {
				t.Fatalf("capabilities = %+v, want Rerank=%v UseCasesDeclared=%v", caps, tc.rerank, tc.declared)
			}
		})
	}
}

func TestModelCapabilitiesMissingRowIsUnknown(t *testing.T) {
	db, mock, cleanup := openCapabilityPostgresMockDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "llm_models"`)).
		WithArgs("dynamic-model", true).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))

	service := &llmGatewayServiceImpl{db: db}
	caps, err := service.ModelCapabilities(context.Background(), "dynamic-model")
	if err != nil {
		t.Fatalf("ModelCapabilities() error = %v", err)
	}
	if caps.Known {
		t.Fatalf("Known = true for a missing row: %+v", caps)
	}
}

func TestModelCapabilitiesLookupErrorIsReported(t *testing.T) {
	db, mock, cleanup := openCapabilityPostgresMockDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "llm_models"`)).
		WithArgs("broken-model", true).
		WillReturnError(gorm.ErrInvalidDB)

	service := &llmGatewayServiceImpl{db: db}
	if _, err := service.ModelCapabilities(context.Background(), "broken-model"); err == nil {
		t.Fatal("ModelCapabilities() error = nil, want the database error")
	}
}

func TestModelCapabilitiesWithoutDatabaseIsUnknown(t *testing.T) {
	service := &llmGatewayServiceImpl{}
	caps, err := service.ModelCapabilities(context.Background(), "any-model")
	if err != nil {
		t.Fatalf("ModelCapabilities() error = %v", err)
	}
	if caps.Known {
		t.Fatalf("Known = true without a database: %+v", caps)
	}
}

func TestCapabilityGateEnabledSwitch(t *testing.T) {
	for _, value := range []string{"0", "false", "OFF", "disabled", "no"} {
		t.Setenv(CapabilityGateEnv, value)
		if CapabilityGateEnabled() {
			t.Fatalf("CapabilityGateEnabled() = true for %q", value)
		}
	}
	for _, value := range []string{"", "1", "on", "true"} {
		t.Setenv(CapabilityGateEnv, value)
		if !CapabilityGateEnabled() {
			t.Fatalf("CapabilityGateEnabled() = false for %q", value)
		}
	}
}
