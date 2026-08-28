package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTripleMentionUpdateStatusBindsRelationship(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE kb_triple_mentions (
			id text PRIMARY KEY,
			status text,
			head_entity_id text,
			tail_entity_id text,
			relationship_id text
		)
	`).Error; err != nil {
		t.Fatalf("create triple mentions table: %v", err)
	}

	mention := &model.TripleMention{
		ID:     uuid.New(),
		Status: "pending",
	}
	if err := db.Exec("INSERT INTO kb_triple_mentions (id, status) VALUES (?, ?)", mention.ID, mention.Status).Error; err != nil {
		t.Fatalf("create triple mention: %v", err)
	}

	headID := uuid.New()
	tailID := uuid.New()
	relationshipID := uuid.New()
	repo := NewTripleMentionRepository(db)
	if err := repo.UpdateStatus(context.Background(), mention.ID, "aligned", &headID, &tailID, &relationshipID); err != nil {
		t.Fatalf("update triple mention: %v", err)
	}

	var stored model.TripleMention
	if err := db.First(&stored, "id = ?", mention.ID).Error; err != nil {
		t.Fatalf("load triple mention: %v", err)
	}
	if stored.RelationshipID == nil || *stored.RelationshipID != relationshipID {
		t.Fatalf("relationship_id = %v, want %s", stored.RelationshipID, relationshipID)
	}
}
