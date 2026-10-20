package socialops

import (
	"github.com/google/uuid"
	"time"
)

type Scope struct {
	OrganizationID uuid.UUID `json:"organization_id" gorm:"type:uuid;not null"`
	WorkspaceID    uuid.UUID `json:"workspace_id" gorm:"type:uuid;not null"`
	OwnerID        uuid.UUID `json:"owner_id" gorm:"type:uuid;not null"`
	EmployeeID     uuid.UUID `json:"employee_id" gorm:"type:uuid;not null"`
}
type Artifact struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	Scope     `gorm:"embedded"`
	Digest    string    `json:"digest" gorm:"type:varchar(64);not null"`
	MIME      string    `json:"mime" gorm:"type:varchar(128);not null"`
	Size      int64     `json:"size" gorm:"not null"`
	CreatedAt time.Time `json:"created_at" gorm:"not null"`
}

func (Artifact) TableName() string { return "social_operation_artifacts" }

// ImageInspection is server-created evidence that a vision model inspected the
// exact immutable image bytes under one canonical configuration revision.
// Model-facing record tools cannot create or update this table.
type ImageInspection struct {
	ID                    uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	Scope                 `gorm:"embedded"`
	ScopeKey              string    `json:"-" gorm:"type:varchar(64);not null;uniqueIndex:scope_image_inspection,priority:1"`
	ArtifactID            uuid.UUID `json:"artifact_id" gorm:"type:uuid;not null;uniqueIndex:scope_image_inspection,priority:2"`
	ArtifactDigest        string    `json:"artifact_digest" gorm:"type:varchar(64);not null"`
	ConfigurationRevision int64     `json:"configuration_revision" gorm:"not null;uniqueIndex:scope_image_inspection,priority:3"`
	ReviewArtifactID      uuid.UUID `json:"review_artifact_id" gorm:"type:uuid;not null"`
	Approved              bool      `json:"approved" gorm:"not null"`
	ReviewedAt            time.Time `json:"reviewed_at" gorm:"not null"`
}

func (ImageInspection) TableName() string { return "social_operation_image_inspections" }

type Record struct {
	ScopeKey   string    `json:"-" gorm:"type:varchar(64);not null;uniqueIndex:scope_record,priority:1"`
	ID         uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	Scope      `gorm:"embedded"`
	Kind       string    `json:"kind" gorm:"type:varchar(32);not null;uniqueIndex:scope_record,priority:5"`
	Key        string    `json:"key" gorm:"column:record_key;type:varchar(128);not null;uniqueIndex:scope_record,priority:6"`
	Revision   int64     `json:"revision" gorm:"not null"`
	ArtifactID uuid.UUID `json:"artifact_id" gorm:"type:uuid;not null"`
	UpdatedAt  time.Time `json:"updated_at" gorm:"not null"`
}

func (Record) TableName() string { return "social_operation_records" }

type Publication struct {
	ArchiveTimezone       string     `json:"archive_timezone" gorm:"type:varchar(128);not null;default:''"`
	ConfigurationRevision int64      `json:"configuration_revision" gorm:"not null;default:0"`
	ReservedAt            *time.Time `json:"reserved_at,omitempty"`
	ContentDigest         string     `json:"-" gorm:"type:varchar(64);not null;default:''"`
	PublicationAction     string     `json:"publication_action" gorm:"type:varchar(16);not null;default:''"`
	AccountID             string     `json:"account_id" gorm:"type:varchar(255);not null;uniqueIndex:scope_publication,priority:6"`
	QuotedID              string     `json:"quoted_id,omitempty" gorm:"type:varchar(255);not null"`
	RepostedID            string     `json:"reposted_id,omitempty" gorm:"type:varchar(255);not null"`
	ParentPublicationID   *uuid.UUID `json:"parent_publication_id,omitempty" gorm:"type:uuid"`
	MediaJSON             string     `json:"-" gorm:"type:text;not null;default:'[]'"`
	ScopeKey              string     `json:"-" gorm:"type:varchar(64);not null;uniqueIndex:scope_publication,priority:1"`
	ID                    uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	Scope                 `gorm:"embedded"`
	ConnectionID          uuid.UUID  `json:"connection_id" gorm:"type:uuid;not null;uniqueIndex:scope_publication,priority:5"`
	ContentKey            string     `json:"content_key" gorm:"type:varchar(128);not null;uniqueIndex:scope_publication,priority:7"`
	Platform              string     `json:"platform" gorm:"type:varchar(32);not null"`
	DraftArtifactID       uuid.UUID  `json:"draft_artifact_id" gorm:"type:uuid;not null"`
	ReviewedDigest        string     `json:"reviewed_digest" gorm:"type:varchar(64);not null"`
	ActualArtifactID      *uuid.UUID `json:"actual_artifact_id,omitempty" gorm:"type:uuid"`
	State                 string     `json:"state" gorm:"type:varchar(32);not null"`
	Revision              int64      `json:"revision" gorm:"not null"`
	RemoteID              string     `json:"remote_id" gorm:"type:varchar(255);not null"`
	RemoteURL             string     `json:"remote_url" gorm:"type:text;not null"`
	ParentRemoteID        string     `json:"parent_remote_id" gorm:"type:varchar(255);not null"`
	PublishedAt           *time.Time `json:"published_at,omitempty"`
	VerifiedAt            *time.Time `json:"verified_at,omitempty"`
	ArchiveDate           string     `json:"archive_date" gorm:"type:varchar(10);not null"`
	CreatedAt             time.Time  `json:"created_at" gorm:"not null"`
	UpdatedAt             time.Time  `json:"updated_at" gorm:"not null"`
}

func (Publication) TableName() string { return "social_operation_publications" }

type Metric struct {
	ID            uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	PublicationID uuid.UUID `json:"publication_id" gorm:"type:uuid;not null;uniqueIndex"`
	ObservedAt    time.Time `json:"observed_at" gorm:"not null"`
	AgeSeconds    int64     `json:"age_seconds" gorm:"not null"`
	Late          bool      `json:"late" gorm:"not null"`
	ValuesJSON    string    `json:"values_json" gorm:"type:text;not null"`
}

func (Metric) TableName() string { return "social_operation_metrics" }
