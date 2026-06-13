package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/datasource/model"
	"github.com/zgiai/zgi/api/pkg/sql_base/audit"
	"github.com/zgiai/zgi/api/pkg/sql_base/guard"
	"gorm.io/gorm"

	"github.com/google/uuid"
)

// DataSourceRepository defines the interface for data source repository
type DataSourceRepository interface {
	Create(ctx context.Context, ds *model.DataSource) error
	FindByID(ctx context.Context, id string) (*model.DataSource, error)
	FindByOrganizationAndName(ctx context.Context, organizationID, name string) (*model.DataSource, error)
	ListByOrganization(ctx context.Context, organizationID string) ([]*model.DataSource, error)
	// ListByOrganizationWithPermissionFilter lists data sources with permission filtering
	ListByOrganizationWithPermissionFilter(ctx context.Context, organizationID, accountID string, isAdmin bool, filterWorkspaceIDs []string) ([]*model.DataSource, error)
	Update(ctx context.Context, ds *model.DataSource) error
	UpdateGuardPolicy(ctx context.Context, id string, policy []byte) error
	UpdateStatus(ctx context.Context, id, status string) error
	Delete(ctx context.Context, id string) error
}

// SQLOperationRepository defines the interface for SQL operation repository
type SQLOperationRepository interface {
	Create(ctx context.Context, log *model.DataSourceSQLOperation) error
	Insert(ctx context.Context, records []audit.Record) error
	ListByTableID(ctx context.Context, tableID string, limit, offset int) ([]*model.DataSourceSQLOperation, error)
	ListByOrganizationID(ctx context.Context, organizationID string, limit, offset int) ([]*model.DataSourceSQLOperation, error)
	ListByDataSourceID(ctx context.Context, dataSourceID string, limit, offset int) ([]*model.DataSourceSQLOperation, error)
	CountByDataSourceID(ctx context.Context, dataSourceID string) (int64, error)
	ListByDataSourceIDWithFilters(ctx context.Context, dataSourceID string, filters dto.SQLOperationFilter, limit, offset int) ([]*model.DataSourceSQLOperation, error)
	CountByDataSourceIDWithFilters(ctx context.Context, dataSourceID string, filters dto.SQLOperationFilter) (int64, error)
	ListAuditByWorkspace(ctx context.Context, organizationID, workspaceID string, filters dto.SQLAuditFilter, limit, offset int) ([]*model.DataSourceSQLOperation, error)
	CountAuditByWorkspace(ctx context.Context, organizationID, workspaceID string, filters dto.SQLAuditFilter) (int64, error)
	FindAuditByWorkspaceAndID(ctx context.Context, organizationID, workspaceID, operationID string) (*model.DataSourceSQLOperation, error)
}

// PostgresDataSourceRepository implements DataSourceRepository using postgres
type PostgresDataSourceRepository struct {
	db *gorm.DB
}

// PostgresOperationLogRepository implements OperationLogRepository using postgres
type PostgresSQLOperationRepository struct {
	db *gorm.DB
}

// NewPostgresDataSourceRepository creates a new PostgresDataSourceRepository
func NewPostgresDataSourceRepository(db *gorm.DB) DataSourceRepository {
	return &PostgresDataSourceRepository{db: db}
}

// NewPostgresOperationLogRepository creates a new PostgresOperationLogRepository
func NewPostgresSQLOperationRepository(db *gorm.DB) SQLOperationRepository {
	return &PostgresSQLOperationRepository{db: db}
}

