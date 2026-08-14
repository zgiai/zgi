package testutil

import (
	"time"

	"github.com/google/uuid"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

var (
	OrganizationID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	WorkspaceID    = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	DatasetID      = uuid.MustParse("00000000-0000-0000-0000-000000000003")
	DocumentID     = uuid.MustParse("00000000-0000-0000-0000-000000000004")
	SegmentID      = uuid.MustParse("00000000-0000-0000-0000-000000000005")
	HeadEntityID   = uuid.MustParse("00000000-0000-0000-0000-000000000006")
	TailEntityID   = uuid.MustParse("00000000-0000-0000-0000-000000000007")
	RelationshipID = uuid.MustParse("00000000-0000-0000-0000-000000000008")
)

type GraphFixture struct {
	Dataset       *datasetmodel.Dataset
	Document      *datasetmodel.Document
	Segment       *datasetmodel.DocumentSegment
	HeadEntity    *graphmodel.Entity
	TailEntity    *graphmodel.Entity
	Relationship  *graphmodel.Relationship
	EntityMention *graphmodel.EntityMention
	TripleMention *graphmodel.TripleMention
}

func NewGraphFixture() GraphFixture {
	now := time.Date(2026, time.July, 22, 0, 0, 0, 0, time.UTC)
	embeddingProvider := "embedding-provider"
	embeddingModel := "embedding-model"
	entityProvider := "text-model-provider"
	entityModel := "text-model"

	dataset := &datasetmodel.Dataset{
		ID:                     DatasetID.String(),
		OrganizationID:         OrganizationID.String(),
		WorkspaceID:            WorkspaceID.String(),
		Name:                   "Graph Fixture Dataset",
		EnableGraphFlow:        true,
		CreatedBy:              OrganizationID.String(),
		CreatedAt:              now,
		UpdatedAt:              now,
		EmbeddingModelProvider: &embeddingProvider,
		EmbeddingModel:         &embeddingModel,
		EntityModelProvider:    &entityProvider,
		EntityModel:            &entityModel,
	}

	document := &datasetmodel.Document{
		ID:             DocumentID.String(),
		OrganizationID: OrganizationID.String(),
		DatasetID:      DatasetID.String(),
		Position:       1,
		DataSourceType: "upload_file",
		Batch:          "fixture-batch",
		Name:           "Graph Fixture Document",
		CreatedFrom:    "test",
		CreatedBy:      OrganizationID.String(),
		CreatedAt:      now,
		UpdatedAt:      now,
		IndexingStatus: datasetmodel.DocumentStatusCompleted,
		Enabled:        true,
		DocForm:        "text_model",
	}

	segment := &datasetmodel.DocumentSegment{
		ID:                  SegmentID.String(),
		OrganizationID:      OrganizationID.String(),
		DatasetID:           DatasetID.String(),
		DocumentID:          DocumentID.String(),
		Position:            1,
		Content:             "Acme Corporation operates the Example Platform.",
		WordCount:           6,
		Tokens:              8,
		Enabled:             true,
		Status:              datasetmodel.SegmentStatusCompleted,
		GraphIndexingStatus: "completed",
		CreatedBy:           OrganizationID.String(),
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	headEntity := &graphmodel.Entity{
		ID:            HeadEntityID,
		KBID:          DatasetID,
		TenantID:      WorkspaceID,
		Name:          "Acme Corporation",
		CanonicalName: "acme corporation",
		Type:          "Organization",
		SourceCount:   1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	tailEntity := &graphmodel.Entity{
		ID:            TailEntityID,
		KBID:          DatasetID,
		TenantID:      WorkspaceID,
		Name:          "Example Platform",
		CanonicalName: "example platform",
		Type:          "Product",
		SourceCount:   1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	relationship := &graphmodel.Relationship{
		ID:           RelationshipID,
		KBID:         DatasetID,
		TenantID:     WorkspaceID,
		HeadEntityID: HeadEntityID,
		TailEntityID: TailEntityID,
		RelationType: "OPERATES",
		Weight:       1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	entityMention := &graphmodel.EntityMention{
		ID:         uuid.MustParse("00000000-0000-0000-0000-000000000009"),
		KBID:       DatasetID,
		TenantID:   WorkspaceID,
		SegmentID:  SegmentID,
		RawName:    "Acme Corporation",
		RawType:    "Organization",
		Confidence: 1,
		EntityID:   &headEntity.ID,
		Status:     "resolved",
		CreatedAt:  now,
	}
	tripleMention := &graphmodel.TripleMention{
		ID:           uuid.MustParse("00000000-0000-0000-0000-000000000010"),
		KBID:         DatasetID,
		TenantID:     WorkspaceID,
		SegmentID:    SegmentID,
		RawSubject:   "Acme Corporation",
		RawPredicate: "OPERATES",
		RawObject:    "Example Platform",
		HeadEntityID: &headEntity.ID,
		TailEntityID: &tailEntity.ID,
		Status:       "resolved",
		CreatedAt:    now,
	}

	return GraphFixture{
		Dataset:       dataset,
		Document:      document,
		Segment:       segment,
		HeadEntity:    headEntity,
		TailEntity:    tailEntity,
		Relationship:  relationship,
		EntityMention: entityMention,
		TripleMention: tripleMention,
	}
}
