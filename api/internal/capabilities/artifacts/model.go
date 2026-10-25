// Package artifacts keeps what agents deliver: versioned documents, files,
// and images that outlive the conversation that produced them (ADR 0009).
// It owns ownership and visibility; products adapt their callers to it.
package artifacts

import (
	"time"

	"github.com/google/uuid"
)

// Kind is what an artifact holds.
type Kind string

const (
	KindDocument Kind = "document"
	KindFile     Kind = "file"
	KindImage    Kind = "image"
	KindTable    Kind = "table"
)

// OwnerKind is who an artifact belongs to.
type OwnerKind string

const (
	OwnerAccount     OwnerKind = "account"
	OwnerAPIEndUser  OwnerKind = "api_end_user"
	OwnerWorkspace   OwnerKind = "workspace"
	OwnerApplication OwnerKind = "application"
)

func (k OwnerKind) valid() bool {
	switch k {
	case OwnerAccount, OwnerAPIEndUser, OwnerWorkspace, OwnerApplication:
		return true
	}
	return false
}

// Visibility is who besides the owner may read an artifact.
type Visibility string

const (
	// VisibilityOwner limits an artifact to its owner and, unless privacy
	// mode is on, the application's operators.
	VisibilityOwner Visibility = "owner"
	// VisibilityTeam shares an artifact with the workspace's members.
	VisibilityTeam Visibility = "team"
	// VisibilityApplication offers an artifact to every user of the
	// application, including end users.
	VisibilityApplication Visibility = "application"
)

// SourceKind says what produced a version.
type SourceKind string

const (
	SourceRun        SourceKind = "run"
	SourceTaskReport SourceKind = "task_report"
	SourceManual     SourceKind = "manual"
)

// Artifact is one deliverable. Its content lives in versions.
type Artifact struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID  uuid.UUID  `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID     uuid.UUID  `gorm:"type:uuid;not null" json:"workspace_id"`
	ApplicationType string     `gorm:"type:varchar(32);not null" json:"application_type"`
	ApplicationID   uuid.UUID  `gorm:"type:uuid;not null" json:"application_id"`
	OwnerKind       OwnerKind  `gorm:"type:varchar(32);not null" json:"owner_kind"`
	OwnerID         uuid.UUID  `gorm:"type:uuid;not null" json:"owner_id"`
	Visibility      Visibility `gorm:"type:varchar(16);not null" json:"visibility"`
	Kind            Kind       `gorm:"type:varchar(16);not null" json:"kind"`
	Title           string     `gorm:"type:varchar(255);not null" json:"title"`
	SourceTaskID    *uuid.UUID `gorm:"type:uuid" json:"source_task_id,omitempty"`
	// SourceConversationID is the conversation that first produced the
	// artifact; a file of the same name it produces again adds a version.
	SourceConversationID *uuid.UUID `gorm:"type:uuid" json:"source_conversation_id,omitempty"`
	CurrentVersion       int        `gorm:"not null" json:"current_version"`
	CreatedByKind        string     `gorm:"type:varchar(32);not null" json:"-"`
	CreatedByID          string     `gorm:"type:varchar(64);not null" json:"-"`
	CreatedAt            time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt            time.Time  `gorm:"not null" json:"updated_at"`
	// ArchivedAt hides an artifact from the default listing. It stays
	// readable, and publishing to it again restores it.
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	// SharingDisabledAt stops every link out of the application: share
	// links and the download links channels were sent.
	SharingDisabledAt *time.Time `json:"sharing_disabled_at,omitempty"`
	DeletedAt         *time.Time `json:"-"`
}

// TableName binds artifacts to their table.
func (Artifact) TableName() string { return "artifacts" }

// Version is one immutable revision of an artifact. A document keeps its
// Markdown in Content; any other kind keeps its bytes in a file the version
// owns, independent of the run that produced them.
type Version struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ArtifactID     uuid.UUID  `gorm:"type:uuid;not null" json:"artifact_id"`
	Number         int        `gorm:"not null" json:"number"`
	Content        *string    `gorm:"type:text" json:"-"`
	FileID         *string    `gorm:"type:varchar(64)" json:"-"`
	FileName       string     `gorm:"type:varchar(255);not null" json:"file_name,omitempty"`
	MimeType       string     `gorm:"type:varchar(128);not null" json:"mime_type"`
	Size           int64      `gorm:"not null" json:"size"`
	Checksum       string     `gorm:"type:varchar(64);not null" json:"-"`
	Note           string     `gorm:"type:text;not null" json:"note,omitempty"`
	SourceKind     SourceKind `gorm:"type:varchar(32);not null" json:"source_kind"`
	SourceTaskID   *uuid.UUID `gorm:"type:uuid" json:"source_task_id,omitempty"`
	SourceRunID    *uuid.UUID `gorm:"type:uuid" json:"source_run_id,omitempty"`
	SourceReportID *uuid.UUID `gorm:"type:uuid" json:"source_report_id,omitempty"`
	CreatedByKind  string     `gorm:"type:varchar(32);not null" json:"-"`
	CreatedByID    string     `gorm:"type:varchar(64);not null" json:"-"`
	CreatedAt      time.Time  `gorm:"not null" json:"created_at"`
}

// TableName binds versions to their table.
func (Version) TableName() string { return "artifact_versions" }

// SourceFile records a file a run produced that became an artifact, so a
// link to that file elsewhere, such as a channel download, can follow the
// artifact's archiving and sharing.
type SourceFile struct {
	ArtifactID uuid.UUID `gorm:"type:uuid;primaryKey"`
	FileID     string    `gorm:"type:varchar(64);primaryKey"`
	CreatedAt  time.Time `gorm:"not null"`
}

func (SourceFile) TableName() string { return "artifact_source_files" }
