package worker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type cleanupTestSegment struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	DocumentID uuid.UUID `gorm:"type:uuid;index"`
}

func (cleanupTestSegment) TableName() string { return "document_segments" }

type cleanupTestRef struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	DatasetDocumentID uuid.UUID `gorm:"type:uuid;index"`
	RetrievalEnabled  bool
	DeletedAt         gorm.DeletedAt
}

func (cleanupTestRef) TableName() string { return "data_library_knowledge_base_asset_refs" }

type cleanupTestDocument struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	Enabled bool
}

func (cleanupTestDocument) TableName() string { return "documents" }

type cleanupTestEntity struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	KBID              uuid.UUID `gorm:"type:uuid;column:kb_id"`
	TenantID          uuid.UUID `gorm:"type:uuid"`
	Name              string
	CanonicalName     string
	Type              string
	SourceCount       int
	ActiveSourceCount int
	EmbeddingID       string
	GraphNodeID       string
	GraphState        string
	VectorState       string
	SyncErrorLog      string
	IsDeleted         bool
	DeletedAt         *time.Time
	UpdatedAt         time.Time
}

func (cleanupTestEntity) TableName() string { return "kb_entities" }

type cleanupTestRelationship struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	KBID         uuid.UUID `gorm:"type:uuid;column:kb_id"`
	TenantID     uuid.UUID `gorm:"type:uuid"`
	HeadEntityID uuid.UUID `gorm:"type:uuid"`
	TailEntityID uuid.UUID `gorm:"type:uuid"`
	RelationType string
	Weight       int
	ActiveWeight int
	GraphState   string
	IsDeleted    bool
	DeletedAt    *time.Time
	UpdatedAt    time.Time
}

func (cleanupTestRelationship) TableName() string { return "kb_relationships" }

type cleanupTestEntityMention struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	KBID        uuid.UUID `gorm:"type:uuid;column:kb_id"`
	TenantID    uuid.UUID `gorm:"type:uuid"`
	SegmentID   uuid.UUID `gorm:"type:uuid"`
	SourceRefID *uuid.UUID
	DocumentID  *uuid.UUID
	EntityID    *uuid.UUID
	RawName     string
	Status      string
	IsDeleted   bool
	DeletedAt   *time.Time
}

func (cleanupTestEntityMention) TableName() string { return "kb_entity_mentions" }

type cleanupTestTripleMention struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	KBID           uuid.UUID `gorm:"type:uuid;column:kb_id"`
	TenantID       uuid.UUID `gorm:"type:uuid"`
	SegmentID      uuid.UUID `gorm:"type:uuid"`
	SourceRefID    *uuid.UUID
	DocumentID     *uuid.UUID
	RelationshipID *uuid.UUID
	HeadEntityID   *uuid.UUID
	TailEntityID   *uuid.UUID
	RawSubject     string
	RawPredicate   string
	RawObject      string
	Status         string
	IsDeleted      bool
	DeletedAt      *time.Time
}

func (cleanupTestTripleMention) TableName() string { return "kb_triple_mentions" }

func TestCleanupGarbageCollectionUsesRemainingEvidence(t *testing.T) {
	tests := []struct {
		name                   string
		remainingEvidence      int
		remainingRelationships int
		deleteRelationship     bool
		deleteEntity           bool
	}{
		{name: "shared fact", remainingEvidence: 1, deleteRelationship: false, deleteEntity: false},
		{name: "zero evidence relationship", remainingEvidence: 0, remainingRelationships: 1, deleteRelationship: true, deleteEntity: false},
		{name: "orphan entity", remainingEvidence: 0, remainingRelationships: 0, deleteRelationship: true, deleteEntity: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := planEvidenceGarbageCollection(tt.remainingEvidence, tt.remainingRelationships)
			if plan.DeleteRelationship != tt.deleteRelationship || plan.DeleteEntity != tt.deleteEntity {
				t.Fatalf("plan=%#v", plan)
			}
		})
	}
}

