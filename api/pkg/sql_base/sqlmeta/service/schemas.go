package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/catalog/schemas"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/types"
)

// SchemasService exposes schema-level workflows to callers.
type SchemasService interface {
	List(ctx context.Context, opts types.SchemaListOptions) ([]types.Schema, error)
	Retrieve(ctx context.Context, identifier types.SchemaIdentifier) (*types.Schema, error)
	Create(ctx context.Context, input types.SchemaCreateInput) (*types.Schema, error)
	Update(ctx context.Context, id int64, input types.SchemaUpdateInput) (*types.Schema, error)
	Delete(ctx context.Context, id int64, opts types.SchemaDropOptions) (*types.Schema, error)
}

type schemasService struct {
	repo schemas.Repository
}

var _ SchemasService = (*schemasService)(nil)

// NewSchemasService constructs a schemas service instance.
func NewSchemasService(repo schemas.Repository) SchemasService {
	return &schemasService{repo: repo}
}

func (s *schemasService) ensureRepo() (schemas.Repository, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("sqlmeta: nil schemas repository")
	}
	return s.repo, nil
}

func (s *schemasService) List(ctx context.Context, opts types.SchemaListOptions) ([]types.Schema, error) {
	repo, err := s.ensureRepo()
	if err != nil {
		return nil, err
	}
	return repo.List(ctx, opts)
}

func (s *schemasService) Retrieve(ctx context.Context, identifier types.SchemaIdentifier) (*types.Schema, error) {
	repo, err := s.ensureRepo()
	if err != nil {
		return nil, err
	}

	switch {
	case identifier.ID != 0:
		return repo.GetByID(ctx, identifier.ID)
	case identifier.Name != "":
		return repo.GetByName(ctx, identifier.Name)
	default:
		return nil, errors.New("sqlmeta: schema identifier requires id or name")
	}
}

func (s *schemasService) Create(ctx context.Context, input types.SchemaCreateInput) (*types.Schema, error) {
	repo, err := s.ensureRepo()
	if err != nil {
		return nil, err
	}
	return repo.Create(ctx, input)
}

func (s *schemasService) Update(ctx context.Context, id int64, input types.SchemaUpdateInput) (*types.Schema, error) {
	repo, err := s.ensureRepo()
	if err != nil {
		return nil, err
	}
	return repo.Update(ctx, id, input)
}

func (s *schemasService) Delete(ctx context.Context, id int64, opts types.SchemaDropOptions) (*types.Schema, error) {
	repo, err := s.ensureRepo()
	if err != nil {
		return nil, err
	}

	existing, err := repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := repo.Delete(ctx, id, opts.Cascade); err != nil {
		return nil, err
	}

	return existing, nil
}
