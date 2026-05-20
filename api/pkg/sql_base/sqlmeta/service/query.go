package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/catalog/query"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

// QueryService orchestrates raw SQL execution and error translation.
type QueryService interface {
	Execute(ctx context.Context, sql string, opts types.QueryOptions) (*types.QueryResponse, error)
}

type queryService struct {
	repo query.Repository
}

var _ QueryService = (*queryService)(nil)

// NewQueryService wires a query service backed by the repository.
func NewQueryService(repo query.Repository) QueryService {
	return &queryService{repo: repo}
}

// Execute runs arbitrary SQL and mirrors postgres-meta error semantics.
func (s *queryService) Execute(ctx context.Context, sql string, opts types.QueryOptions) (*types.QueryResponse, error) {
	if s.repo == nil {
		return nil, errors.New("sqlmeta: nil query repository")
	}

	rows, err := s.repo.Execute(ctx, sql, opts)
	if err != nil {
		if pgErr := new(pgconn.PgError); errors.As(err, &pgErr) {
			return &types.QueryResponse{
				Data:  nil,
				Error: buildPgError(pgErr, sql),
			}, nil
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			msg := "context deadline exceeded"
			if errors.Is(err, context.Canceled) {
				msg = "query canceled"
			}
			return &types.QueryResponse{
				Data: nil,
				Error: &types.QueryError{
					Message: msg,
					Code:    "57014", // PostgreSQL cancellation / timeout
				},
			}, nil
		}
		return nil, err
	}

	return &types.QueryResponse{Data: rows, Error: nil}, nil
}

func buildPgError(pgErr *pgconn.PgError, sql string) *types.QueryError {
	if pgErr == nil {
		return nil
	}

	var position *int
	if pgErr.Position > 0 {
		pos := int(pgErr.Position)
		position = &pos
	}

	return &types.QueryError{
		Message:        pgErr.Message,
		Code:           pgErr.Code,
		Detail:         pgErr.Detail,
		Hint:           pgErr.Hint,
		Where:          pgErr.Where,
		SchemaName:     pgErr.SchemaName,
		TableName:      pgErr.TableName,
		ColumnName:     pgErr.ColumnName,
		DataTypeName:   pgErr.DataTypeName,
		ConstraintName: pgErr.ConstraintName,
		FormattedError: query.FormatError(pgErr, sql),
		Position:       position,
	}
}
