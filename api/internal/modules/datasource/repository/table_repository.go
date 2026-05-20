package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/zgiai/ginext/internal/modules/datasource/model"
	"gorm.io/gorm"

	"github.com/google/uuid"
)

// TableRepository defines the interface for table metadata repository
type TableRepository interface {
	Create(ctx context.Context, table *model.Table) error
	FindByID(ctx context.Context, id string) (*model.Table, error)
	FindByOrganizationAndName(ctx context.Context, organizationID, name string) (*model.Table, error)
	FindByDataSourceAndTableName(ctx context.Context, dataSourceID, tableName string) (*model.Table, error)
	FindByDataSourceAndName(ctx context.Context, dataSourceID, name string) (*model.Table, error)
	ListByDataSource(ctx context.Context, dataSourceID string) ([]*model.Table, error)
	ListByOrganization(ctx context.Context, organizationID string) ([]*model.Table, error)
	Update(ctx context.Context, table *model.Table) error
	Delete(ctx context.Context, id string) error
}

// PostgresTableRepository implements TableRepository using postgres
type PostgresTableRepository struct {
	db *gorm.DB
}

// NewPostgresTableRepository creates a new PostgresTableRepository
func NewPostgresTableRepository(db *gorm.DB) TableRepository {
	return &PostgresTableRepository{db: db}
}

// Create creates a new table metadata record
func (r *PostgresTableRepository) Create(ctx context.Context, table *model.Table) error {
	if table.ID == "" {
		table.ID = uuid.New().String()
	}

	if table.CreatedAt.IsZero() {
		table.CreatedAt = time.Now()
	}

	table.UpdatedAt = time.Now()

	query := `
		INSERT INTO data_source_tables (
			id, organization_id, data_source_id, name, table_id, table_name, 
			description, created_by, updated_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	err := r.db.WithContext(ctx).Exec(
		query,
		table.ID,
		table.OrganizationID,
		table.DataSourceID,
		table.Name,
		table.TableID,
		table.PhysicalTableName,
		table.Description,
		table.CreatedBy,
		table.UpdatedBy,
		table.CreatedAt,
		table.UpdatedAt,
	).Error
	return err
}

// FindByID finds a table metadata by ID
func (r *PostgresTableRepository) FindByID(ctx context.Context, id string) (*model.Table, error) {
	query := `
		SELECT 
			id, organization_id, data_source_id, name, table_id, table_name,
			description, created_by, updated_by, created_at, updated_at
		FROM data_source_tables
		WHERE id = ?
	`
	row := r.db.WithContext(ctx).Raw(query, id).Row()

	var table model.Table
	err := row.Scan(
		&table.ID,
		&table.OrganizationID,
		&table.DataSourceID,
		&table.Name,
		&table.TableID,
		&table.PhysicalTableName,
		&table.Description,
		&table.CreatedBy,
		&table.UpdatedBy,
		&table.CreatedAt,
		&table.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &table, nil
}

// FindByOrganizationAndName finds a table metadata by organization ID and name
func (r *PostgresTableRepository) FindByOrganizationAndName(ctx context.Context, organizationID, name string) (*model.Table, error) {
	var table model.Table
	err := r.db.WithContext(ctx).Where("organization_id = ? AND name = ?", organizationID, name).First(&table).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &table, nil
}

// FindByDataSourceAndTableName finds a table metadata by data source ID and table name
func (r *PostgresTableRepository) FindByDataSourceAndTableName(ctx context.Context, dataSourceID, tableName string) (*model.Table, error) {
	var table model.Table
	err := r.db.WithContext(ctx).Where("data_source_id = ? AND table_name = ?", dataSourceID, tableName).First(&table).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &table, nil
}

// FindByDataSourceAndName finds a table metadata by data source ID and user-defined name
func (r *PostgresTableRepository) FindByDataSourceAndName(ctx context.Context, dataSourceID, name string) (*model.Table, error) {
	var table model.Table
	err := r.db.WithContext(ctx).Where("data_source_id = ? AND name = ?", dataSourceID, name).First(&table).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &table, nil
}

// ListByDataSource lists all table metadata for a data source
func (r *PostgresTableRepository) ListByDataSource(ctx context.Context, dataSourceID string) ([]*model.Table, error) {
	query := `
		SELECT 
			id, organization_id, data_source_id, name, table_id, table_name,
			description, created_by, updated_by, created_at, updated_at
		FROM data_source_tables
		WHERE data_source_id = ?
		ORDER BY created_at DESC
	`
	rows, err := r.db.WithContext(ctx).Raw(query, dataSourceID).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []*model.Table
	for rows.Next() {
		var table model.Table
		err := rows.Scan(
			&table.ID,
			&table.OrganizationID,
			&table.DataSourceID,
			&table.Name,
			&table.TableID,
			&table.PhysicalTableName,
			&table.Description,
			&table.CreatedBy,
			&table.UpdatedBy,
			&table.CreatedAt,
			&table.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		tables = append(tables, &table)
	}

	return tables, rows.Err()
}

// ListByOrganization lists all table metadata for an organization
func (r *PostgresTableRepository) ListByOrganization(ctx context.Context, organizationID string) ([]*model.Table, error) {
	query := `
		SELECT 
			id, organization_id, data_source_id, name, table_id, table_name,
			description, created_by, updated_by, created_at, updated_at
		FROM data_source_tables
		WHERE organization_id = ?
		ORDER BY created_at DESC
	`
	rows, err := r.db.WithContext(ctx).Raw(query, organizationID).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []*model.Table
	for rows.Next() {
		var table model.Table
		err := rows.Scan(
			&table.ID,
			&table.OrganizationID,
			&table.DataSourceID,
			&table.Name,
			&table.TableID,
			&table.PhysicalTableName,
			&table.Description,
			&table.CreatedBy,
			&table.UpdatedBy,
			&table.CreatedAt,
			&table.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		tables = append(tables, &table)
	}

	return tables, rows.Err()
}

// Update updates a table metadata record
func (r *PostgresTableRepository) Update(ctx context.Context, table *model.Table) error {
	table.UpdatedAt = time.Now()

	query := `
		UPDATE data_source_tables
		SET name = ?, description = ?, updated_by = ?, updated_at = ?
		WHERE id = ?
	`
	err := r.db.WithContext(ctx).Exec(
		query,
		table.Name,
		table.Description,
		table.UpdatedBy,
		table.UpdatedAt,
		table.ID,
	).Error
	return err
}

// Delete deletes a table metadata record
func (r *PostgresTableRepository) Delete(ctx context.Context, id string) error {
	query := `
		DELETE FROM data_source_tables
		WHERE id = ?
	`
	err := r.db.WithContext(ctx).Exec(query, id).Error
	return err
}