func TestCleanupDocumentEvidenceOnlyDeletesOwnedProjections(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:cleanup-%s?mode=memory&cache=shared", uuid.NewString())), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&cleanupTestSegment{},
		&cleanupTestRef{},
		&cleanupTestDocument{},
		&cleanupTestEntity{},
		&cleanupTestRelationship{},
		&cleanupTestEntityMention{},
		&cleanupTestTripleMention{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	ctx := context.Background()
	kbID := uuid.New()
	tenantID := uuid.New()
	documentID := uuid.New()
	otherDocumentID := uuid.New()
	ownedHeadID := uuid.New()
	ownedTailID := uuid.New()
	otherEntityID := uuid.New()
	legacyOrphanID := uuid.New()
	ownedRelationshipID := uuid.New()

	documents := []cleanupTestDocument{
		{ID: documentID, Enabled: true},
		{ID: otherDocumentID, Enabled: true},
	}
	if err := db.Create(&documents).Error; err != nil {
		t.Fatalf("create documents: %v", err)
	}
	entities := []*cleanupTestEntity{
		{ID: ownedHeadID, KBID: kbID, TenantID: tenantID, Name: "owned-head", CanonicalName: "owned-head", Type: "test", SourceCount: 1, GraphState: "synced", VectorState: "synced"},
		{ID: ownedTailID, KBID: kbID, TenantID: tenantID, Name: "owned-tail", CanonicalName: "owned-tail", Type: "test", SourceCount: 1, GraphState: "synced", VectorState: "synced"},
		{ID: otherEntityID, KBID: kbID, TenantID: tenantID, Name: "other", CanonicalName: "other", Type: "test", SourceCount: 1, GraphState: "synced", VectorState: "synced"},
		{ID: legacyOrphanID, KBID: kbID, TenantID: tenantID, Name: "legacy", CanonicalName: "legacy", Type: "test", SourceCount: 0, GraphState: "synced", VectorState: "synced"},
	}
	if err := db.Create(&entities).Error; err != nil {
		t.Fatalf("create entities: %v", err)
	}
	relationship := &cleanupTestRelationship{
		ID: ownedRelationshipID, KBID: kbID, TenantID: tenantID,
		HeadEntityID: ownedHeadID, TailEntityID: ownedTailID,
		RelationType: "owns", Weight: 1, GraphState: "synced",
	}
	if err := db.Create(relationship).Error; err != nil {
		t.Fatalf("create relationship: %v", err)
	}
	entityMentions := []*cleanupTestEntityMention{
		{ID: uuid.New(), KBID: kbID, TenantID: tenantID, SegmentID: uuid.New(), DocumentID: &documentID, EntityID: &ownedHeadID, RawName: "owned-head", Status: "synced"},
		{ID: uuid.New(), KBID: kbID, TenantID: tenantID, SegmentID: uuid.New(), DocumentID: &documentID, EntityID: &ownedTailID, RawName: "owned-tail", Status: "synced"},
		{ID: uuid.New(), KBID: kbID, TenantID: tenantID, SegmentID: uuid.New(), DocumentID: &otherDocumentID, EntityID: &otherEntityID, RawName: "other", Status: "synced"},
	}
	if err := db.Create(&entityMentions).Error; err != nil {
		t.Fatalf("create entity mentions: %v", err)
	}
	tripleMention := &cleanupTestTripleMention{
		ID: uuid.New(), KBID: kbID, TenantID: tenantID, SegmentID: uuid.New(), DocumentID: &documentID,
		RelationshipID: &ownedRelationshipID, HeadEntityID: &ownedHeadID, TailEntityID: &ownedTailID,
		RawSubject: "owned-head", RawPredicate: "owns", RawObject: "owned-tail", Status: "synced",
	}
	if err := db.Create(tripleMention).Error; err != nil {
		t.Fatalf("create triple mention: %v", err)
	}

	cleanup, err := cleanupDocumentEvidence(ctx, db, kbID, documentID)
	if err != nil {
		t.Fatalf("cleanup document evidence: %v", err)
	}
	if len(cleanup.Relationships) != 1 || cleanup.Relationships[0].ID != ownedRelationshipID {
		t.Fatalf("cleanup relationships=%v, want only %s", cleanup.Relationships, ownedRelationshipID)
	}
	if len(cleanup.Entities) != 2 {
		t.Fatalf("cleanup entities=%d, want 2", len(cleanup.Entities))
	}

	var otherEntity graphmodel.Entity
	if err := db.First(&otherEntity, "id = ?", otherEntityID).Error; err != nil {
		t.Fatalf("load other entity: %v", err)
	}
	if otherEntity.IsDeleted || otherEntity.GraphState != "synced" || otherEntity.SourceCount != 1 {
		t.Fatalf("other document entity was changed: %#v", otherEntity)
	}
	var legacyOrphan graphmodel.Entity
	if err := db.First(&legacyOrphan, "id = ?", legacyOrphanID).Error; err != nil {
		t.Fatalf("load legacy orphan: %v", err)
	}
	if legacyOrphan.IsDeleted || legacyOrphan.GraphState != "synced" {
		t.Fatalf("unowned legacy orphan was changed: %#v", legacyOrphan)
	}

	svc := &graphflow.Service{
		EntityRepo:       repository.NewEntityRepository(db),
		RelationshipRepo: repository.NewRelationshipRepository(db),
	}
	if err := cleanupDocumentProjections(ctx, svc, cleanup); err != nil {
		t.Fatalf("cleanup document projections: %v", err)
	}
	var cleanedEntity graphmodel.Entity
	if err := db.First(&cleanedEntity, "id = ?", ownedHeadID).Error; err != nil {
		t.Fatalf("load cleaned entity: %v", err)
	}
	if cleanedEntity.GraphState != "deleted" || cleanedEntity.VectorState != "deleted" {
		t.Fatalf("cleaned entity states=(%s,%s), want deleted", cleanedEntity.GraphState, cleanedEntity.VectorState)
	}
}
