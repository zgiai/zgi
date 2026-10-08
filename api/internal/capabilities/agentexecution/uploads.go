package agentexecution

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"gorm.io/gorm"
)

// UploadOwner is assigned by authenticated entry adapters before returning an
// upload ID. Uploads precede conversations; a prepared turn separately admits
// explicit file references into exactly one conversation and message.
type UploadOwner struct {
	OrganizationID  string `gorm:"type:uuid;not null"`
	WorkspaceID     string `gorm:"type:uuid;not null"`
	ApplicationType string `gorm:"type:varchar(32);not null"`
	ApplicationID   string `gorm:"type:uuid;not null"`
	Entry           string `gorm:"type:varchar(32);not null"`
	SubjectKind     string `gorm:"type:varchar(32);not null"`
	SubjectID       string `gorm:"type:uuid;not null"`
}

func (o UploadOwner) valid() bool {
	for _, raw := range []string{o.OrganizationID, o.WorkspaceID, o.ApplicationID, o.SubjectID} {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil || id.String() != raw {
			return false
		}
	}
	return (o.ApplicationType == "agent" || o.ApplicationType == "digital_human") &&
		((o.Entry == "webapp" && o.SubjectKind == "account") || (o.Entry == "external-api" && o.SubjectKind == "api_end_user"))
}

// UploadBinding never changes ownership. Existing DigitalHuman uploads retain
// their domain binding; adapters can use that authorizer instead of duplicating
// records here. Historical unbound uploads are not silently adopted.
type UploadBinding struct {
	FileID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	UploadOwner `gorm:"embedded"`
	CreatedAt   time.Time `gorm:"not null"`
}

func (UploadBinding) TableName() string { return "agent_execution_uploads" }

// BindUpload records a new external upload; a duplicate is an error, not an
// ownership update. On failure the entry adapter must remove the new upload.
func BindUpload(ctx context.Context, db *gorm.DB, fileID string, owner UploadOwner) error {
	id, err := uuid.Parse(fileID)
	if db == nil || err != nil || id == uuid.Nil || id.String() != fileID || !owner.valid() {
		return uploadAccessDenied()
	}
	if err := db.WithContext(ctx).Create(&UploadBinding{FileID: id, UploadOwner: owner, CreatedAt: time.Now().UTC()}).Error; err != nil {
		return fmt.Errorf("bind execution upload: %w", err)
	}
	return nil
}

// AuthorizeUpload requires every ownership dimension, independently of the
// credential that uploaded the bytes. Credential rotation does not change it.
func AuthorizeUpload(ctx context.Context, db *gorm.DB, fileID string, owner UploadOwner) error {
	id, err := uuid.Parse(fileID)
	if db == nil || err != nil || id == uuid.Nil || id.String() != fileID || !owner.valid() {
		return uploadAccessDenied()
	}
	var record UploadBinding
	if err := db.WithContext(ctx).Where("file_id = ?", id).Take(&record).Error; err != nil || record.UploadOwner != owner {
		return uploadAccessDenied()
	}
	return nil
}

func uploadAccessDenied() error {
	return apperror.New(catalog.CodePermissionDenied, apperror.WithOperation("agent_execution.upload_owner"))
}
