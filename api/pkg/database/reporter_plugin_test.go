package database

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zgiai/zgi/api/internal/observability"
)

func TestClassifyDatabaseErrorKeepsSQLStateSafeAndActionable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		code      string
		source    observability.ErrorSource
		retryable bool
	}{
		{name: "query bug", err: &pgconn.PgError{Code: "42601"}, code: "postgres_42601", source: observability.ErrorSourceZGI},
		{name: "authentication rejection", err: fmt.Errorf("connect: %w", &pgconn.PgError{Code: "28P01"}), code: "postgres_28P01", source: observability.ErrorSourceZGI},
		{name: "connection", err: &pgconn.PgError{Code: "08006"}, code: "postgres_08006", source: observability.ErrorSourceInfrastructure, retryable: true},
		{name: "network disconnect", err: &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}, code: "database_transport_failed", source: observability.ErrorSourceInfrastructure, retryable: true},
		{name: "timeout", err: context.DeadlineExceeded, code: "database_timeout", source: observability.ErrorSourceInfrastructure, retryable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := classifyDatabaseError(test.err)
			if got.Code != test.code || got.Source != test.source || got.Retryable != test.retryable {
				t.Fatalf("classification = %#v", got)
			}
		})
	}
}

func TestOperationErrorPreservesDatabaseBoundaryAndCause(t *testing.T) {
	cause := context.DeadlineExceeded
	err := WrapOperationError("list models", cause)

	if !IsOperationError(err) || !errors.Is(err, cause) {
		t.Fatalf("error = %v, want marked operation with original cause", err)
	}
}
