package service

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/catalog/indexes"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

// IndexesService orchestrates read flows for index metadata.
type IndexesService interface {
	List(ctx context.Context, opts types.IndexListOptions) ([]types.Index, error)
	Retrieve(ctx context.Context, id int64) (*types.Index, error)
}

type indexesService struct {
	repo indexes.Repository
}

var _ IndexesService = (*indexesService)(nil)

// NewIndexesService constructs an indexes service instance.
func NewIndexesService(repo indexes.Repository) IndexesService {
	return &indexesService{repo: repo}
}

func (s *indexesService) ensureRepo() (indexes.Repository, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("sqlmeta: nil indexes repository")
	}
	return s.repo, nil
}

func (s *indexesService) List(ctx context.Context, opts types.IndexListOptions) ([]types.Index, error) {
	repo, err := s.ensureRepo()
	if err != nil {
		return nil, err
	}
	return repo.List(ctx, opts)
}

func (s *indexesService) Retrieve(ctx context.Context, id int64) (*types.Index, error) {
	repo, err := s.ensureRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetByID(ctx, id)
}
