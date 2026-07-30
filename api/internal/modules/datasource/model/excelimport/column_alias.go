package excelimport

import "time"

type ColumnAlias struct {
	ID               string    `gorm:"type:uuid;primaryKey"`
	OrganizationID   string    `gorm:"type:uuid;not null"`
	DataSourceID     string    `gorm:"type:uuid;not null"`
	TableID          string    `gorm:"type:uuid;not null"`
	TargetColumnID   string    `gorm:"type:varchar(255);not null"`
	TargetColumnName string    `gorm:"type:varchar(255);not null"`
	SourceHeader     string    `gorm:"type:varchar(512);not null"`
	NormalizedHeader string    `gorm:"type:varchar(512);not null"`
	ConfirmedCount   int       `gorm:"not null;default:1"`
	CreatedBy        string    `gorm:"type:varchar(36);not null"`
	UpdatedBy        string    `gorm:"type:varchar(36);not null"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (ColumnAlias) TableName() string {
	return "data_source_import_column_aliases"
}
