package model

import "time"

// FileObjectDeletion records an object whose file record is already gone but whose content
// could not be removed from storage yet.
//
// The row is written inside the same transaction that deletes the file record, so the
// object can never become untracked: either both are recorded or neither is. Its presence
// alone means "not removed yet", which keeps the table free of a status column.
type FileObjectDeletion struct {
	ID            string    `gorm:"column:id;type:uuid;primaryKey"`
	ObjectKey     string    `gorm:"column:object_key;type:varchar(1024);not null"`
	AttemptCount  int       `gorm:"column:attempt_count;not null"`
	NextAttemptAt time.Time `gorm:"column:next_attempt_at;not null"`
	LastError     string    `gorm:"column:last_error;type:text;not null"`
	CreatedAt     time.Time `gorm:"column:created_at;not null"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null"`
}

func (FileObjectDeletion) TableName() string {
	return "file_object_deletions"
}

// NewFileObjectDeletion builds a due-now pending removal for an object key.
func NewFileObjectDeletion(objectKey string, now time.Time) *FileObjectDeletion {
	return &FileObjectDeletion{
		ObjectKey:     objectKey,
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}
