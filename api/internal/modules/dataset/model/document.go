package model

import (
	"time"

	"gorm.io/gorm"
)

// Document represents a document in a dataset
type Document struct {
	ID                   string     `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	OrganizationID       string     `gorm:"type:uuid;not null;index" json:"organization_id"`
	DatasetID            string     `gorm:"type:uuid;not null;index" json:"dataset_id"`
	Position             int        `gorm:"not null" json:"position"`
	DataSourceType       string     `gorm:"type:varchar(255);not null" json:"data_source_type"`
	DataSourceInfo       *string    `gorm:"type:text" json:"data_source_info"`
	DatasetProcessRuleID *string    `gorm:"type:uuid" json:"dataset_process_rule_id"`
	Batch                string     `gorm:"type:varchar(255);not null" json:"batch"`
	Name                 string     `gorm:"type:varchar(255);not null" json:"name"`
	CreatedFrom          string     `gorm:"type:varchar(255);not null" json:"created_from"`
	CreatedBy            string     `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAPIRequestID  *string    `gorm:"type:uuid" json:"created_api_request_id"`
	CreatedAt            time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	ProcessingStartedAt  *time.Time `json:"processing_started_at"`
	FileID               *string    `gorm:"type:text" json:"file_id"`
	WordCount            *int       `json:"word_count"`
	ParsingCompletedAt   *time.Time `json:"parsing_completed_at"`
	CleaningCompletedAt  *time.Time `json:"cleaning_completed_at"`
	SplittingCompletedAt *time.Time `json:"splitting_completed_at"`
	Tokens               *int       `json:"tokens"`
	IndexingLatency      *float64   `json:"indexing_latency"`
	CompletedAt          *time.Time `json:"completed_at"`
	IsPaused             bool       `gorm:"default:false" json:"is_paused"`
	PausedBy             *string    `gorm:"type:uuid" json:"paused_by"`
	PausedAt             *time.Time `json:"paused_at"`
	Error                *string    `gorm:"type:text" json:"error"`
	StoppedAt            *time.Time `json:"stopped_at"`
	IndexingStatus       string     `gorm:"type:varchar(255);not null;default:'waiting'" json:"indexing_status"`
	Enabled              bool       `gorm:"not null;default:true" json:"enabled"`
	DisabledAt           *time.Time `json:"disabled_at"`
	DisabledBy           *string    `gorm:"type:uuid" json:"disabled_by"`
	Archived             bool       `gorm:"not null;default:false" json:"archived"`
	ArchivedReason       *string    `gorm:"type:varchar(255)" json:"archived_reason"`
	ArchivedBy           *string    `gorm:"type:uuid" json:"archived_by"`
	ArchivedAt           *time.Time `json:"archived_at"`
	UpdatedAt            time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DocType              *string    `gorm:"type:varchar(40)" json:"doc_type"`
	DocMetadata          JSONMap    `gorm:"type:jsonb" json:"doc_metadata"`
	DocForm              string     `gorm:"type:varchar(255);not null;default:'text_model'" json:"doc_form"`
	DocLanguage          *string    `gorm:"type:varchar(255)" json:"doc_language"`

	// Virtual fields (computed at query time)
	SegmentCount         int     `gorm:"-" json:"segment_count"`
	AverageSegmentLength float64 `gorm:"-" json:"average_segment_length"`
	HitCount             int     `gorm:"-" json:"hit_count"`
	DisplayStatus        string  `gorm:"-" json:"display_status"`
	CompletedSegments    *int    `gorm:"-" json:"completed_segments,omitempty"`
	TotalSegments        *int    `gorm:"-" json:"total_segments,omitempty"`
}

