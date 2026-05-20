package schemas

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/driver"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

// Repository defines the minimal schema operations at the catalog layer for composition by upper services.
type Repository interface {
	List(ctx context.Context, opts types.SchemaListOptions) ([]types.Schema, error)
	GetByID(ctx context.Context, id int64) (*types.Schema, error)
	GetByName(ctx context.Context, name string) (*types.Schema, error)
	Create(ctx context.Context, input types.SchemaCreateInput) (*types.Schema, error)
	Update(ctx context.Context, id int64, input types.SchemaUpdateInput) (*types.Schema, error)
	Delete(ctx context.Context, id int64, cascade bool) error
}

type repository struct {
	pool *driver.Pool
}

var _ Repository = (*repository)(nil)

// NewRepository constructs a schemas repository backed by a pgx pool.
func NewRepository(pool *driver.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) List(ctx context.Context, opts types.SchemaListOptions) ([]types.Schema, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	sql, args := buildListQuery(opts)
	rows, err := raw.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlmeta: list schemas: %w", err)
	}
	defer rows.Close()

	var out []types.Schema
	for rows.Next() {
		var s types.Schema
		if err := rows.Scan(&s.ID, &s.Name, &s.Owner); err != nil {
			return nil, fmt.Errorf("sqlmeta: scan schema: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlmeta: iter schemas: %w", err)
	}

	return out, nil
}

func (r *repository) GetByID(ctx context.Context, id int64) (*types.Schema, error) {
	return r.retrieve(ctx, types.SchemaIdentifier{ID: id})
}

func (r *repository) GetByName(ctx context.Context, name string) (*types.Schema, error) {
	return r.retrieve(ctx, types.SchemaIdentifier{Name: name})
}

func (r *repository) retrieve(ctx context.Context, identifier types.SchemaIdentifier) (*types.Schema, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	sql, args, err := buildRetrieveQuery(identifier)
	if err != nil {
		return nil, err
	}

	var s types.Schema
	if err := raw.QueryRow(ctx, sql, args...).Scan(&s.ID, &s.Name, &s.Owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("sqlmeta: schema not found")
		}
		return nil, fmt.Errorf("sqlmeta: retrieve schema: %w", err)
	}
	return &s, nil
}

func (r *repository) Create(ctx context.Context, input types.SchemaCreateInput) (*types.Schema, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}
	if strings.TrimSpace(input.Name) == "" {
		return nil, errors.New("sqlmeta: schema name required")
	}

	owner := input.Owner
	if owner == "" {
		owner = "postgres"
	}

	stmt := fmt.Sprintf("CREATE SCHEMA %s AUTHORIZATION %s;", pgx.Identifier{input.Name}.Sanitize(), pgx.Identifier{owner}.Sanitize())
	if _, err := raw.Exec(ctx, stmt); err != nil {
		return nil, fmt.Errorf("sqlmeta: create schema: %w", err)
	}

	return r.retrieve(ctx, types.SchemaIdentifier{Name: input.Name})
}

func (r *repository) Update(ctx context.Context, id int64, input types.SchemaUpdateInput) (*types.Schema, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	existing, err := r.retrieve(ctx, types.SchemaIdentifier{ID: id})
	if err != nil {
		return nil, err
	}

	tx, err := raw.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("sqlmeta: begin schema update: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if input.Owner != nil {
		stmt := fmt.Sprintf("ALTER SCHEMA %s OWNER TO %s;", pgx.Identifier{existing.Name}.Sanitize(), pgx.Identifier{*input.Owner}.Sanitize())
		if _, err = tx.Exec(ctx, stmt); err != nil {
			return nil, fmt.Errorf("sqlmeta: alter schema owner: %w", err)
		}
	}

	newName := existing.Name
	if input.Name != nil {
		stmt := fmt.Sprintf("ALTER SCHEMA %s RENAME TO %s;", pgx.Identifier{existing.Name}.Sanitize(), pgx.Identifier{*input.Name}.Sanitize())
		if _, err = tx.Exec(ctx, stmt); err != nil {
			return nil, fmt.Errorf("sqlmeta: rename schema: %w", err)
		}
		newName = *input.Name
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("sqlmeta: commit schema update: %w", err)
	}

	return r.retrieve(ctx, types.SchemaIdentifier{Name: newName})
}

func (r *repository) Delete(ctx context.Context, id int64, cascade bool) error {
	raw := r.rawPool()
	if raw == nil {
		return errors.New("sqlmeta: nil pool")
	}

	existing, err := r.retrieve(ctx, types.SchemaIdentifier{ID: id})
	if err != nil {
		return err
	}

	cascadeOption := "RESTRICT"
	if cascade {
		cascadeOption = "CASCADE"
	}

	stmt := fmt.Sprintf("DROP SCHEMA %s %s;", pgx.Identifier{existing.Name}.Sanitize(), cascadeOption)
	if _, err := raw.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("sqlmeta: drop schema: %w", err)
	}

	return nil
}

func (r *repository) rawPool() *pgxpool.Pool {
	if r == nil || r.pool == nil {
		return nil
	}
	return r.pool.Raw()
}
