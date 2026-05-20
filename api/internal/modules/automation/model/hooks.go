package model

import (
	"time"

	"github.com/google/uuid"
)

func ensureModelID(id *string) {
	if *id == "" {
		*id = uuid.New().String()
	}
}

func ensureModelTimestamps(createdAt, updatedAt *time.Time) {
	now := time.Now()
	if createdAt.IsZero() {
		*createdAt = now
	}
	if updatedAt.IsZero() {
		*updatedAt = now
	}
}

func touchModelUpdatedAt(updatedAt *time.Time) {
	*updatedAt = time.Now()
}
