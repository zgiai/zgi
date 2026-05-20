package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPlaygroundRunShareRequiresExplicitEnable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.PlaygroundRun{}); err != nil {
		t.Fatalf("migrate playground runs: %v", err)
	}

	workspaceID := uuid.New()
	run := &model.PlaygroundRun{
		ID:                   uuid.New(),
		WorkspaceID:          &workspaceID,
		FileName:             "sample.pdf",
		SourceContentHash:    "abc123",
		RequestedProviderKey: "local",
		Profile:              "auto",
		Status:               "succeeded",
		QualityLevel:         "standard",
		ShareToken:           "share-token",
		IsShareEnabled:       false,
	}

	repo := NewPlaygroundRunRepository(db)
	if err := repo.Create(context.Background(), run); err != nil {
		t.Fatalf("create playground run: %v", err)
	}
	if item, err := repo.GetByShareToken(context.Background(), "share-token"); err != nil {
		t.Fatalf("get disabled share token: %v", err)
	} else if item != nil {
		t.Fatal("expected disabled share token to be unreadable")
	}

	otherWorkspaceID := uuid.New()
	if item, err := repo.SetShareEnabled(context.Background(), run.ID, PlaygroundRunListFilter{WorkspaceID: &otherWorkspaceID}, true); err != nil {
		t.Fatalf("enable share from other workspace: %v", err)
	} else if item != nil {
		t.Fatal("expected other workspace to be unable to enable share")
	}
	if item, err := repo.GetByID(context.Background(), run.ID, PlaygroundRunListFilter{}); err != nil {
		t.Fatalf("get without scope: %v", err)
	} else if item != nil {
		t.Fatal("expected unscoped public lookup to be denied")
	}
	if item, err := repo.GetByID(context.Background(), run.ID, PlaygroundRunListFilter{AllowUnscoped: true}); err != nil {
		t.Fatalf("get internal unscoped: %v", err)
	} else if item == nil || item.ID != run.ID {
		t.Fatal("expected internal unscoped lookup to resolve the run")
	}

	if item, err := repo.SetShareEnabled(context.Background(), run.ID, PlaygroundRunListFilter{WorkspaceID: &workspaceID}, true); err != nil {
		t.Fatalf("enable share: %v", err)
	} else if item == nil || !item.IsShareEnabled {
		t.Fatal("expected owner workspace to enable share")
	}
	if item, err := repo.GetByShareToken(context.Background(), "share-token"); err != nil {
		t.Fatalf("get enabled share token: %v", err)
	} else if item == nil || item.ID != run.ID {
		t.Fatal("expected enabled share token to resolve the run")
	}
}