// Create creates a new data source record
func (r *PostgresDataSourceRepository) Create(ctx context.Context, ds *model.DataSource) error {
	if ds.ID == "" {
		ds.ID = uuid.New().String()
	}

	if ds.CreatedAt.IsZero() {
		ds.CreatedAt = time.Now()
	}

	ds.UpdatedAt = time.Now()

	query := `
		INSERT INTO data_sources (id, organization_id, workspace_id, name, schema_name, schema_id, description, permission, status, created_by, updated_by, created_at, updated_at, icon_type, icon, icon_background, guard_policy)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	guardPolicy := []byte(ds.GuardPolicy)
	if len(guardPolicy) == 0 {
		guardPolicy = guard.DefaultPolicyJSON()
	}
	err := r.db.WithContext(ctx).Exec(
		query,
		ds.ID,
		ds.OrganizationID,
		ds.WorkspaceID,
		ds.Name,
		ds.SchemaName,
		ds.SchemaID,
		ds.Description,
		ds.Permission,
		ds.Status,
		ds.CreatedBy,
		ds.UpdatedBy,
		ds.CreatedAt,
		ds.UpdatedAt,
		ds.IconType,
		ds.Icon,
		ds.IconBackground,
		guardPolicy,
	).Error
	return err
}

// FindByID finds a data source by ID
func (r *PostgresDataSourceRepository) FindByID(ctx context.Context, id string) (*model.DataSource, error) {
	var ds model.DataSource
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&ds).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &ds, nil
}

// FindByOrganizationAndName finds a data source by organization ID and name
func (r *PostgresDataSourceRepository) FindByOrganizationAndName(ctx context.Context, organizationID, name string) (*model.DataSource, error) {
	var ds model.DataSource
	err := r.db.WithContext(ctx).Where("organization_id = ? AND name = ?", organizationID, name).First(&ds).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &ds, nil
}

// ListByOrganization lists all data sources for a organization
func (r *PostgresDataSourceRepository) ListByOrganization(ctx context.Context, organizationID string) ([]*model.DataSource, error) {
	query := `
		SELECT id, organization_id, workspace_id, name, schema_name, schema_id, description, permission, status, created_by, updated_by, created_at, updated_at, icon_type, icon, icon_background, guard_policy
		FROM data_sources
		WHERE organization_id = ?
		ORDER BY created_at DESC
	`
	rows, err := r.db.WithContext(ctx).Raw(query, organizationID).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dataSources []*model.DataSource
	for rows.Next() {
		var ds model.DataSource
		err := rows.Scan(
			&ds.ID,
			&ds.OrganizationID,
			&ds.WorkspaceID,
			&ds.Name,
			&ds.SchemaName,
			&ds.SchemaID,
			&ds.Description,
			&ds.Permission,
			&ds.Status,
			&ds.CreatedBy,
			&ds.UpdatedBy,
			&ds.CreatedAt,
			&ds.UpdatedAt,
			&ds.IconType,
			&ds.Icon,
			&ds.IconBackground,
			&ds.GuardPolicy,
		)
		if err != nil {
			return nil, err
		}
		dataSources = append(dataSources, &ds)
	}

	return dataSources, rows.Err()
}

// ListByOrganizationWithPermissionFilter lists data sources with permission filtering
func (r *PostgresDataSourceRepository) ListByOrganizationWithPermissionFilter(ctx context.Context, organizationID, accountID string, isAdmin bool, filterWorkspaceIDs []string) ([]*model.DataSource, error) {
	var dataSources []*model.DataSource

	// If user is admin, show all data sources
	if isAdmin {
		query := `
			SELECT id, organization_id, workspace_id, name, schema_name, schema_id, description, permission, status, created_by, updated_by, created_at, updated_at, icon_type, icon, icon_background, guard_policy
			FROM data_sources
			WHERE organization_id = ?
		`
		var rows *sql.Rows
		var err error

		args := []interface{}{organizationID}
		if filterWorkspaceIDs != nil && len(filterWorkspaceIDs) > 0 {
			query += " AND workspace_id IN (?) ORDER BY created_at DESC"
			rows, err = r.db.WithContext(ctx).Raw(query, append(args, filterWorkspaceIDs)...).Rows()
		} else {
			query += " ORDER BY created_at DESC"
			rows, err = r.db.WithContext(ctx).Raw(query, args...).Rows()
		}
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var ds model.DataSource
			err := rows.Scan(
				&ds.ID,
				&ds.OrganizationID,
				&ds.WorkspaceID,
				&ds.Name,
				&ds.SchemaName,
				&ds.SchemaID,
				&ds.Description,
				&ds.Permission,
				&ds.Status,
				&ds.CreatedBy,
				&ds.UpdatedBy,
				&ds.CreatedAt,
				&ds.UpdatedAt,
				&ds.IconType,
				&ds.Icon,
				&ds.IconBackground,
				&ds.GuardPolicy,
			)
			if err != nil {
				return nil, err
			}
			dataSources = append(dataSources, &ds)
		}

		return dataSources, rows.Err()
	}

	// For non-admin users, enforce membership-based visibility.

	// Build query for non-admin users with membership-based filtering
	var rows *sql.Rows
	var err error
	query := `
		SELECT id, organization_id, workspace_id, name, schema_name, schema_id, description, permission, status, created_by, updated_by, created_at, updated_at, icon_type, icon, icon_background, guard_policy
		FROM data_sources
		WHERE organization_id = ?
	`
	args := []interface{}{organizationID}

	// If filter list explicitly provided
	if filterWorkspaceIDs != nil {
		// Empty allowed list → return empty without hitting DB
		if len(filterWorkspaceIDs) == 0 {
			return []*model.DataSource{}, nil
		}
		// Restrict to allowed workspace IDs
		query += " AND workspace_id IN (?)"
		args = append(args, filterWorkspaceIDs)
	}

	query += `
		AND EXISTS (
			SELECT 1 FROM workspace_members taj
			WHERE taj.account_id = ?
			  AND taj.workspace_id = data_sources.workspace_id
		)
		ORDER BY created_at DESC
	`

	args = append(args, accountID)

	rows, err = r.db.WithContext(ctx).Raw(query, args...).Rows()

	// If we need fine-grained permissions again, restore the original blocks below.

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ds model.DataSource
		err := rows.Scan(
			&ds.ID,
			&ds.OrganizationID,
			&ds.WorkspaceID,
			&ds.Name,
			&ds.SchemaName,
			&ds.SchemaID,
			&ds.Description,
			&ds.Permission,
			&ds.Status,
			&ds.CreatedBy,
			&ds.UpdatedBy,
			&ds.CreatedAt,
			&ds.UpdatedAt,
			&ds.IconType,
			&ds.Icon,
			&ds.IconBackground,
			&ds.GuardPolicy,
		)
		if err != nil {
			return nil, err
		}
		dataSources = append(dataSources, &ds)
	}

	return dataSources, rows.Err()
}

// UpdateStatus updates the status of a data source
func (r *PostgresDataSourceRepository) UpdateStatus(ctx context.Context, id, status string) error {
	query := `
		UPDATE data_sources
		SET status = ?, updated_at = NOW()
		WHERE id = ?
	`
	err := r.db.WithContext(ctx).Exec(query, status, id).Error
	return err
}

// Update updates a data source record
func (r *PostgresDataSourceRepository) Update(ctx context.Context, ds *model.DataSource) error {
	ds.UpdatedAt = time.Now()

	query := `
		UPDATE data_sources 
		SET name = ?, schema_name = ?, schema_id = ?, description = ?, permission = ?, status = ?, updated_by = ?, updated_at = ?, icon_type = ?, icon = ?, icon_background = ?, workspace_id = ?
		WHERE id = ?
	`
	err := r.db.WithContext(ctx).Exec(
		query,
		ds.Name,
		ds.SchemaName,
		ds.SchemaID,
		ds.Description,
		ds.Permission,
		ds.Status,
		ds.UpdatedBy,
		ds.UpdatedAt,
		ds.IconType,
		ds.Icon,
		ds.IconBackground,
		ds.WorkspaceID,
		ds.ID,
	).Error
	return err
}

func (r *PostgresDataSourceRepository) UpdateGuardPolicy(ctx context.Context, id string, policy []byte) error {
	return r.db.WithContext(ctx).Exec(
		`UPDATE data_sources SET guard_policy = ?, updated_at = NOW() WHERE id = ?`,
		policy,
		id,
	).Error
}

// Delete deletes a data source record
func (r *PostgresDataSourceRepository) Delete(ctx context.Context, id string) error {
	query := `
		DELETE FROM data_sources
		WHERE id = ?
	`
	err := r.db.WithContext(ctx).Exec(query, id).Error
	return err
}

// Create creates a new operation log record
func (r *PostgresSQLOperationRepository) Create(ctx context.Context, log *model.DataSourceSQLOperation) error {
	if log.ID == "" {
		log.ID = uuid.New().String()
	}

	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO data_source_sql_operations (
			id, organization_id, workspace_id, data_source_id, table_id, table_name, data_source_name,
			sql_statement, operation_type, client_type, workflow_run_id, node_id, params_json, row_count,
			duration_ms, error_code, error_message, executed_at, request_id, guard_verdict,
			guard_reasons, guard_policy, start_time, end_time, status, created_by, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	err := r.db.WithContext(ctx).Exec(
		query,
		log.ID,
		log.OrganizationID,
		log.WorkspaceID,
		log.DataSourceID,
		log.TableID,
		log.TableName,
		log.DataSourceName,
		log.SqlStatement,
		log.OperationType,
		log.ClientType,
		log.WorkflowRunID,
		log.NodeID,
		log.ParamsJSON,
		log.RowCount,
		log.DurationMS,
		log.ErrorCode,
		log.ErrorMessage,
		log.ExecutedAt,
		log.RequestID,
		log.GuardVerdict,
		log.GuardReasons,
		log.GuardPolicy,
		log.StartTime,
		log.EndTime,
		log.Status,
		log.CreatedBy,
		log.CreatedAt,
	).Error
	return err
}

func (r *PostgresSQLOperationRepository) Insert(ctx context.Context, records []audit.Record) error {
	for _, record := range records {
		paramsJSON, err := json.Marshal(record.Params)
		if err != nil {
			paramsJSON = nil
		}

		durationMS := record.DurationMS
		executedAt := record.ExecutedAt
		log := &model.DataSourceSQLOperation{
			ID:             uuid.New().String(),
			OrganizationID: record.OrganizationID,
			WorkspaceID:    stringPtrOrNil(record.WorkspaceID),
			DataSourceID:   record.DataSourceID,
			TableID:        stringPtrOrNil(record.TableID),
			TableName:      stringPtrOrNil(record.TableName),
			DataSourceName: stringPtrOrNil(record.DataSourceName),
			SqlStatement:   record.SQLStatement,
			OperationType:  record.OperationType,
			ClientType:     string(record.ClientType),
			WorkflowRunID:  stringPtrOrNil(record.WorkflowRunID),
			NodeID:         stringPtrOrNil(record.NodeID),
			ParamsJSON:     paramsJSON,
			RowCount:       record.RowCount,
			DurationMS:     &durationMS,
			ErrorCode:      stringPtrOrNil(record.ErrorCode),
			ErrorMessage:   stringPtrOrNil(record.ErrorMessage),
			ExecutedAt:     &executedAt,
			RequestID:      stringPtrOrNil(record.RequestID),
			GuardVerdict:   stringPtrOrNil(record.GuardVerdict),
			GuardReasons:   record.GuardReasons,
			GuardPolicy:    record.GuardPolicy,
			StartTime:      record.StartTime,
			EndTime:        record.EndTime,
			Status:         string(record.Status),
			CreatedBy:      record.CreatedBy,
			CreatedAt:      time.Now(),
		}
		if log.ClientType == "" {
			log.ClientType = string(audit.ClientTypeUnknown)
		}
		if err := r.Create(ctx, log); err != nil {
			return err
		}
	}
	return nil
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// ListByTableID lists operation logs for a specific table
func (r *PostgresSQLOperationRepository) ListByTableID(ctx context.Context, tableID string, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT id, organization_id, table_id, table_name, data_source_name, sql_statement, operation_type, start_time, end_time, status, created_by, created_at
		FROM data_source_sql_operations
		WHERE table_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.WithContext(ctx).Raw(query, tableID, limit, offset).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*model.DataSourceSQLOperation
	for rows.Next() {
		var log model.DataSourceSQLOperation
		err := rows.Scan(
			&log.ID,
			&log.OrganizationID,
			&log.TableID,
			&log.TableName,
			&log.DataSourceName,
			&log.SqlStatement,
			&log.OperationType,
			&log.StartTime,
			&log.EndTime,
			&log.Status,
			&log.CreatedBy,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, &log)
	}

	return logs, rows.Err()
}

// ListByOrganizationID lists operation logs for a specific organization
func (r *PostgresSQLOperationRepository) ListByOrganizationID(ctx context.Context, organizationID string, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT id, organization_id, table_id, table_name, data_source_name, sql_statement, operation_type, start_time, end_time, status, created_by, created_at
		FROM data_source_sql_operations
		WHERE organization_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.WithContext(ctx).Raw(query, organizationID, limit, offset).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*model.DataSourceSQLOperation
	for rows.Next() {
		var log model.DataSourceSQLOperation
		err := rows.Scan(
			&log.ID,
			&log.OrganizationID,
			&log.TableID,
			&log.TableName,
			&log.DataSourceName,
			&log.SqlStatement,
			&log.OperationType,
			&log.StartTime,
			&log.EndTime,
			&log.Status,
			&log.CreatedBy,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, &log)
	}

	return logs, rows.Err()
}

// ListByDataSourceID lists operation logs for a specific data source
func (r *PostgresSQLOperationRepository) ListByDataSourceID(ctx context.Context, dataSourceID string, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT id, organization_id, data_source_id, table_id, table_name, data_source_name, sql_statement, operation_type, start_time, end_time, status, created_by, created_at
		FROM data_source_sql_operations
		WHERE data_source_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.WithContext(ctx).Raw(query, dataSourceID, limit, offset).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*model.DataSourceSQLOperation
	for rows.Next() {
		var log model.DataSourceSQLOperation
		err := rows.Scan(
			&log.ID,
			&log.OrganizationID,
			&log.DataSourceID,
			&log.TableID,
			&log.TableName,
			&log.DataSourceName,
			&log.SqlStatement,
			&log.OperationType,
			&log.StartTime,
			&log.EndTime,
			&log.Status,
			&log.CreatedBy,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, &log)
	}

	return logs, rows.Err()
}

// CountByDataSourceID counts operation logs for a specific data source
func (r *PostgresSQLOperationRepository) CountByDataSourceID(ctx context.Context, dataSourceID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.DataSourceSQLOperation{}).
		Where("data_source_id = ?", dataSourceID).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ListByDataSourceIDWithFilters lists operation logs for a specific data source with filters
func (r *PostgresSQLOperationRepository) ListByDataSourceIDWithFilters(ctx context.Context, dataSourceID string, filters dto.SQLOperationFilter, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Build query with filters
	query := `
		SELECT id, organization_id, data_source_id, table_id, table_name, data_source_name, sql_statement, operation_type, start_time, end_time, status, created_by, created_at
		FROM data_source_sql_operations
		WHERE data_source_id = ?
	`

	args := []interface{}{dataSourceID}

	// Add filters
	if filters.TableID != nil {
		query += " AND table_id = ?"
		args = append(args, *filters.TableID)
	}

	if filters.CreatedBy != nil {
		query += " AND created_by = ?"
		args = append(args, *filters.CreatedBy)
	}

	if filters.OperationType != nil {
		query += " AND operation_type = ?"
		args = append(args, *filters.OperationType)
	}

	if filters.Status != nil {
		query += " AND status = ?"
		args = append(args, *filters.Status)
	}

	if filters.CreatedAtGTE != nil {
		query += " AND created_at >= ?"
		args = append(args, *filters.CreatedAtGTE)
	}

	if filters.CreatedAtLTE != nil {
		query += " AND created_at <= ?"
		args = append(args, *filters.CreatedAtLTE)
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*model.DataSourceSQLOperation
	for rows.Next() {
		var log model.DataSourceSQLOperation
		err := rows.Scan(
			&log.ID,
			&log.OrganizationID,
			&log.DataSourceID,
			&log.TableID,
			&log.TableName,
			&log.DataSourceName,
			&log.SqlStatement,
			&log.OperationType,
			&log.StartTime,
			&log.EndTime,
			&log.Status,
			&log.CreatedBy,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, &log)
	}

	return logs, rows.Err()
}

// CountByDataSourceIDWithFilters counts operation logs for a specific data source with filters
func (r *PostgresSQLOperationRepository) CountByDataSourceIDWithFilters(ctx context.Context, dataSourceID string, filters dto.SQLOperationFilter) (int64, error) {
	// Build query with filters
	query := `
		SELECT COUNT(*)
		FROM data_source_sql_operations
		WHERE data_source_id = ?
	`

	args := []interface{}{dataSourceID}

	// Add filters
	if filters.TableID != nil {
		query += " AND table_id = ?"
		args = append(args, *filters.TableID)
	}

	if filters.CreatedBy != nil {
		query += " AND created_by = ?"
		args = append(args, *filters.CreatedBy)
	}

	if filters.OperationType != nil {
		query += " AND operation_type = ?"
		args = append(args, *filters.OperationType)
	}

	if filters.Status != nil {
		query += " AND status = ?"
		args = append(args, *filters.Status)
	}

	if filters.CreatedAtGTE != nil {
		query += " AND created_at >= ?"
		args = append(args, *filters.CreatedAtGTE)
	}

	if filters.CreatedAtLTE != nil {
		query += " AND created_at <= ?"
		args = append(args, *filters.CreatedAtLTE)
	}

	var count int64
	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&count).Error
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (r *PostgresSQLOperationRepository) ListAuditByWorkspace(ctx context.Context, organizationID, workspaceID string, filters dto.SQLAuditFilter, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var logs []*model.DataSourceSQLOperation
	base := r.applySQLAuditFilters(
		r.db.WithContext(ctx).Model(&model.DataSourceSQLOperation{}).
			Where("organization_id = ? AND workspace_id = ?", organizationID, workspaceID),
		filters,
	)
	ranked := base.Select("id, ROW_NUMBER() OVER (PARTITION BY COALESCE(NULLIF(request_id, ''), id::text) ORDER BY created_at DESC, id DESC) AS rn")
	latestIDs := r.db.Table("(?) AS ranked_sql_audit", ranked).Select("id").Where("rn = 1")
	err := r.db.WithContext(ctx).
		Model(&model.DataSourceSQLOperation{}).
		Where("id IN (?)", latestIDs).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs).Error
	return logs, err
}

func (r *PostgresSQLOperationRepository) CountAuditByWorkspace(ctx context.Context, organizationID, workspaceID string, filters dto.SQLAuditFilter) (int64, error) {
	var count int64
	base := r.applySQLAuditFilters(
		r.db.WithContext(ctx).Model(&model.DataSourceSQLOperation{}).
			Where("organization_id = ? AND workspace_id = ?", organizationID, workspaceID),
		filters,
	)
	ranked := base.Select("id, ROW_NUMBER() OVER (PARTITION BY COALESCE(NULLIF(request_id, ''), id::text) ORDER BY created_at DESC, id DESC) AS rn")
	latestIDs := r.db.Table("(?) AS ranked_sql_audit", ranked).Select("id").Where("rn = 1")
	err := r.db.WithContext(ctx).Table("(?) AS latest_sql_audit", latestIDs).Count(&count).Error
	return count, err
}

func (r *PostgresSQLOperationRepository) FindAuditByWorkspaceAndID(ctx context.Context, organizationID, workspaceID, operationID string) (*model.DataSourceSQLOperation, error) {
	var log model.DataSourceSQLOperation
	err := r.db.WithContext(ctx).
		Model(&model.DataSourceSQLOperation{}).
		Where("organization_id = ? AND workspace_id = ? AND id = ?", organizationID, workspaceID, operationID).
		First(&log).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *PostgresSQLOperationRepository) applySQLAuditFilters(query *gorm.DB, filters dto.SQLAuditFilter) *gorm.DB {
	if filters.DataSourceID != nil {
		query = query.Where("data_source_id = ?", *filters.DataSourceID)
	}
	if filters.TableID != nil {
		query = query.Where("table_id = ?", *filters.TableID)
	}
	if filters.ClientType != nil {
		query = query.Where("client_type = ?", *filters.ClientType)
	}
	if filters.WorkflowRunID != nil {
		query = query.Where("workflow_run_id = ?", *filters.WorkflowRunID)
	}
	if filters.NodeID != nil {
		query = query.Where("node_id = ?", *filters.NodeID)
	}
	if filters.CreatedBy != nil {
		query = query.Where("created_by = ?", *filters.CreatedBy)
	}
	if filters.OperationType != nil {
		query = query.Where("operation_type = ?", *filters.OperationType)
	}
	if filters.Status != nil {
		query = query.Where("status = ?", *filters.Status)
	}
	if filters.StartTime != nil {
		query = query.Where("executed_at >= ?", *filters.StartTime)
	}
	if filters.EndTime != nil {
		query = query.Where("executed_at <= ?", *filters.EndTime)
	}
	return query
}