// DocumentSegment represents a segment of a document
type DocumentSegment struct {
	ID                  string     `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	OrganizationID      string     `gorm:"type:uuid;not null;index" json:"organization_id"`
	DatasetID           string     `gorm:"type:uuid;not null;index" json:"dataset_id"`
	DocumentID          string     `gorm:"type:uuid;not null;index" json:"document_id"`
	Position            int        `gorm:"not null" json:"position"`
	Content             string     `gorm:"type:text;not null" json:"content"`
	WordCount           int        `gorm:"not null" json:"word_count"`
	Tokens              int        `gorm:"not null" json:"tokens"`
	Keywords            JSONMap    `gorm:"type:json" json:"keywords"`
	IndexNodeID         *string    `gorm:"type:varchar(255)" json:"index_node_id"`
	IndexNodeHash       *string    `gorm:"type:varchar(255)" json:"index_node_hash"`
	HitCount            int        `gorm:"not null;default:0" json:"hit_count"`
	Enabled             bool       `gorm:"not null;default:true" json:"enabled"`
	DisabledAt          *time.Time `json:"disabled_at"`
	DisabledBy          *string    `gorm:"type:uuid" json:"disabled_by"`
	Status              string     `gorm:"type:varchar(255);not null;default:'waiting'" json:"status"`
	GraphIndexingStatus string     `gorm:"type:varchar(50);default:'pending'" json:"graph_indexing_status"`
	CreatedBy           string     `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt           time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	IndexingAt          *time.Time `json:"indexing_at"`
	CompletedAt         *time.Time `json:"completed_at"`
	Error               *string    `gorm:"type:text" json:"error"`
	StoppedAt           *time.Time `json:"stopped_at"`
	Answer              *string    `gorm:"type:text" json:"answer"`
	UpdatedBy           *string    `gorm:"type:uuid" json:"updated_by"`
	UpdatedAt           time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	IsDeleted bool           `gorm:"not null;default:false" json:"is_deleted"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	Document *Document `gorm:"foreignKey:DocumentID;references:ID" json:"document,omitempty"`
	Dataset  *Dataset  `gorm:"foreignKey:DatasetID;references:ID" json:"dataset,omitempty"`
}

// TableName specifies the table name for Document
func (Document) TableName() string {
	return "documents"
}

// TableName specifies the table name for DocumentSegment
func (DocumentSegment) TableName() string {
	return "document_segments"
}

// BeforeCreate hook for Document
func (d *Document) BeforeCreate(tx *gorm.DB) error {
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate hook for Document
func (d *Document) BeforeUpdate(tx *gorm.DB) error {
	d.UpdatedAt = time.Now()
	return nil
}

// BeforeCreate hook for DocumentSegment
func (ds *DocumentSegment) BeforeCreate(tx *gorm.DB) error {
	if ds.CreatedAt.IsZero() {
		ds.CreatedAt = time.Now()
	}
	if ds.UpdatedAt.IsZero() {
		ds.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate hook for DocumentSegment
func (ds *DocumentSegment) BeforeUpdate(tx *gorm.DB) error {
	ds.UpdatedAt = time.Now()
	return nil
}

// Constants for document indexing status
const (
	DocumentStatusWaiting   = "waiting"
	DocumentStatusParsing   = "parsing"
	DocumentStatusCleaning  = "cleaning"
	DocumentStatusSplitting = "splitting"
	DocumentStatusIndexing  = "indexing"
	DocumentStatusPaused    = "paused"
	DocumentStatusError     = "error"
	DocumentStatusCompleted = "completed"
)

// Constants for document segment status
const (
	SegmentStatusWaiting   = "waiting"
	SegmentStatusIndexing  = "indexing"
	SegmentStatusCompleted = "completed"
	SegmentStatusError     = "error"
	SegmentStatusReSegment = "re_segment"
)

// GetDisplayStatus returns the display status for the document
func (d *Document) GetDisplayStatus() string {
	if d.IsPaused {
		return DocumentStatusPaused
	}
	return d.IndexingStatus
}

// DocumentSegmentQuestion represents a question associated with a document segment
type DocumentSegmentQuestion struct {
	ID             string    `json:"id" gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	OrganizationID string    `json:"organization_id" gorm:"type:uuid;not null;index"`
	DatasetID      string    `json:"dataset_id" gorm:"type:uuid;not null;index"`
	DocumentID     string    `json:"document_id" gorm:"type:uuid;not null;index"`
	SegmentID      string    `json:"segment_id" gorm:"type:uuid;not null;index"`
	Question       string    `json:"question" gorm:"type:text;not null"`
	CreatedBy      string    `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt      time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedBy      *string   `json:"updated_by" gorm:"type:uuid"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP"`

	// Indexing status fields
	Status      string     `json:"status" gorm:"type:varchar(255);not null;default:'waiting'"`
	IndexingAt  *time.Time `json:"indexing_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Error       *string    `json:"error" gorm:"type:text"`
}

// TableName specifies the table name for DocumentSegmentQuestion
func (DocumentSegmentQuestion) TableName() string {
	return "document_segment_questions"
}

// BeforeCreate hook for DocumentSegmentQuestion
func (dsq *DocumentSegmentQuestion) BeforeCreate(tx *gorm.DB) error {
	if dsq.CreatedAt.IsZero() {
		dsq.CreatedAt = time.Now()
	}
	if dsq.UpdatedAt.IsZero() {
		dsq.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate hook for DocumentSegmentQuestion
func (dsq *DocumentSegmentQuestion) BeforeUpdate(tx *gorm.DB) error {
	dsq.UpdatedAt = time.Now()
	return nil
}
