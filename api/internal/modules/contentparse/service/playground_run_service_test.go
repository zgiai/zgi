package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
	"github.com/zgiai/ginext/internal/modules/contentparse/repository"
)

type fakePlaygroundRunRepository struct {
	created *model.PlaygroundRun
}

func (f *fakePlaygroundRunRepository) Create(_ context.Context, item *model.PlaygroundRun) error {
	f.created = item
	return nil
}

func (f *fakePlaygroundRunRepository) GetByID(context.Context, uuid.UUID, repository.PlaygroundRunListFilter) (*model.PlaygroundRun, error) {
	return f.created, nil
}

func (f *fakePlaygroundRunRepository) GetByShareToken(context.Context, string) (*model.PlaygroundRun, error) {
	return nil, nil
}

func (f *fakePlaygroundRunRepository) List(context.Context, repository.PlaygroundRunListFilter) ([]*model.PlaygroundRun, error) {
	return nil, nil
}

func (f *fakePlaygroundRunRepository) SetShareEnabled(context.Context, uuid.UUID, repository.PlaygroundRunListFilter, bool) (*model.PlaygroundRun, error) {
	return f.created, nil
}

func TestPlaygroundRunServiceCreateDefaultsShareDisabled(t *testing.T) {
	repo := &fakePlaygroundRunRepository{}
	svc := NewPlaygroundRunService(repo)
	item := &model.PlaygroundRun{IsShareEnabled: true}

	if err := svc.Create(context.Background(), item); err != nil {
		t.Fatalf("create playground run: %v", err)
	}
	if repo.created == nil {
		t.Fatal("expected repository create to be called")
	}
	if repo.created.ShareToken == "" {
		t.Fatal("expected share token to be generated for later explicit sharing")
	}
	if repo.created.IsShareEnabled {
		t.Fatal("expected saved playground run to keep sharing disabled by default")
	}
}
