package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/catalog/tables"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

const tableNamePrefix = "zgi_base_"

// TablesService orchestrates create/read flows for tables.
type TablesService interface {
	List(ctx context.Context, opts types.TableListOptions) ([]types.Table, error)
	Retrieve(ctx context.Context, identifier types.TableIdentifier, includeColumns bool) (*types.Table, error)
	Create(ctx context.Context, input types.TableCreateInput) (*types.Table, error)
	Update(ctx context.Context, id int64, input types.TableUpdateInput) (*types.Table, error)
	Delete(ctx context.Context, id int64, opts types.TableDeleteOptions) (*types.Table, error)
}

type tablesService struct {
	repo tables.Repository
}

var _ TablesService = (*tablesService)(nil)

// NewTablesService wires a tables service instance.
func NewTablesService(repo tables.Repository) TablesService {
	return &tablesService{repo: repo}
}

// Create delegates to the repository and hosts future orchestration logic.
func (s *tablesService) List(ctx context.Context, opts types.TableListOptions) ([]types.Table, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("sqlmeta: nil tables repository")
	}
	return s.repo.List(ctx, opts)
}

// Retrieve proxies to the repository.
func (s *tablesService) Retrieve(ctx context.Context, identifier types.TableIdentifier, includeColumns bool) (*types.Table, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("sqlmeta: nil tables repository")
	}
	if identifier.ID != 0 {
		return s.repo.GetByID(ctx, identifier.ID, includeColumns)
	}
	if identifier.Schema != "" && identifier.Name != "" {
		return s.repo.GetByName(ctx, identifier.Schema, applyPrefix(identifier.Name), includeColumns)
	}
	if identifier.Name != "" {
		return s.repo.GetByName(ctx, "public", applyPrefix(identifier.Name), includeColumns)
	}
	return nil, fmt.Errorf("sqlmeta: table identifier requires id or schema/name")
}

func (s *tablesService) Create(ctx context.Context, input types.TableCreateInput) (*types.Table, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("sqlmeta: nil tables repository")
	}
	input.Name = applyPrefix(input.Name)
	return s.repo.Create(ctx, input)
}

func (s *tablesService) Update(ctx context.Context, id int64, input types.TableUpdateInput) (*types.Table, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("sqlmeta: nil tables repository")
	}
	if input.Name != nil {
		normalized := applyPrefix(*input.Name)
		input.Name = &normalized
	}
	return s.repo.Update(ctx, id, input)
}

func (s *tablesService) Delete(ctx context.Context, id int64, opts types.TableDeleteOptions) (*types.Table, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("sqlmeta: nil tables repository")
	}
	return s.repo.Delete(ctx, id, opts.Cascade)
}

func applyPrefix(name string) string {
	if strings.HasPrefix(name, tableNamePrefix) {
		return name
	}
	return tableNamePrefix + name
}
