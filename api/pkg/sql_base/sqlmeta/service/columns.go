package service

import (
	"context"
	"fmt"

	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/catalog/columns"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/types"
)

// ColumnsService orchestrates high-level column workflows.
type ColumnsService interface {
	ListColumns(ctx context.Context, opts types.ColumnListOptions) ([]types.Column, error)
	GetColumn(ctx context.Context, identifier types.ColumnIdentifier) (*types.Column, error)
	CreateColumn(ctx context.Context, input types.ColumnCreateInput) (*types.Column, error)
	UpdateColumn(ctx context.Context, id string, input types.ColumnUpdateInput) (*types.Column, error)
	DeleteColumn(ctx context.Context, id string, cascade bool) (*types.Column, error)
}

type columnsService struct {
	repo columns.Repository
}

var _ ColumnsService = (*columnsService)(nil)

// NewColumnsService constructs a columns service instance.
func NewColumnsService(repo columns.Repository) ColumnsService {
	return &columnsService{repo: repo}
}

func (s *columnsService) ensureRepo() (columns.Repository, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("sqlmeta: nil columns repository")
	}
	return s.repo, nil
}

func (s *columnsService) ListColumns(ctx context.Context, opts types.ColumnListOptions) ([]types.Column, error) {
	repo, err := s.ensureRepo()
	if err != nil {
		return nil, err
	}
	return repo.List(ctx, opts)
}

func (s *columnsService) GetColumn(ctx context.Context, identifier types.ColumnIdentifier) (*types.Column, error) {
	repo, err := s.ensureRepo()
	if err != nil {
		return nil, err
	}
	return repo.Retrieve(ctx, identifier)
}

func (s *columnsService) CreateColumn(ctx context.Context, input types.ColumnCreateInput) (*types.Column, error) {
	repo, err := s.ensureRepo()
	if err != nil {
		return nil, err
	}
	return repo.Create(ctx, input)
}

func (s *columnsService) UpdateColumn(ctx context.Context, id string, input types.ColumnUpdateInput) (*types.Column, error) {
	repo, err := s.ensureRepo()
	if err != nil {
		return nil, err
	}
	return repo.Update(ctx, id, input)
}

func (s *columnsService) DeleteColumn(ctx context.Context, id string, cascade bool) (*types.Column, error) {
	repo, err := s.ensureRepo()
	if err != nil {
		return nil, err
	}
	return repo.Delete(ctx, id, cascade)
}
