package model

import (
	"time"

	"github.com/google/uuid"
)

// TypeDefinition represents a multilingual type label definition for entity categories.
type TypeDefinition struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	DatasetID uuid.UUID `gorm:"type:uuid;not null;index" json:"dataset_id"`

	TypeKey string  `gorm:"type:varchar(100);not null" json:"type_key"` // Original English type key (for example, "Person")
	LabelZh *string `gorm:"type:varchar(100)" json:"label_zh"`          // Chinese label value
	LabelEn *string `gorm:"type:varchar(100)" json:"label_en"`          // English label (for example, "Person")

	StyleConfig map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"style_config"` // Optional styling such as color and icon

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName specifies the table name for TypeDefinition.
func (TypeDefinition) TableName() string {
	return "kb_type_definitions"
}

// TypeLabel represents the multilingual label structure for API responses.
type TypeLabel struct {
	ZhHans string `json:"zh-Hans"`
	EnUS   string `json:"en-US"`
}

// ToTypeLabel converts TypeDefinition to TypeLabel for API responses.
func (t *TypeDefinition) ToTypeLabel() TypeLabel {
	zhLabel := t.TypeKey // Default to type key if no translation
	enLabel := t.TypeKey

	if t.LabelZh != nil && *t.LabelZh != "" {
		zhLabel = *t.LabelZh
	}
	if t.LabelEn != nil && *t.LabelEn != "" {
		enLabel = *t.LabelEn
	}

	return TypeLabel{
		ZhHans: zhLabel,
		EnUS:   enLabel,
	}
}
