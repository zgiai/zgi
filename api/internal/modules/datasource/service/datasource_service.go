package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/datasource/model"
	excelimportmodel "github.com/zgiai/zgi/api/internal/modules/datasource/model/excelimport"
	"github.com/zgiai/zgi/api/internal/modules/datasource/repository"
	excelimportrepo "github.com/zgiai/zgi/api/internal/modules/datasource/repository/excelimport"
	excelimportsvc "github.com/zgiai/zgi/api/internal/modules/datasource/service/excelimport"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	defaultmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/prompt"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	sql_base "github.com/zgiai/zgi/api/pkg/sql_base"
	"github.com/zgiai/zgi/api/pkg/sql_base/audit"
	"github.com/zgiai/zgi/api/pkg/sql_base/guard"
)

var errDataSourceTableNotFound = errors.New("data source table not found")

const (
	fileIngestStageParse       = "parse"
	fileIngestStageRecognition = "recognition"
)

func fileIngestContentHash(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum)
}

func fileIngestExtractionInfo(extraction databaseIngestionExtractionResult, content string) *dto.FileIngestExtractionInfo {
	return &dto.FileIngestExtractionInfo{
		PrimaryStrategy: extraction.PrimaryStrategy,
		ActualStrategy:  extraction.ActualStrategy,
		FallbackReason:  extraction.FallbackReason,
		SourceType:      extraction.SourceType,
		ContentHash:     fileIngestContentHash(content),
		Attempts:        append([]dto.FileIngestAttempt(nil), extraction.Attempts...),
	}
}

func updateFileIngestAttemptResult(extraction *databaseIngestionExtractionResult, method, result, reason string, recordCount int) {
	if extraction == nil {
		return
	}
	for i := len(extraction.Attempts) - 1; i >= 0; i-- {
		if extraction.Attempts[i].Method != method {
			continue
		}
		extraction.Attempts[i].Result = result
		extraction.Attempts[i].Reason = reason
		extraction.Attempts[i].RecordCount = recordCount
		return
	}
	extraction.Attempts = append(extraction.Attempts, dto.FileIngestAttempt{
		Method:      method,
		Status:      databaseIngestionAttemptStatusCompleted,
		Result:      result,
		Reason:      reason,
		RecordCount: recordCount,
	})
}

func fileIngestAttemptMethodForExtraction(extraction databaseIngestionExtractionResult) string {
	return databaseIngestionAttemptMethodFileParse
}

func IsDataSourceTableNotFound(err error) bool {
	return errors.Is(err, errDataSourceTableNotFound)
}

// DataSourceService defines the interface for data source service
type DataSourceService interface {
	CreateDataSource(ctx context.Context, organizationID string, accountID string, req dto.CreateDataSourceRequest) (*dto.DataSourceResponse, error)
	ListDataSources(ctx context.Context, organizationID, accountID string, filterWorkspaceIDs []string) ([]*dto.DataSourceResponse, error)
	GetDataSourceByName(ctx context.Context, organizationID, name string) (*dto.DataSourceResponse, error)
	GetDataSourceByID(ctx context.Context, organizationID, id, accountID string) (*dto.DataSourceResponse, error)
	UpdateDataSource(ctx context.Context, organizationID, id, accountID string, req dto.UpdateDataSourceRequest) (*dto.DataSourceResponse, error)
	DeleteDataSourceByID(ctx context.Context, organizationID, id string, accountID string) error
	GetGuardPolicy(ctx context.Context, organizationID, dataSourceID string) (guard.Policy, error)
	UpdateGuardPolicy(ctx context.Context, organizationID, dataSourceID string, policy guard.Policy) (guard.Policy, error)
	PreviewGuard(ctx context.Context, organizationID, dataSourceID, sql string, policy *guard.Policy) (guard.Result, error)

	// Table operations
	CreateTable(ctx context.Context, organizationID, dataSourceID string, accountID string, req dto.CreateTableRequest) (*model.Table, error)
	ListTables(ctx context.Context, organizationID, dataSourceID string, accountID string) ([]*model.Table, error)
	GetTable(ctx context.Context, organizationID, dataSourceID, tableID string, accountID string) (*model.Table, error)
	DeleteTable(ctx context.Context, organizationID, dataSourceID, tableID string, accountID string) error
	UpdateTable(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.UpdateTableRequest) (*model.Table, error)
	UpdateTableColumns(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.UpdateTableColumnsRequest) error
	GetTableColumns(ctx context.Context, organizationID, dataSourceID, tableID string, includeSystemFields bool) (dto.GetTableColumnsResponse, error)

	// Table prompt operations
	GetTablePrompt(ctx context.Context, tableID string, lang string) (*model.TablePrompt, error)
	UpsertTablePrompt(ctx context.Context, tableID string, req dto.UpdateTablePromptRequest) (*model.TablePrompt, error)
	DeleteTablePrompt(ctx context.Context, tableID string) error

	// Table data operations
	AddTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.AddRecordRequest) (dto.AddRecordResponse, error)
	UpdateTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.UpdateRecordRequest) (dto.UpdateRecordResponse, error)
	DeleteTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.DeleteRecordRequest) (dto.DeleteRecordResponse, error)
	QueryTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, limit, offset int, order string) (dto.QueryRecordResponse, error)
	ImportTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, file io.Reader, fileName string, skipUnmatchedColumns bool) (dto.ImportRecordResponse, error)
	ImportTableRecordsFromUploadFile(ctx context.Context, organizationID, dataSourceID, tableID, accountID, uploadFileID string, skipUnmatchedColumns bool) (dto.ImportRecordResponse, error)

	// File analysis for table structure
	AnalyzeFileForTable(ctx context.Context, dataSourceID, accountID, fileID string, description *string, modelSpec *dto.ModelSpec) (dto.AnalyzeFileForTableResponse, error)
	// File ingestion into table
	ParseFileForTableIngest(ctx context.Context, organizationID, accountID string, req dto.ParseFileForTableIngestRequest) (dto.ParseFileForTableIngestResponse, error)
	ExtractTextToTableRecords(ctx context.Context, organizationID, accountID string, req dto.ExtractTextToTableRecordsRequest) (dto.ExtractTextToTableRecordsResponse, error)
	IngestFileToTable(ctx context.Context, organizationID, accountID string, req dto.IngestFileToTableRequest) (dto.IngestFileToTableResponse, error)
	BatchIngestFileToTable(ctx context.Context, organizationID, accountID string, req dto.BatchIngestFileToTableRequest) (dto.BatchIngestFileToTableResponse, error)

	// SQL operation logs
	ListOperationLogsByDataSourceID(ctx context.Context, organizationID, dataSourceID string, limit, offset int) ([]*model.DataSourceSQLOperation, error)
	CountOperationLogsByDataSourceID(ctx context.Context, organizationID, dataSourceID string) (int64, error)
	ListOperationLogsByDataSourceIDWithFilters(ctx context.Context, organizationID, dataSourceID string, filters dto.SQLOperationFilter, limit, offset int) ([]*model.DataSourceSQLOperation, error)
	CountOperationLogsByDataSourceIDWithFilters(ctx context.Context, organizationID, dataSourceID string, filters dto.SQLOperationFilter) (int64, error)
	ListSQLAuditByWorkspace(ctx context.Context, organizationID, workspaceID string, filters dto.SQLAuditFilter, limit, offset int) ([]*model.DataSourceSQLOperation, error)
	CountSQLAuditByWorkspace(ctx context.Context, organizationID, workspaceID string, filters dto.SQLAuditFilter) (int64, error)
	GetSQLAuditDetail(ctx context.Context, organizationID, workspaceID, operationID string) (*model.DataSourceSQLOperation, error)

	// Table template operations
	GenerateTableTemplateExcel(ctx context.Context, organizationID, dataSourceID, tableID string) ([]byte, error)

	// Excel import operations
	AnalyzeExcelImport(ctx context.Context, organizationID, dataSourceID, accountID string, req dto.AnalyzeExcelImportRequest) (dto.AnalyzeExcelImportData, error)
	RecognizeExcelImportFields(ctx context.Context, organizationID, dataSourceID, accountID, jobID string, req dto.RecognizeExcelImportRequest) (dto.RecognizeExcelImportData, error)
	ConfirmExcelImport(ctx context.Context, organizationID, dataSourceID, accountID, jobID string, req dto.ConfirmExcelImportRequest) (dto.ConfirmExcelImportData, error)
	GetExcelImportJob(ctx context.Context, organizationID, dataSourceID, jobID string) (*dto.ExcelImportJobResponse, error)
	ListExcelImportErrors(ctx context.Context, organizationID, dataSourceID, jobID string, limit, offset int) (dto.ExcelImportErrorList, error)
}

// dataSourceService implements DataSourceService
type dataSourceService struct {
	repo                      repository.DataSourceRepository
	tableRepo                 repository.TableRepository
	promptRepo                repository.PromptRepository
	sqlOperationRepo          repository.SQLOperationRepository
	sqlBase                   sql_base.SQLBase
	sqlAuditRecorder          audit.Recorder
	accountService            interfaces.AccountService
	fileService               interfaces.FileService
	organizationService       interfaces.OrganizationService
	resourcePermissionService interfaces.ResourcePermissionService
	quotaService              interfaces.QuotaService
	llmClient                 llmclient.LLMClient
	defaultModelResolver      defaultmodelsvc.DefaultModelResolver
	contentParseService       contracts.ContentParseService
	db                        *gorm.DB
}

type DataSourceServiceOption func(*dataSourceService)

func WithContentParseService(contentParseService contracts.ContentParseService) DataSourceServiceOption {
	return func(s *dataSourceService) {
		s.contentParseService = contentParseService
	}
}

type databaseIngestionTableContext struct {
	DataSourceID      string
	LLMOrganizationID string
	Columns           dto.GetTableColumnsResponse
}

// NewDataSourceService creates a new DataSourceService
func NewDataSourceService(repo repository.DataSourceRepository, tableRepo repository.TableRepository, promptRepo repository.PromptRepository, sqlOperationRepo repository.SQLOperationRepository, accountService interfaces.AccountService, fileService interfaces.FileService, organizationService interfaces.OrganizationService, resourcePermissionService interfaces.ResourcePermissionService, quotaService interfaces.QuotaService, llmClient llmclient.LLMClient, defaultModelResolver defaultmodelsvc.DefaultModelResolver, db *gorm.DB, options ...DataSourceServiceOption) DataSourceService {
	sqlAuditRecorder := audit.NewAsyncRecorder(sqlOperationRepo)
	sqlBaseClient, err := sql_base.NewSQLBaseClient(
		sql_base.WithAuditRecorder(sqlAuditRecorder),
		sql_base.WithGuardPolicyProvider(func(ctx context.Context, dataSourceID string) (*guard.Policy, error) {
			dataSource, err := repo.FindByID(ctx, dataSourceID)
			if err != nil || dataSource == nil {
				return nil, err
			}
			policy, err := guard.ParsePolicyJSON([]byte(dataSource.GuardPolicy))
			if err != nil {
				return nil, err
			}
			return &policy, nil
		}),
	)
	if err != nil {
		panic("failed to create postgres meta client: " + err.Error())
	}

	svc := &dataSourceService{
		repo:                      repo,
		tableRepo:                 tableRepo,
		promptRepo:                promptRepo,
		sqlOperationRepo:          sqlOperationRepo,
		sqlBase:                   sqlBaseClient,
		sqlAuditRecorder:          sqlAuditRecorder,
		accountService:            accountService,
		fileService:               fileService,
		organizationService:       organizationService,
		resourcePermissionService: resourcePermissionService,
		quotaService:              quotaService,
		llmClient:                 llmClient,
		defaultModelResolver:      defaultModelResolver,
		db:                        db,
	}
	for _, option := range options {
		if option != nil {
			option(svc)
		}
	}
	return svc
}

func (s *dataSourceService) Close(ctx context.Context) error {
	if s == nil || s.sqlAuditRecorder == nil {
		return nil
	}
	return s.sqlAuditRecorder.Close(ctx)
}

// CreateDataSource creates a new data source
func (s *dataSourceService) CreateDataSource(ctx context.Context, organizationID string, accountID string, req dto.CreateDataSourceRequest) (*dto.DataSourceResponse, error) {
	// Check if data source with the same name already exists
	existing, err := s.repo.FindByOrganizationAndName(ctx, organizationID, req.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing data source: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("data source with name '%s' already exists", req.Name)
	}

	workspaceID := req.WorkspaceID
	// use role_permissions to check if account has permission in handler, instead of check in service use role

	// For virtual data sources, we don't create actual schemas
	// Just use a fixed schema name and ID
	schemaName := "public"
	schemaID := 0 // Public schema ID is typically 0 or can be a fixed value
	// // Generate unique schema name with organizationID and uuid only to avoid issues with special characters in req.Name
	// schemaName := fmt.Sprintf("ds_%s_%s", organizationID, uuid.New().String())

	// // Create schema using sql_base CreateSchema
	// schemaReq := sql_base.CreateSchemaRequest{
	// 	Name: schemaName,
	// }
	// createdSchema, err := s.sql_base.CreateSchema(ctx, schemaReq)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create schema: %w", err)
	// }

	// // Get schema details using GetSchema
	// schemaDetails, err := s.sql_base.GetSchema(ctx, createdSchema.ID)
	// if err != nil {
	// 	// If we can't get schema details, try to delete the created schema
	// 	_, _ = s.sql_base.DeleteSchema(ctx, createdSchema.ID, true)
	// 	return nil, fmt.Errorf("failed to get schema details: %w", err)
	// }

	// Create data source record
	dataSource := &model.DataSource{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		Name:           req.Name,
		SchemaID:       schemaID,
		SchemaName:     schemaName,
		Description:    req.Description,
		Permission:     req.Permission,
		Status:         "active",
		CreatedBy:      accountID,
		UpdatedBy:      accountID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		IconType:       req.IconType,
		Icon:           req.Icon,
		IconBackground: req.IconBackground,
		GuardPolicy:    guard.DefaultPolicyJSON(),
	}

	if err := s.repo.Create(ctx, dataSource); err != nil {
		// // If metadata creation fails, try to delete the schema
		// _, _ = s.sqlBase.DeleteSchema(ctx, schemaDetails.ID, true)
		return nil, fmt.Errorf("failed to save data source metadata: %w", err)
	}

	return dto.ConvertDataSourceModelToResponse(dataSource), nil
}

// ListDataSources lists data sources
func (s *dataSourceService) ListDataSources(ctx context.Context, organizationID, accountID string, filterWorkspaceIDs []string) ([]*dto.DataSourceResponse, error) {
	// Check if user is admin
	isAdmin, err := s.accountService.IsOrganizationAdminOrOwner(ctx, organizationID, accountID)
	if err != nil {
		isAdmin = false
	}

	dataSources, err := s.repo.ListByOrganizationWithPermissionFilter(ctx, organizationID, accountID, isAdmin, filterWorkspaceIDs)
	if err != nil {
		return nil, err
	}

	// Prepare resources for batch permission check
	resources := make([]interfaces.ResourcePermissionInfo, len(dataSources))
	for i, ds := range dataSources {
		resources[i] = interfaces.ResourcePermissionInfo{
			ResourceID:  ds.ID,
			WorkspaceID: getStringValue(ds.WorkspaceID),
			CreatedBy:   ds.CreatedBy,
			GroupID:     &ds.OrganizationID,
		}
	}

	// Batch check permissions
	permissionMap, err := s.resourcePermissionService.CheckBatchResourceEditPermission(ctx, interfaces.BatchResourcePermissionParams{
		AccountID: accountID,
		Resources: resources,
	})
	if err != nil {
		// On error, default to false for all
		permissionMap = make(map[string]bool)
	}

	// Convert datasources to response with permission info
	var responses []*dto.DataSourceResponse
	for _, ds := range dataSources {
		response := dto.ConvertDataSourceModelToResponse(ds)

		// Set can_edit from batch permission check
		canEdit, exists := permissionMap[ds.ID]
		if !exists {
			canEdit = false
		}
		response.CanEdit = canEdit

		responses = append(responses, response)
	}

	return responses, nil
}

// getStringValue safely extracts string value from pointer
func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func auditWorkspaceID(organizationID string, workspaceID *string) string {
	if workspaceID != nil && *workspaceID != "" {
		return *workspaceID
	}
	return organizationID
}

func sqlAuditContext(organizationID string, dataSource *model.DataSource, table *model.Table, accountID string, operationType string) *audit.Context {
	if dataSource == nil {
		return nil
	}

	auditCtx := &audit.Context{
		OrganizationID: organizationID,
		WorkspaceID:    auditWorkspaceID(organizationID, dataSource.WorkspaceID),
		DataSourceID:   dataSource.ID,
		DataSourceName: dataSource.Name,
		ClientType:     audit.ClientTypeAPI,
		CreatedBy:      accountID,
		OperationType:  operationType,
	}
	if policy, err := guard.ParsePolicyJSON([]byte(dataSource.GuardPolicy)); err == nil {
		auditCtx.GuardPolicy = &policy
	}
	if table != nil {
		auditCtx.TableID = table.ID
		auditCtx.TableName = table.Name
	}
	return auditCtx
}

func (s *dataSourceService) guardPolicyForDataSource(ctx context.Context, dataSourceID string) *guard.Policy {
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil || dataSource == nil {
		return nil
	}
	policy, err := guard.ParsePolicyJSON([]byte(dataSource.GuardPolicy))
	if err != nil {
		return nil
	}
	return &policy
}

func (s *dataSourceService) GetGuardPolicy(ctx context.Context, organizationID, dataSourceID string) (guard.Policy, error) {
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return guard.Policy{}, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil || dataSource.OrganizationID != organizationID {
		return guard.Policy{}, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}
	return guard.ParsePolicyJSON([]byte(dataSource.GuardPolicy))
}

func (s *dataSourceService) UpdateGuardPolicy(ctx context.Context, organizationID, dataSourceID string, policy guard.Policy) (guard.Policy, error) {
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return guard.Policy{}, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil || dataSource.OrganizationID != organizationID {
		return guard.Policy{}, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}
	policy, err = guard.NormalizeAndValidatePolicy(policy)
	if err != nil {
		return guard.Policy{}, fmt.Errorf("invalid sql guard policy: %w", err)
	}
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return guard.Policy{}, fmt.Errorf("failed to encode guard policy: %w", err)
	}
	if err := s.repo.UpdateGuardPolicy(ctx, dataSourceID, policyJSON); err != nil {
		return guard.Policy{}, fmt.Errorf("failed to update guard policy: %w", err)
	}
	return policy, nil
}

func (s *dataSourceService) PreviewGuard(ctx context.Context, organizationID, dataSourceID, sql string, policy *guard.Policy) (guard.Result, error) {
	currentPolicy, err := s.GetGuardPolicy(ctx, organizationID, dataSourceID)
	if err != nil {
		return guard.Result{}, err
	}
	if policy != nil {
		currentPolicy = *policy
	}
	currentPolicy, err = guard.NormalizeAndValidatePolicy(currentPolicy)
	if err != nil {
		return guard.Result{}, fmt.Errorf("invalid sql guard policy: %w", err)
	}
	return guard.Check(sql, currentPolicy), nil
}

// GetDataSourceByName gets a specific data source by name
func (s *dataSourceService) GetDataSourceByName(ctx context.Context, organizationID, name string) (*dto.DataSourceResponse, error) {
	dataSource, err := s.repo.FindByOrganizationAndName(ctx, organizationID, name)
	if err != nil {
		return nil, err
	}

	if dataSource == nil {
		return nil, nil
	}

	return dto.ConvertDataSourceModelToResponse(dataSource), nil
}

// GetDataSourceByID gets a specific data source by ID
func (s *dataSourceService) GetDataSourceByID(ctx context.Context, organizationID, id, accountID string) (*dto.DataSourceResponse, error) {
	dataSource, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if dataSource == nil {
		return nil, nil
	}

	response := dto.ConvertDataSourceModelToResponse(dataSource)

	// Check single resource edit permission
	canEdit, err := s.resourcePermissionService.CheckSingleResourceEditPermission(ctx, interfaces.SingleResourcePermissionParams{
		AccountID: accountID,
		TenantID:  getStringValue(dataSource.WorkspaceID),
		CreatedBy: dataSource.CreatedBy,
		GroupID:   &dataSource.OrganizationID,
	})
	if err != nil {
		// On error, default to false
		canEdit = false
	}
	response.CanEdit = canEdit

	return response, nil
}

// DeleteDataSourceByID deletes a data source by ID
func (s *dataSourceService) DeleteDataSourceByID(ctx context.Context, organizationID, id string, accountID string) error {
	// Find the data source
	dataSource, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return fmt.Errorf("data source with id '%s' not found", id)
	}

	// Find all tables associated with this data source
	tables, err := s.tableRepo.ListByDataSource(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to list tables for data source: %w", err)
	}

	// Delete all tables associated with this data source
	for _, table := range tables {
		// Use DeleteTable method to ensure consistent table deletion including prompts
		err = s.DeleteTable(ctx, organizationID, id, table.ID, accountID)
		if err != nil {
			return fmt.Errorf("failed to delete table '%s': %w", table.PhysicalTableName, err)
		}
	}

	// For virtual data sources, we don't delete actual schemas
	// _, err = s.sqlBase.DeleteSchema(ctx, dataSource.SchemaID, true)
	// if err != nil {
	// 	return fmt.Errorf("failed to delete schema: %w", err)
	// }
	// Just delete the metadata
	if err := s.repo.Delete(ctx, dataSource.ID); err != nil {
		return fmt.Errorf("failed to delete data source metadata: %w", err)
	}

	return nil
}

// CreateTable creates a new table in a data source
func (s *dataSourceService) CreateTable(ctx context.Context, organizationID, dataSourceID string, accountID string, req dto.CreateTableRequest) (*model.Table, error) {
	// Find the data source
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return nil, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}

	// Check if a table with the same name already exists in this data source
	existingTable, err := s.tableRepo.FindByDataSourceAndName(ctx, dataSourceID, req.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing table: %w", err)
	}
	if existingTable != nil {
		return nil, fmt.Errorf("table with name '%s' already exists in this data source", req.Name)
	}

	// Generate unique
	tableName, err := s.generateUniqueTableName(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	// Create table using sqlBase CreateTable in public schema
	// Add a comment to ensure the request is valid
	comment := fmt.Sprintf("Table created for organization %s, data source %s", organizationID, dataSource.Name)
	createTableReq := sql_base.CreateTableRequest{
		Name:    tableName,
		Schema:  "public", // Always use public schema
		Comment: &comment,
	}

	var createdTable *sql_base.Table
	createTableSQL := fmt.Sprintf("CREATE TABLE %s ();", quoteIdentifier(tableName))
	err = s.auditSQLOperation(ctx, organizationID, dataSourceID, "", dataSource.Name, tableName, accountID, string(model.OperationTypeCreate), createTableSQL, func() error {
		var opErr error
		createdTable, opErr = s.sqlBase.CreateTable(ctx, createTableReq)
		return opErr
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	// Create default columns after table creation
	// id - primary key
	idColumnReq := sql_base.CreateColumnRequest{
		TableID:            createdTable.ID,
		Name:               "id",
		Type:               "bigint",
		IsPrimaryKey:       true,
		IsIdentity:         true,
		IdentityGeneration: stringPtr("BY DEFAULT"),
		IsNullable:         boolPtr(false),
		Comment:            stringPtr("数据的唯一标识（主键）"),
	}
	err = s.auditSQLOperation(ctx, organizationID, dataSourceID, "", dataSource.Name, createdTable.Name, accountID, string(model.OperationTypeCreate), fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s bigint PRIMARY KEY", quoteIdentifier(createdTable.Name), quoteIdentifier("id")), func() error {
		_, opErr := s.sqlBase.CreateColumn(ctx, idColumnReq)
		return opErr
	})
	if err != nil {
		// If column creation fails, try to delete the table
		s.sqlBase.DeleteTable(ctx, createdTable.ID, true)
		return nil, fmt.Errorf("failed to create id column: %w", err)
	}

	// uuid - unique user identifier
	uuidColumnReq := sql_base.CreateColumnRequest{
		TableID:            createdTable.ID,
		Name:               "uuid",
		Type:               "uuid",
		DefaultValue:       "gen_random_uuid()",
		DefaultValueFormat: stringPtr("expression"),
		IsNullable:         boolPtr(false),
		Comment:            stringPtr("用户唯一标识，由系统生成"),
	}
	err = s.auditSQLOperation(ctx, organizationID, dataSourceID, "", dataSource.Name, createdTable.Name, accountID, string(model.OperationTypeCreate), fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s uuid", quoteIdentifier(createdTable.Name), quoteIdentifier("uuid")), func() error {
		_, opErr := s.sqlBase.CreateColumn(ctx, uuidColumnReq)
		return opErr
	})
	if err != nil {
		// If column creation fails, try to delete the table
		s.sqlBase.DeleteTable(ctx, createdTable.ID, true)
		return nil, fmt.Errorf("failed to create uuid column: %w", err)
	}

	// created_time - creation timestamp
	createdTimeColumnReq := sql_base.CreateColumnRequest{
		TableID:            createdTable.ID,
		Name:               "created_time",
		Type:               "timestamp",
		DefaultValue:       "now()",
		DefaultValueFormat: stringPtr("expression"),
		IsNullable:         boolPtr(false),
		Comment:            stringPtr("数据创建时间"),
	}
	err = s.auditSQLOperation(ctx, organizationID, dataSourceID, "", dataSource.Name, createdTable.Name, accountID, string(model.OperationTypeCreate), fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s timestamp", quoteIdentifier(createdTable.Name), quoteIdentifier("created_time")), func() error {
		_, opErr := s.sqlBase.CreateColumn(ctx, createdTimeColumnReq)
		return opErr
	})
	if err != nil {
		// If column creation fails, try to delete the table
		s.sqlBase.DeleteTable(ctx, createdTable.ID, true)
		return nil, fmt.Errorf("failed to create created_time column: %w", err)
	}

	// updated_time - update timestamp
	updatedTimeColumnReq := sql_base.CreateColumnRequest{
		TableID:            createdTable.ID,
		Name:               "updated_time",
		Type:               "timestamp",
		DefaultValue:       "now()",
		DefaultValueFormat: stringPtr("expression"),
		IsNullable:         boolPtr(false),
		Comment:            stringPtr("数据更新时间"),
	}
	err = s.auditSQLOperation(ctx, organizationID, dataSourceID, "", dataSource.Name, createdTable.Name, accountID, string(model.OperationTypeCreate), fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s timestamp", quoteIdentifier(createdTable.Name), quoteIdentifier("updated_time")), func() error {
		_, opErr := s.sqlBase.CreateColumn(ctx, updatedTimeColumnReq)
		return opErr
	})
	if err != nil {
		// If column creation fails, try to delete the table
		s.sqlBase.DeleteTable(ctx, createdTable.ID, true)
		return nil, fmt.Errorf("failed to create updated_time column: %w", err)
	}

	// Create table metadata record
	table := &model.Table{
		OrganizationID:    organizationID,
		DataSourceID:      dataSourceID,
		Name:              req.Name,
		TableID:           createdTable.ID,
		PhysicalTableName: createdTable.Name,
		Description:       req.Description, // Use description from request
		CreatedBy:         accountID,
		UpdatedBy:         accountID,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := s.tableRepo.Create(ctx, table); err != nil {
		// If metadata creation fails, try to delete the table
		s.sqlBase.DeleteTable(ctx, createdTable.ID, true)
		return nil, fmt.Errorf("failed to save table metadata: %w", err)
	}

	return table, nil
}

// generateUniqueTableName generates a unique table name that conforms to the specification
// Format: tbl_<cleaned_name>_<6_digit_random>
// cleaned_name only retains letters, numbers and underscores
// Random part is fixed at 6 characters
// Ensure total length does not exceed PostgreSQL's 63-character limit
func (s *dataSourceService) generateUniqueTableName(ctx context.Context, reqName string) (string, error) {
	const maxAttempts = 10
	const randomSuffixLength = 6
	// "zgi_base_" prefix will be added by the system, which is 9 characters, use lenth 10 for prefix
	const prefixLength = 10

	for i := 0; i < maxAttempts; i++ {
		base := "tbl"

		if reqName != "" {
			cleanedName := s.cleanTableName(reqName)
			if cleanedName != "" {
				// Ensure total length does not exceed 63 characters (PostgreSQL identifier limit)
				// Subtract prefix length as well since system will add "zgi_base_" prefix
				maxBaseLength := 63 - prefixLength - randomSuffixLength - 1 // 6-digit random number + underscore
				if len(base)+1+len(cleanedName) > maxBaseLength {
					cleanedName = cleanedName[:maxBaseLength-len(base)-1]
				}
				base = fmt.Sprintf("%s_%s", base, cleanedName)
			}
		}

		randomSuffix := s.generateRandomString(randomSuffixLength)
		tableName := fmt.Sprintf("%s_%s", base, randomSuffix)

		// Check if table name already exists
		exists, err := s.checkTableNameExists(ctx, tableName)
		if err != nil {
			return "", err
		}

		if !exists {
			return tableName, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique table name after %d attempts", maxAttempts)
}

// cleanTableName cleans the table name, retaining only characters that conform to database naming conventions
func (s *dataSourceService) cleanTableName(name string) string {
	name = strings.ToLower(name)
	var cleaned strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			cleaned.WriteRune(r)
		}
	}
	return cleaned.String()
}

// generateRandomString generates a random string of specified length
func (s *dataSourceService) generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// checkTableNameExists checks if a table name already exists
func (s *dataSourceService) checkTableNameExists(ctx context.Context, tableName string) (bool, error) {
	query := fmt.Sprintf("SELECT 1 FROM information_schema.tables WHERE table_name = '%s'",
		strings.ReplaceAll(tableName, "'", "''"))
	result, err := s.sqlBase.ExecuteSQL(ctx, query, nil, nil)
	if err != nil {
		return false, err
	}

	return len(result.Rows) > 0, nil
}

// ListTables lists all tables in a data source
func (s *dataSourceService) ListTables(ctx context.Context, organizationID, dataSourceID string, accountID string) ([]*model.Table, error) {
	// Find the data source
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return nil, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}

	// Get table metadata
	tables, err := s.tableRepo.ListByDataSource(ctx, dataSourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list table metadata: %w", err)
	}

	// Log the operation
	// s.logSQLOperation(ctx, organizationID, dataSourceID, "", dataSource.Name, "", accountID, string(model.OperationTypeQuery),
	// 	fmt.Sprintf("SELECT * FROM data_source_tables WHERE data_source_id = '%s'", dataSourceID))

	return tables, nil
}

// GetTable gets a specific table in a data source
func (s *dataSourceService) GetTable(ctx context.Context, organizationID, dataSourceID, tableID string, accountID string) (*model.Table, error) {
	// Find the data source
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return nil, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}

	table, err := s.tableRepo.FindByID(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("failed to find table: %w", err)
	}
	if table == nil {
		return nil, fmt.Errorf("table with id '%s' not found", tableID)
	}

	// Log the operation
	// s.logSQLOperation(ctx, organizationID, dataSourceID, tableID, dataSource.Name, table.Name, accountID, string(model.OperationTypeQuery),
	// 	fmt.Sprintf("SELECT * FROM data_source_tables WHERE id = '%s'", tableID))

	return table, nil
}

// DeleteTable deletes a table in a data source
func (s *dataSourceService) DeleteTable(ctx context.Context, organizationID, dataSourceID, tableID string, accountID string) error {
	// Find the data source
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}

	// Find the table metadata
	tableMetadata, err := s.tableRepo.FindByID(ctx, tableID)
	if err != nil {
		return fmt.Errorf("failed to find table metadata: %w", err)
	}
	if tableMetadata == nil {
		return fmt.Errorf("table metadata with id '%s' not found", tableID)
	}

	// Query total row count before deletion
	var totalRows int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdentifier(tableMetadata.PhysicalTableName))
	countResult, err := s.sqlBase.ExecuteSQL(ctx, countQuery, nil, sqlAuditContext(organizationID, dataSource, tableMetadata, accountID, string(model.OperationTypeQuery)))
	if err != nil {
		// If count fails, log but continue with deletion
		logger.WarnContext(ctx, "failed to count rows before table deletion", "table_id", tableID, "physical_table", tableMetadata.PhysicalTableName, err)
		totalRows = 0
	} else if len(countResult.Rows) > 0 && len(countResult.Rows[0]) > 0 {
		// Handle different numeric types returned by database drivers
		switch v := countResult.Rows[0][0].(type) {
		case int64:
			totalRows = v
		case float64:
			totalRows = int64(v)
		case int:
			totalRows = int64(v)
		default:
			totalRows = 0
		}
	}

	dropSQL := fmt.Sprintf("DROP TABLE %s CASCADE", quoteIdentifier(tableMetadata.PhysicalTableName))
	if err := s.auditSQLOperation(ctx, organizationID, dataSourceID, tableID, dataSource.Name, tableMetadata.Name, accountID, string(model.OperationTypeDelete), dropSQL, func() error {
		_, opErr := s.sqlBase.DeleteTable(ctx, tableMetadata.TableID, true)
		return opErr
	}); err != nil {
		return fmt.Errorf("failed to delete physical table: %w", err)
	}

	// Delete metadata and record usage in transaction after the physical table is gone.
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Delete associated table prompt if exists
		err = s.promptRepo.DeleteByTableID(ctx, tableID)
		if err != nil {
			// Log the error but continue with table deletion
			logger.WarnContext(ctx, "failed to delete table prompt", "table_id", tableID, err)
		}

		// Delete table metadata
		if err := s.tableRepo.Delete(ctx, tableID); err != nil {
			return fmt.Errorf("failed to delete table metadata: %w", err)
		}

		// Record usage history if quotaService is available and rows existed
		if s.quotaService != nil && dataSource.OrganizationID != "" && totalRows > 0 {
			// Parse IDs
			organizationUUID, _ := uuid.Parse(dataSource.OrganizationID)
			accountUUID, _ := uuid.Parse(accountID)
			workspaceUUID, _ := uuid.Parse(organizationID)

			// Create metadata
			metadata := quota_model.JSONMap{
				"datasource_id":   dataSourceID,
				"datasource_name": dataSource.Name,
				"table_id":        tableID,
				"table_name":      tableMetadata.Name,
				"rows_affected":   totalRows,
				"operation":       "drop_table",
			}

			// Create usage history record with negative delta
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      organizationUUID,
				AccountID:    accountUUID,
				TenantID:     &workspaceUUID,
				ResourceType: quota_model.ResourceTypeDBRows,
				Delta:        -totalRows, // Negative delta for deletion
				ResourceID:   &tableID,
				ResourceName: &tableMetadata.Name,
				Metadata:     &metadata,
				CreatedAt:    time.Now(),
			}

			// Record usage in transaction
			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record usage: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// UpdateTable updates a table's metadata (name and/or description)
func (s *dataSourceService) UpdateTable(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.UpdateTableRequest) (*model.Table, error) {
	// Find the data source
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return nil, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}

	// Find the table metadata
	table, err := s.tableRepo.FindByID(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("failed to find table: %w", err)
	}
	if table == nil {
		return nil, fmt.Errorf("table with id '%s' not found", tableID)
	}

	// Update fields if provided
	if req.Name != nil && *req.Name != "" {
		table.Name = *req.Name
	}

	if req.Description != nil {
		table.Description = *req.Description
	}

	table.UpdatedBy = accountID
	table.UpdatedAt = time.Now()

	// Update table metadata
	if err := s.tableRepo.Update(ctx, table); err != nil {
		return nil, fmt.Errorf("failed to update table metadata: %w", err)
	}

	return table, nil
}

// UpdateTableColumns updates the columns of a specific table
func (s *dataSourceService) UpdateTableColumns(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.UpdateTableColumnsRequest) error {
	// Find the data source
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}

	// Find the table metadata
	tableMetadata, err := s.tableRepo.FindByID(ctx, tableID)
	if err != nil {
		return fmt.Errorf("failed to find table metadata: %w", err)
	}
	if tableMetadata == nil {
		return fmt.Errorf("table metadata with id '%s' not found", tableID)
	}

	// Get existing columns by getting the table information
	tableInfo, err := s.sqlBase.GetTable(ctx, tableMetadata.TableID)
	if err != nil {
		return fmt.Errorf("failed to get table: %w", err)
	}

	// Create a map of existing columns by ID for easy lookup
	existingColumnsMap := make(map[string]sql_base.Column)
	for _, col := range tableInfo.Columns {
		// Skip default system columns
		if col.Name == "id" || col.Name == "uuid" || col.Name == "created_time" || col.Name == "updated_time" {
			continue
		}
		existingColumnsMap[col.ID] = col
	}

	// Create a map of incoming columns by ID for easy lookup
	incomingColumnsMap := make(map[string]dto.TableColumn)
	for _, col := range req.Columns {
		if col.ID != "" {
			incomingColumnsMap[col.ID] = col
		}
	}

	// Identify columns to delete (exist in DB but not in incoming request)
	columnsToDelete := make([]sql_base.Column, 0)
	for id, col := range existingColumnsMap {
		if _, exists := incomingColumnsMap[id]; !exists {
			columnsToDelete = append(columnsToDelete, col)
		}
	}

	// Identify columns to create (have no ID in request)
	columnsToCreate := make([]dto.TableColumn, 0)
	for _, col := range req.Columns {
		if col.ID == "" {
			columnsToCreate = append(columnsToCreate, col)
		}
	}

	// Identify columns to update (have ID and exist in DB)
	columnsToUpdate := make([]dto.TableColumn, 0)
	for _, col := range req.Columns {
		if col.ID != "" {
			if _, exists := existingColumnsMap[col.ID]; exists {
				columnsToUpdate = append(columnsToUpdate, col)
			}
		}
	}

	// Delete columns that are not in the incoming request
	for _, col := range columnsToDelete {
		dropColumnSQL := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s CASCADE", quoteIdentifier(tableMetadata.PhysicalTableName), quoteIdentifier(col.Name))
		err = s.auditSQLOperation(ctx, organizationID, dataSourceID, tableID, dataSource.Name, tableMetadata.Name, accountID, string(model.OperationTypeUpdate), dropColumnSQL, func() error {
			_, opErr := s.sqlBase.DeleteColumn(ctx, col.ID, true)
			return opErr
		})
		if err != nil {
			return fmt.Errorf("failed to delete column '%s': %w", col.Name, err)
		}
	}

	// Update existing columns
	for _, col := range columnsToUpdate {
		existingCol := existingColumnsMap[col.ID]

		// Check if any properties have changed
		needsUpdate := false
		updateReq := sql_base.UpdateColumnRequest{}

		if existingCol.Name != col.Name {
			updateReq.Name = col.Name
			needsUpdate = true
		}

		// Use normalized data type for comparison
		if existingCol.NormalizeDataType() != col.Type {
			updateReq.Type = col.Type
			needsUpdate = true
		}

		if (existingCol.Comment == nil && col.Description != nil) ||
			(existingCol.Comment != nil && col.Description == nil) ||
			(existingCol.Comment != nil && col.Description != nil && *existingCol.Comment != *col.Description) {
			updateReq.Comment = col.Description
			needsUpdate = true
		}

		isNullable := !col.IsRequired
		if existingCol.IsNullable != isNullable {
			updateReq.IsNullable = &isNullable
			needsUpdate = true

			// If changing from nullable to NOT NULL, we need to handle existing NULL values
			if col.IsRequired {
				// Get default value for the column type
				defaultValue, defaultValueFormat := s.getDefaultForType(col.Type, col.IsRequired)
				if defaultValue != nil {
					updateReq.DefaultValue = defaultValue
					updateReq.DefaultValueFormat = defaultValueFormat
				}

				// If we're changing from nullable to NOT NULL, we need to update existing NULL values first
				if !existingCol.IsNullable { // was already NOT NULL
					// No action needed
				} else { // was nullable, now making it NOT NULL
					// First, update all NULL values to the default value
					var updateQuery string
					if defaultValue != nil {
						// Format the default value properly for the SQL query
						var defaultValueStr string
						switch defaultValue := defaultValue.(type) {
						case string:
							if defaultValueFormat != nil && *defaultValueFormat == "expression" {
								defaultValueStr = defaultValue
							} else {
								// Escape single quotes in string values
								escapedValue := strings.ReplaceAll(defaultValue, "'", "''")
								defaultValueStr = fmt.Sprintf("'%s'", escapedValue)
							}
						case bool:
							if defaultValue {
								defaultValueStr = "true"
							} else {
								defaultValueStr = "false"
							}
						default:
							defaultValueStr = fmt.Sprintf("%v", defaultValue)
						}
						// Quote table and column names to handle special characters
						updateQuery = fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s IS NULL",
							quoteIdentifier(tableMetadata.PhysicalTableName), quoteIdentifier(col.Name), defaultValueStr, quoteIdentifier(col.Name))
					} else {
						// If no default value, use NULL - this might still cause an error
						// but it's the best we can do
						updateQuery = fmt.Sprintf("UPDATE %s SET %s = NULL WHERE %s IS NULL",
							quoteIdentifier(tableMetadata.PhysicalTableName), quoteIdentifier(col.Name), quoteIdentifier(col.Name))
					}

					// Execute the update query to set NULL values to default values
					_, err := s.sqlBase.ExecuteSQL(ctx, updateQuery, nil, sqlAuditContext(organizationID, dataSource, tableMetadata, accountID, string(model.OperationTypeUpdate)))
					if err != nil {
						return fmt.Errorf("failed to update existing NULL values for column '%s': %w", col.Name, err)
					}
				}
			}
		}

		// Only perform update if something has changed
		if needsUpdate {
			var changes []string
			if updateReq.Name != "" && updateReq.Name != existingCol.Name {
				changes = append(changes, fmt.Sprintf("rename column from '%s' to '%s'", existingCol.Name, updateReq.Name))
			}
			if updateReq.Type != "" {
				changes = append(changes, fmt.Sprintf("change type from '%s' to '%s'", existingCol.NormalizeDataType(), updateReq.Type))
			}
			if updateReq.Comment != existingCol.Comment {
				if updateReq.Comment == nil {
					changes = append(changes, "remove comment")
				} else if existingCol.Comment == nil {
					changes = append(changes, fmt.Sprintf("add comment: '%s'", *updateReq.Comment))
				} else if *existingCol.Comment != *updateReq.Comment {
					changes = append(changes, fmt.Sprintf("update comment from '%s' to '%s'", *existingCol.Comment, *updateReq.Comment))
				}
			}
			if updateReq.IsNullable != nil && *updateReq.IsNullable != existingCol.IsNullable {
				if *updateReq.IsNullable {
					changes = append(changes, "set to nullable")
				} else {
					changes = append(changes, "set to NOT NULL")
				}
			}
			if updateReq.DefaultValue != existingCol.DefaultValue {
				if updateReq.DropDefault {
					changes = append(changes, "drop default value")
				} else {
					changes = append(changes, fmt.Sprintf("set default value to '%v'", updateReq.DefaultValue))
				}
			}

			// Construct detailed log message
			logMessage := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s", tableMetadata.PhysicalTableName, col.Name)
			if len(changes) > 0 {
				logMessage += " (" + strings.Join(changes, ", ") + ")"
			} else {
				logMessage += " (no changes)"
			}

			err = s.auditSQLOperation(ctx, organizationID, dataSourceID, tableID, dataSource.Name, tableMetadata.Name, accountID, string(model.OperationTypeUpdate), logMessage, func() error {
				_, opErr := s.sqlBase.UpdateColumn(ctx, col.ID, updateReq)
				return opErr
			})
			if err != nil {
				return fmt.Errorf("failed to update column '%s': %w", col.Name, err)
			}
		}
	}

	// Create new columns
	for _, col := range columnsToCreate {
		isNullable := !col.IsRequired
		createColReq := sql_base.CreateColumnRequest{
			TableID:    tableMetadata.TableID,
			Name:       col.Name,
			Type:       col.Type,
			IsNullable: &isNullable,
			Comment:    col.Description,
		}

		if col.IsRequired {
			defaultValue, defaultValueFormat := s.getDefaultForType(col.Type, col.IsRequired)
			if defaultValue != nil {
				createColReq.DefaultValue = defaultValue
				createColReq.DefaultValueFormat = defaultValueFormat
			}
		}

		// Log the SQL operation for column creation with detailed information
		createDetails := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s",
			quoteIdentifier(tableMetadata.PhysicalTableName),
			quoteIdentifier(col.Name),
			col.Type)
		if col.Description != nil {
			createDetails += fmt.Sprintf(" COMMENT '%s'", *col.Description)
		}
		if !col.IsRequired {
			createDetails += " NULL"
		} else {
			createDetails += " NOT NULL"
		}
		err = s.auditSQLOperation(ctx, organizationID, dataSourceID, tableID, dataSource.Name, tableMetadata.Name, accountID, string(model.OperationTypeUpdate), createDetails, func() error {
			_, opErr := s.sqlBase.CreateColumn(ctx, createColReq)
			return opErr
		})
		if err != nil {
			return fmt.Errorf("failed to create column '%s': %w", col.Name, err)
		}
	}

	// Update the table's updated_by and updated_at fields
	tableMetadata.UpdatedBy = accountID
	tableMetadata.UpdatedAt = time.Now()

	if err := s.tableRepo.Update(ctx, tableMetadata); err != nil {
		return fmt.Errorf("failed to update table metadata: %w", err)
	}

	// Log the SQL operation for table metadata update
	// s.logSQLOperation(ctx, organizationID, dataSourceID, tableID, dataSource.Name, tableMetadata.Name, accountID, string(model.OperationTypeUpdate),
	// 	fmt.Sprintf("UPDATE data_source_tables SET updated_by = '%s', updated_at = NOW() WHERE id = '%s'", accountID, tableID))

	return nil
}

// GetTableColumns retrieves the columns of a specific table
func (s *dataSourceService) GetTableColumns(ctx context.Context, organizationID, dataSourceID, tableID string, includeSystemFields bool) (dto.GetTableColumnsResponse, error) {
	// Find the data source
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return dto.GetTableColumnsResponse{}, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return dto.GetTableColumnsResponse{}, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}

	// Find the table metadata
	tableMetadata, err := s.tableRepo.FindByID(ctx, tableID)
	if err != nil {
		return dto.GetTableColumnsResponse{}, fmt.Errorf("failed to find table metadata: %w", err)
	}
	if tableMetadata == nil {
		return dto.GetTableColumnsResponse{}, fmt.Errorf("table metadata with id '%s' not found", tableID)
	}

	// Get table information including columns from postgres-meta
	table, err := s.sqlBase.GetTable(ctx, tableMetadata.TableID)
	if err != nil {
		return dto.GetTableColumnsResponse{}, fmt.Errorf("failed to get table: %w", err)
	}

	importColumnMetadata, err := s.getExcelImportColumnMetadata(ctx, organizationID, dataSourceID, tableID)
	if err != nil {
		return dto.GetTableColumnsResponse{}, err
	}

	// Convert to response format
	var resultColumns []dto.TableColumn
	for _, col := range table.Columns {
		// Skip default system columns unless includeSystemFields is true
		isSystemField := col.Name == "id" || col.Name == "uuid" || col.Name == "created_time" || col.Name == "updated_time"
		if isSystemField && !includeSystemFields {
			continue
		}

		resultColumn := dto.TableColumn{
			ID:            col.ID,
			Name:          col.Name,
			Description:   col.Comment,
			Type:          col.NormalizeDataType(), // Use normalized data type
			IsRequired:    !col.IsNullable,
			IsSystemField: isSystemField, // Mark system fields
		}
		if metadata, ok := importColumnMetadata[col.Name]; ok {
			resultColumn.DisplayName = &metadata.DisplayName
			resultColumn.SourceColumnName = &metadata.SourceColumnName
		}
		resultColumns = append(resultColumns, resultColumn)
	}

	return dto.GetTableColumnsResponse{
		Columns: resultColumns,
	}, nil
}

type excelImportColumnMetadata struct {
	DisplayName      string
	SourceColumnName string
}

func (s *dataSourceService) getExcelImportColumnMetadata(ctx context.Context, organizationID, dataSourceID, tableID string) (map[string]excelImportColumnMetadata, error) {
	jobRepo := excelimportrepo.NewJobRepository(s.db)
	job, err := jobRepo.FindLatestByTableID(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("failed to find import job by table: %w", err)
	}
	if job == nil ||
		job.OrganizationID != organizationID ||
		job.DataSourceID != dataSourceID ||
		job.Status == string(dto.ExcelImportStatusFailed) {
		return map[string]excelImportColumnMetadata{}, nil
	}

	var inferredColumns []dto.InferredExcelColumn
	if err := json.Unmarshal(job.SchemaSnapshot, &inferredColumns); err != nil {
		return nil, fmt.Errorf("failed to parse import schema snapshot: %w", err)
	}

	metadata := make(map[string]excelImportColumnMetadata, len(inferredColumns))
	for _, col := range inferredColumns {
		if col.Name == "" || col.SourceColumn == "" {
			continue
		}
		displayName := col.DisplayName
		if displayName == "" {
			displayName = col.SourceColumn
		}
		metadata[col.Name] = excelImportColumnMetadata{
			DisplayName:      displayName,
			SourceColumnName: col.SourceColumn,
		}
	}
	return metadata, nil
}

// AddTableRecords adds records to a table
func (s *dataSourceService) AddTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.AddRecordRequest) (dto.AddRecordResponse, error) {
	// 1. Validate data source and table existence
	dataSource, table, err := s.validateDataSourceAndTable(ctx, organizationID, dataSourceID, tableID)
	if err != nil {
		return dto.AddRecordResponse{}, err
	}

	// 2. Get table structure for the specific table
	tableInfo, err := s.sqlBase.GetTable(ctx, table.TableID)
	if err != nil {
		return dto.AddRecordResponse{}, fmt.Errorf("failed to get table: %w", err)
	}

	// 3. Build column name mapping
	columnMap := make(map[string]sql_base.Column)
	for _, col := range tableInfo.Columns {
		columnMap[col.Name] = col
	}

	// 4. Validate and prepare insert data
	var validRecords []map[string]interface{}
	for _, record := range req.Records {
		validatedRecord, err := s.validateRecord(record, columnMap)
		if err != nil {
			return dto.AddRecordResponse{}, err
		}
		validRecords = append(validRecords, validatedRecord)
	}

	// Calculate row count for quota check
	rowCount := int64(len(validRecords))

	// 5. Check database rows quota if quotaService is available
	if s.quotaService != nil && dataSource.OrganizationID != "" {
		// Parse organization ID
		organizationUUID, err := uuid.Parse(dataSource.OrganizationID)
		if err != nil {
			return dto.AddRecordResponse{}, fmt.Errorf("failed to parse organization ID: %w", err)
		}

		// Check quota
		canProceed, currentUsage, limit, err := s.quotaService.CheckQuota(ctx, organizationUUID, quota_model.ResourceTypeDBRows, rowCount)
		if err != nil {
			return dto.AddRecordResponse{}, fmt.Errorf("failed to check db rows quota: %w", err)
		}

		// If quota exceeded, return error
		if !canProceed {
			return dto.AddRecordResponse{}, fmt.Errorf(response.ErrQuotaDBRowsExceeded.Message, currentUsage, limit, rowCount)
		}
	}

	// 6. Perform batch insert and record usage in transaction
	var affectedRows int64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Perform batch insert
		rows, err := s.batchInsertRecords(ctx, organizationID, auditWorkspaceID(organizationID, dataSource.WorkspaceID), dataSourceID, tableID, dataSource.Name, table.Name, accountID, table.PhysicalTableName, validRecords, columnMap)
		if err != nil {
			return fmt.Errorf("failed to insert records: %w", err)
		}
		affectedRows = rows

		// Record usage history if quotaService is available
		if s.quotaService != nil && dataSource.OrganizationID != "" {
			// Parse IDs
			organizationUUID, _ := uuid.Parse(dataSource.OrganizationID)
			accountUUID, _ := uuid.Parse(accountID)
			workspaceUUID, _ := uuid.Parse(organizationID)

			// Create metadata
			metadata := quota_model.JSONMap{
				"datasource_id":   dataSourceID,
				"datasource_name": dataSource.Name,
				"table_id":        tableID,
				"table_name":      table.Name,
				"rows_affected":   affectedRows,
				"operation":       "insert",
			}

			// Create usage history record
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      organizationUUID,
				AccountID:    accountUUID,
				TenantID:     &workspaceUUID,
				ResourceType: quota_model.ResourceTypeDBRows,
				Delta:        affectedRows,
				ResourceID:   &tableID,
				ResourceName: &table.Name,
				Metadata:     &metadata,
				CreatedAt:    time.Now(),
			}

			// Record usage in transaction
			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record usage: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return dto.AddRecordResponse{}, err
	}

	return dto.AddRecordResponse{AffectedRows: affectedRows}, nil
}

// QueryTableRecords queries data from a table
func (s *dataSourceService) QueryTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, limit, offset int, order string) (dto.QueryRecordResponse, error) {
	// 1. Validate data source and table existence
	dataSource, table, err := s.validateDataSourceAndTable(ctx, organizationID, dataSourceID, tableID)
	if err != nil {
		return dto.QueryRecordResponse{}, err
	}

	// 2. Build query statement (safely handle table name and order fields)
	query, err := s.buildSafeQuery(table.PhysicalTableName, limit, offset, order)
	if err != nil {
		return dto.QueryRecordResponse{}, err
	}

	result, err := s.sqlBase.ExecuteSQL(ctx, query, nil, sqlAuditContext(organizationID, dataSource, table, accountID, string(model.OperationTypeQuery)))
	if err != nil {
		return dto.QueryRecordResponse{}, fmt.Errorf("failed to execute query: %w", err)
	}

	// 3. Build total count query
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdentifier(table.PhysicalTableName))
	countResult, err := s.sqlBase.ExecuteSQL(ctx, countQuery, nil, sqlAuditContext(organizationID, dataSource, table, accountID, string(model.OperationTypeQuery)))
	if err != nil {
		return dto.QueryRecordResponse{}, fmt.Errorf("failed to count records: %w", err)
	}

	var totalNum int64
	if len(countResult.Rows) > 0 && len(countResult.Rows[0]) > 0 {
		// Handle different numeric types returned by database drivers
		switch v := countResult.Rows[0][0].(type) {
		case int64:
			totalNum = v
		case float64:
			totalNum = int64(v)
		case int:
			totalNum = int64(v)
		default:
			totalNum = 0
		}
	}

	// 4. Convert results
	var data []map[string]interface{}
	for _, row := range result.Rows {
		record := make(map[string]interface{})
		for i, col := range result.Columns {
			record[col] = row[i]
		}
		data = append(data, record)
	}

	return dto.QueryRecordResponse{
		HasMore:  offset+limit < int(totalNum),
		TotalNum: totalNum,
		Data:     data,
	}, nil
}

// validateDataSourceAndTable validates data source and table existence
func (s *dataSourceService) validateDataSourceAndTable(ctx context.Context, organizationID, dataSourceID, tableID string) (*model.DataSource, *model.Table, error) {
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return nil, nil, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}

	table, err := s.tableRepo.FindByID(ctx, tableID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find table: %w", err)
	}
	if table == nil {
		return nil, nil, fmt.Errorf("table with id '%s' not found", tableID)
	}

	return dataSource, table, nil
}

// validateRecord validates fields and data types of a single record
func (s *dataSourceService) validateRecord(record map[string]interface{}, columnMap map[string]sql_base.Column) (map[string]interface{}, error) {
	validatedRecord := make(map[string]interface{})

	for field, value := range record {
		// Skip system fields
		if field == "id" || field == "uuid" || field == "created_time" || field == "updated_time" {
			continue
		}

		// Validate field existence
		col, exists := columnMap[field]
		if !exists {
			return nil, fmt.Errorf("field '%s' does not exist in table", field)
		}

		// Validate required fields
		if !col.IsNullable && value == nil {
			return nil, fmt.Errorf("field '%s' is required", field)
		}

		validatedRecord[field] = value
	}

	return validatedRecord, nil
}

// batchInsertRecords performs batch insert operation
func (s *dataSourceService) batchInsertRecords(ctx context.Context, organizationID, workspaceID, dataSourceID, tableID, dataSourceName, tableName, accountID, actualTableName string, records []map[string]interface{}, columnMap map[string]sql_base.Column) (int64, error) {
	if len(records) == 0 {
		return 0, nil
	}

	// Get all field names (excluding system fields) in a consistent order
	var columns []string
	// Use the first record to determine column order
	for field := range records[0] {
		if field != "id" && field != "uuid" && field != "created_time" && field != "updated_time" {
			columns = append(columns, field)
		}
	}

	// Build batch insert query
	var placeholders []string
	var values []interface{}

	for _, record := range records {
		var recordPlaceholders []string
		// Use consistent column order for all records
		for _, col := range columns {
			recordPlaceholders = append(recordPlaceholders, fmt.Sprintf("$%d", len(values)+1))
			values = append(values, record[col])
		}
		placeholders = append(placeholders, "("+strings.Join(recordPlaceholders, ", ")+")")
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		quoteIdentifier(actualTableName),
		strings.Join(quoteIdentifiers(columns), ", "),
		strings.Join(placeholders, ", "),
	)

	guardPolicy := s.guardPolicyForDataSource(ctx, dataSourceID)
	result, err := s.sqlBase.ExecuteSQL(ctx, query, values, &audit.Context{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		DataSourceID:   dataSourceID,
		DataSourceName: dataSourceName,
		TableID:        tableID,
		TableName:      tableName,
		ClientType:     audit.ClientTypeAPI,
		CreatedBy:      accountID,
		OperationType:  string(model.OperationTypeCreate),
		GuardPolicy:    guardPolicy,
	})
	if err != nil {
		return 0, err
	}

	return result.RowsAffected, nil
}

// buildSafeQuery builds a safe query statement
func (s *dataSourceService) buildSafeQuery(tableName string, limit, offset int, order string) (string, error) {
	// Validate order field
	var safeOrder string
	if order == "" {
		safeOrder = "id DESC"
	} else {
		// Simple validation of order field to prevent SQL injection
		if !regexp.MustCompile(`^[\w\s,]+$`).MatchString(order) {
			return "", fmt.Errorf("invalid order parameter")
		}
		safeOrder = order
	}

	// Ensure limit and offset are within reasonable range
	if limit <= 0 {
		limit = 20
	}
	if limit > 1000 {
		limit = 1000
	}

	if offset < 0 {
		offset = 0
	}

	return fmt.Sprintf(
		"SELECT * FROM %s ORDER BY %s LIMIT %d OFFSET %d",
		quoteIdentifier(tableName),
		safeOrder,
		limit,
		offset,
	), nil
}

// UpdateTableRecords updates existing records in a table
func (s *dataSourceService) UpdateTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.UpdateRecordRequest) (dto.UpdateRecordResponse, error) {
	// 1. Validate data source and table existence
	dataSource, table, err := s.validateDataSourceAndTable(ctx, organizationID, dataSourceID, tableID)
	if err != nil {
		return dto.UpdateRecordResponse{}, err
	}

	// 2. Get table structure for the specific table
	tableInfo, err := s.sqlBase.GetTable(ctx, table.TableID)
	if err != nil {
		return dto.UpdateRecordResponse{}, fmt.Errorf("failed to get table: %w", err)
	}

	// 3. Build column name mapping
	columnMap := make(map[string]sql_base.Column)
	for _, col := range tableInfo.Columns {
		columnMap[col.Name] = col
	}

	// 4. Validate and prepare update data
	var validRecords []map[string]interface{}
	for _, record := range req.Records {
		validatedRecord, err := s.validateUpdateRecord(record, columnMap)
		if err != nil {
			return dto.UpdateRecordResponse{}, err
		}
		validRecords = append(validRecords, validatedRecord)
	}

	// 5. Perform batch update
	affectedRows, err := s.batchUpdateRecords(ctx, organizationID, auditWorkspaceID(organizationID, dataSource.WorkspaceID), dataSourceID, tableID, dataSource.Name, table.Name, accountID, table.PhysicalTableName, validRecords, columnMap)
	if err != nil {
		return dto.UpdateRecordResponse{}, fmt.Errorf("failed to update records: %w", err)
	}

	return dto.UpdateRecordResponse{AffectedRows: affectedRows}, nil
}

// validateUpdateRecord validates fields and data types of a single record for update
func (s *dataSourceService) validateUpdateRecord(record map[string]interface{}, columnMap map[string]sql_base.Column) (map[string]interface{}, error) {
	validatedRecord := make(map[string]interface{})

	// Check if id is provided
	id, exists := record["id"]
	if !exists {
		return nil, fmt.Errorf("id is required for update operation")
	}
	validatedRecord["id"] = id

	for field, value := range record {
		// Skip id as it's already handled
		if field == "id" {
			continue
		}

		// Skip system fields that should not be updated directly
		if field == "uuid" || field == "created_time" || field == "updated_time" {
			continue
		}

		// Validate field existence
		col, exists := columnMap[field]
		if !exists {
			return nil, fmt.Errorf("field '%s' does not exist in table", field)
		}

		// Validate required fields
		if !col.IsNullable && value == nil {
			return nil, fmt.Errorf("field '%s' is required", field)
		}

		validatedRecord[field] = value
	}

	return validatedRecord, nil
}

// batchUpdateRecords performs batch update operation
func (s *dataSourceService) batchUpdateRecords(ctx context.Context, organizationID, workspaceID, dataSourceID, tableID, dataSourceName, tableName, accountID, actualTableName string, records []map[string]interface{}, columnMap map[string]sql_base.Column) (int64, error) {
	if len(records) == 0 {
		return 0, nil
	}

	totalAffectedRows := int64(0)
	guardPolicy := s.guardPolicyForDataSource(ctx, dataSourceID)

	// Process each record individually for update
	for _, record := range records {
		id := record["id"]
		delete(record, "id") // Remove id from the update fields

		if len(record) == 0 {
			continue // Nothing to update
		}

		// Build SET clause
		var setClauses []string
		var values []interface{}
		valueIndex := 1

		for field, value := range record {
			// Skip system fields that should not be updated directly
			if field == "uuid" || field == "created_time" {
				continue
			}

			setClauses = append(setClauses, fmt.Sprintf("%s = $%d", quoteIdentifier(field), valueIndex))
			values = append(values, value)
			valueIndex++
		}

		// Add updated_time
		setClauses = append(setClauses, "updated_time = NOW()")

		// Add id to the values for WHERE clause
		values = append(values, id)
		whereClause := fmt.Sprintf("%s = $%d", quoteIdentifier("id"), valueIndex)

		query := fmt.Sprintf(
			"UPDATE %s SET %s WHERE %s",
			quoteIdentifier(actualTableName),
			strings.Join(setClauses, ", "),
			whereClause,
		)

		result, err := s.sqlBase.ExecuteSQL(ctx, query, values, &audit.Context{
			OrganizationID: organizationID,
			WorkspaceID:    workspaceID,
			DataSourceID:   dataSourceID,
			DataSourceName: dataSourceName,
			TableID:        tableID,
			TableName:      tableName,
			ClientType:     audit.ClientTypeAPI,
			CreatedBy:      accountID,
			OperationType:  string(model.OperationTypeUpdate),
			GuardPolicy:    guardPolicy,
		})
		if err != nil {
			return 0, err
		}

		totalAffectedRows += result.RowsAffected
	}

	return totalAffectedRows, nil
}

// DeleteTableRecords deletes records from a table
func (s *dataSourceService) DeleteTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.DeleteRecordRequest) (dto.DeleteRecordResponse, error) {
	// 1. Validate data source and table existence
	dataSource, table, err := s.validateDataSourceAndTable(ctx, organizationID, dataSourceID, tableID)
	if err != nil {
		return dto.DeleteRecordResponse{}, err
	}

	// 2. Validate and prepare delete data
	var validRecords []map[string]interface{}
	for _, record := range req.Records {
		validatedRecord, err := s.validateDeleteRecord(record)
		if err != nil {
			return dto.DeleteRecordResponse{}, err
		}
		validRecords = append(validRecords, validatedRecord)
	}

	// 3. Perform batch delete and record usage in transaction
	var affectedRows int64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Perform batch delete
		rows, err := s.batchDeleteRecords(ctx, organizationID, auditWorkspaceID(organizationID, dataSource.WorkspaceID), dataSourceID, tableID, dataSource.Name, table.Name, accountID, table.PhysicalTableName, validRecords)
		if err != nil {
			return fmt.Errorf("failed to delete records: %w", err)
		}
		affectedRows = rows

		// Record usage history if quotaService is available and rows were deleted
		if s.quotaService != nil && dataSource.OrganizationID != "" && affectedRows > 0 {
			// Parse IDs
			organizationUUID, _ := uuid.Parse(dataSource.OrganizationID)
			accountUUID, _ := uuid.Parse(accountID)
			workspaceUUID, _ := uuid.Parse(organizationID)

			// Create metadata
			metadata := quota_model.JSONMap{
				"datasource_id":   dataSourceID,
				"datasource_name": dataSource.Name,
				"table_id":        tableID,
				"table_name":      table.Name,
				"rows_affected":   affectedRows,
				"operation":       "delete",
			}

			// Create usage history record with negative delta
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      organizationUUID,
				AccountID:    accountUUID,
				TenantID:     &workspaceUUID,
				ResourceType: quota_model.ResourceTypeDBRows,
				Delta:        -affectedRows, // Negative delta for deletion
				ResourceID:   &tableID,
				ResourceName: &table.Name,
				Metadata:     &metadata,
				CreatedAt:    time.Now(),
			}

			// Record usage in transaction
			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record usage: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return dto.DeleteRecordResponse{}, err
	}

	return dto.DeleteRecordResponse{AffectedRows: affectedRows}, nil
}

// validateDeleteRecord validates fields for deleting records
func (s *dataSourceService) validateDeleteRecord(record map[string]interface{}) (map[string]interface{}, error) {
	validatedRecord := make(map[string]interface{})

	// Check if id is provided
	id, exists := record["id"]
	if !exists {
		return nil, fmt.Errorf("id is required for delete operation")
	}
	validatedRecord["id"] = id

	return validatedRecord, nil
}

// batchDeleteRecords performs batch delete operation
func (s *dataSourceService) batchDeleteRecords(ctx context.Context, organizationID, workspaceID, dataSourceID, tableID, dataSourceName, tableName, accountID, actualTableName string, records []map[string]interface{}) (int64, error) {
	if len(records) == 0 {
		return 0, nil
	}

	totalAffectedRows := int64(0)
	guardPolicy := s.guardPolicyForDataSource(ctx, dataSourceID)

	// Process each record individually for delete
	for _, record := range records {
		id := record["id"]

		// Build DELETE query
		query := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", quoteIdentifier(actualTableName), quoteIdentifier("id"))
		values := []interface{}{id}

		result, err := s.sqlBase.ExecuteSQL(ctx, query, values, &audit.Context{
			OrganizationID: organizationID,
			WorkspaceID:    workspaceID,
			DataSourceID:   dataSourceID,
			DataSourceName: dataSourceName,
			TableID:        tableID,
			TableName:      tableName,
			ClientType:     audit.ClientTypeAPI,
			CreatedBy:      accountID,
			OperationType:  string(model.OperationTypeDelete),
			GuardPolicy:    guardPolicy,
		})
		if err != nil {
			return 0, err
		}

		totalAffectedRows += result.RowsAffected
	}

	return totalAffectedRows, nil
}

// Helper functions to create pointers
func stringPtr(s string) *string {
	return &s
}

func int64Ptr(value int64) *int64 {
	return &value
}

func boolPtr(b bool) *bool {
	return &b
}

// quoteIdentifier quotes a PostgreSQL identifier (table name, column name, etc.)
// to handle reserved keywords and special characters
func quoteIdentifier(identifier string) string {
	// Escape double quotes by doubling them
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

// quoteIdentifiers quotes multiple PostgreSQL identifiers
func quoteIdentifiers(identifiers []string) []string {
	quoted := make([]string, len(identifiers))
	for i, id := range identifiers {
		quoted[i] = quoteIdentifier(id)
	}
	return quoted
}

// AnalyzeFileForTable analyzes a file and infers table structure
func (s *dataSourceService) AnalyzeFileForTable(ctx context.Context, dataSourceID, accountID, fileID string, prompt *string, modelSpec *dto.ModelSpec) (dto.AnalyzeFileForTableResponse, error) {
	// Get data source to retrieve organization scope
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return dto.AnalyzeFileForTableResponse{}, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return dto.AnalyzeFileForTableResponse{}, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}

	// Get organization ID from data source
	var organizationID string
	if dataSource.OrganizationID != "" {
		organizationID = dataSource.OrganizationID
	} else {
		return dto.AnalyzeFileForTableResponse{}, fmt.Errorf("data source '%s' has no associated organization_id", dataSourceID)
	}

	var content string

	// Only get file content if fileID is provided
	if fileID != "" {
		// Get file service to retrieve file content
		_, err := s.fileService.GetFileByID(ctx, fileID)
		if err != nil {
			return dto.AnalyzeFileForTableResponse{}, fmt.Errorf("failed to get file: %w", err)
		}

		// Get file content with database ingestion extraction settings.
		extraction, err := s.extractDatabaseIngestionFileContent(ctx, accountID, fileID)
		if err != nil {
			return dto.AnalyzeFileForTableResponse{}, fmt.Errorf("failed to get file content: %w", err)
		}
		content = extraction.Content
	}

	// Analyze file content to infer table structure using data source's organization scope
	columns, err := s.inferTableStructureFromFile(ctx, organizationID, accountID, content, prompt, modelSpec)
	if err != nil {
		return dto.AnalyzeFileForTableResponse{}, fmt.Errorf("failed to infer table structure: %w", err)
	}

	return dto.AnalyzeFileForTableResponse{
		Columns: columns,
		Content: content,
	}, nil
}

// inferTableStructureFromFile infers table structure from file content or user prompt
func (s *dataSourceService) inferTableStructureFromFile(ctx context.Context, tenantID, accountID, content string, userPrompt *string, modelSpec *dto.ModelSpec) ([]dto.TableColumn, error) {
	// Ensure at least one input source is provided
	if content == "" && (userPrompt == nil || (userPrompt != nil && *userPrompt == "")) {
		return nil, fmt.Errorf("either content or user prompt must be provided to generate table structure")
	}

	var columns []dto.TableColumn

	// Get prompt template
	tmpl, err := prompt.GetTemplate(prompt.DatasourceTableAnalysis)
	if err != nil {
		return nil, fmt.Errorf("failed to get prompt template: %w", err)
	}

	// Prepare template data
	templateData := struct {
		Content string
		Prompt  *string
	}{
		Content: content,
		Prompt:  userPrompt,
	}

	// When no file content but user prompt is provided, adjust the content to guide the model
	if content == "" && userPrompt != nil && *userPrompt != "" {
		templateData.Content = "No file content provided. Please generate table structure based solely on the user prompt below."
	}

	// Render prompt
	promptText, err := tmpl.Render(templateData)
	if err != nil {
		return nil, fmt.Errorf("failed to render prompt template: %w", err)
	}

	// Build model slug from modelSpec
	modelSlug := s.getModelSlug(modelSpec)

	// Build chat request
	chatReq := &llmadapter.ChatRequest{
		Model: modelSlug,
		Messages: []llmadapter.Message{
			{Role: "user", Content: promptText},
		},
		Stream: false,
		User:   accountID,
	}

	// Call LLM via client
	resp, err := s.llmClient.Chat(ctx, tenantID, chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to generate table structure with LLM: %w", err)
	}

	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("failed to generate table structure with LLM: empty response")
	}

	generatedContent, _ := resp.Choices[0].Message.Content.(string)
	if generatedContent == "" {
		return nil, fmt.Errorf("failed to generate table structure with LLM: empty result")
	}

	cleanContent, err := s.extractJSONContent(generatedContent)
	if err != nil {
		return nil, fmt.Errorf("failed to extract JSON from LLM response: %w", err)
	}

	type llmTableColumn struct {
		Name        string `json:"Name"`
		Type        string `json:"Type"`
		IsRequired  bool   `json:"IsRequired"`
		Description string `json:"Description"`
	}

	var llmColumns []llmTableColumn
	if err := json.Unmarshal([]byte(cleanContent), &llmColumns); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	for _, llmCol := range llmColumns {
		description := llmCol.Description
		columns = append(columns, dto.TableColumn{
			Name:        llmCol.Name,
			Type:        llmCol.Type,
			IsRequired:  llmCol.IsRequired,
			Description: &description,
		})
	}

	return columns, nil
}

// getModelSlug returns the model name from modelSpec
// The gateway will handle provider selection based on tenant configuration
func (s *dataSourceService) getModelSlug(modelSpec *dto.ModelSpec) string {
	if modelSpec != nil && modelSpec.Name != "" {
		return modelSpec.Name
	}
	return "" // Empty string will use default model in gateway
}

// extractJSONContent extracts pure JSON content from a string that may contain Markdown markers
func (s *dataSourceService) extractJSONContent(content string) (string, error) {
	content = strings.TrimSpace(content)

	// Handle Markdown code block prefix
	if strings.HasPrefix(content, "```json") {
		content = content[7:]
	} else if strings.HasPrefix(content, "```") {
		content = content[3:]
	}

	// Handle Markdown code block suffix
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	// If content is already valid JSON, return directly
	var temp interface{}
	if err := json.Unmarshal([]byte(content), &temp); err == nil {
		return content, nil
	}

	// Try to find the first { or [ and match closing brackets
	startIdx := -1
	for i, c := range content {
		if c == '{' || c == '[' {
			startIdx = i
			break
		}
	}

	if startIdx == -1 {
		return "", fmt.Errorf("could not find json start in the output")
	}

	// Find matching closing brackets
	var stack []rune
	for i := startIdx; i < len(content); i++ {
		switch content[i] {
		case '{', '[':
			stack = append(stack, rune(content[i]))
		case '}', ']':
			if len(stack) == 0 {
				continue
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return content[startIdx : i+1], nil
			}
		}
	}

	return "", fmt.Errorf("could not find matching closing bracket for json")
}

// convertFileContentToRecords converts file content to table records using LLM
func (s *dataSourceService) convertFileContentToRecords(ctx context.Context, tenantID, accountID, content string, columns dto.GetTableColumnsResponse, userPrompt *string, modelSpec *dto.ModelSpec) ([]map[string]interface{}, *dto.FileIngestFieldExtraction, error) {
	columnSchema, err := buildFileConversionColumnSchema(columns)
	if err != nil {
		return nil, nil, err
	}

	// Get prompt template
	tmpl, err := prompt.GetTemplate(prompt.DatasourceFileConversion)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get prompt template: %w", err)
	}

	// Prepare template data
	templateData := struct {
		Columns string
		Content string
		Prompt  *string
	}{
		Columns: columnSchema,
		Content: content,
		Prompt:  userPrompt,
	}

	// Render prompt
	promptText, err := tmpl.Render(templateData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to render prompt template: %w", err)
	}

	// Build model slug from modelSpec
	modelSlug := s.getModelSlug(modelSpec)

	// Build chat request
	chatReq := &llmadapter.ChatRequest{
		Model: modelSlug,
		Messages: []llmadapter.Message{
			{Role: "user", Content: promptText},
		},
		Stream: false,
		User:   accountID,
	}

	// Call LLM via client
	resp, err := s.llmClient.Chat(ctx, tenantID, chatReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert content with LLM: %w", err)
	}

	if resp == nil || len(resp.Choices) == 0 {
		return nil, nil, fmt.Errorf("failed to convert content with LLM: empty response")
	}

	generatedContent, _ := resp.Choices[0].Message.Content.(string)
	if generatedContent == "" {
		return nil, nil, fmt.Errorf("failed to convert content with LLM: empty result")
	}

	// Extract pure JSON content from the LLM response
	cleanContent, err := s.extractJSONContent(generatedContent)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract JSON from LLM response: %w", err)
	}

	records, fieldExtraction, err := normalizeFileConversionOutput(cleanContent, columns)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return records, fieldExtraction, nil
}

func (s *dataSourceService) prepareDatabaseIngestionTable(ctx context.Context, organizationID, tableID string) (databaseIngestionTableContext, error) {
	table, err := s.tableRepo.FindByID(ctx, tableID)
	if err != nil {
		return databaseIngestionTableContext{}, fmt.Errorf("failed to find table: %w", err)
	}
	if table == nil {
		return databaseIngestionTableContext{}, fmt.Errorf("table with id '%s' not found", tableID)
	}

	dataSourceID := table.DataSourceID
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return databaseIngestionTableContext{}, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil || dataSource.WorkspaceID == nil {
		return databaseIngestionTableContext{}, fmt.Errorf("data source '%s' has no associated workspace scope", dataSourceID)
	}

	columns, err := s.GetTableColumns(ctx, organizationID, dataSourceID, tableID, false)
	if err != nil {
		return databaseIngestionTableContext{}, fmt.Errorf("failed to get table columns: %w", err)
	}

	return databaseIngestionTableContext{
		DataSourceID:      dataSourceID,
		LLMOrganizationID: dataSource.OrganizationID,
		Columns:           columns,
	}, nil
}

// ParseFileForTableIngest parses a file into text content for table ingestion review.
func (s *dataSourceService) ParseFileForTableIngest(ctx context.Context, organizationID, accountID string, req dto.ParseFileForTableIngestRequest) (dto.ParseFileForTableIngestResponse, error) {
	if _, err := s.prepareDatabaseIngestionTable(ctx, organizationID, req.TableID); err != nil {
		return dto.ParseFileForTableIngestResponse{}, err
	}
	result := s.parseDatabaseIngestionFileForTable(ctx, accountID, req.FileID)
	logDatabaseIngestionFileParseResult(ctx, req.TableID, result)
	return result, nil
}

// ExtractTextToTableRecords recognizes table records from previously parsed content.
func (s *dataSourceService) ExtractTextToTableRecords(ctx context.Context, organizationID, accountID string, req dto.ExtractTextToTableRecordsRequest) (dto.ExtractTextToTableRecordsResponse, error) {
	tableCtx, err := s.prepareDatabaseIngestionTable(ctx, organizationID, req.TableID)
	if err != nil {
		return dto.ExtractTextToTableRecordsResponse{}, err
	}
	result := s.extractDatabaseIngestionTextToRecords(ctx, tableCtx, accountID, req)
	logDatabaseIngestionTextRecognitionResult(ctx, req.TableID, req.Model, result)
	return result, nil
}

// IngestFileToTable ingests file content into a table.
func (s *dataSourceService) IngestFileToTable(ctx context.Context, organizationID, accountID string, req dto.IngestFileToTableRequest) (dto.IngestFileToTableResponse, error) {
	tableCtx, err := s.prepareDatabaseIngestionTable(ctx, organizationID, req.TableID)
	if err != nil {
		return dto.IngestFileToTableResponse{}, err
	}

	result := s.processDatabaseIngestionFile(ctx, tableCtx, accountID, req.TableID, req.FileID, req.Prompt, req.Model)
	logDatabaseIngestionFileResult(ctx, req.TableID, result)
	return dto.IngestFileToTableResponse{
		FileID:          result.FileID,
		FileName:        result.FileName,
		Records:         result.Records,
		Columns:         tableCtx.Columns.Columns,
		Message:         result.Message,
		Content:         result.Content,
		Extraction:      result.Extraction,
		FieldExtraction: result.FieldExtraction,
		Stage:           result.Stage,
		Error:           result.Error,
	}, nil
}

func (s *dataSourceService) parseDatabaseIngestionFileForTable(ctx context.Context, accountID, fileID string) dto.ParseFileForTableIngestResponse {
	fileInfo, err := s.fileService.GetFileByID(ctx, fileID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get file info: %v", err)
		return dto.ParseFileForTableIngestResponse{
			FileID:  fileID,
			Message: "",
			Stage:   fileIngestStageParse,
			Error:   &errMsg,
		}
	}

	extraction, err := s.extractDatabaseIngestionFileInfoContent(ctx, accountID, fileInfo)
	content := extraction.Content
	if err != nil {
		errMsg := fmt.Sprintf("failed to get file content: %v", err)
		return dto.ParseFileForTableIngestResponse{
			FileID:     fileID,
			FileName:   fileInfo.Name,
			Message:    "",
			Content:    content,
			Extraction: fileIngestExtractionInfo(extraction, content),
			Stage:      fileIngestStageParse,
			Error:      &errMsg,
		}
	}

	return dto.ParseFileForTableIngestResponse{
		FileID:     fileID,
		FileName:   fileInfo.Name,
		Message:    "Successfully parsed file content",
		Content:    content,
		Extraction: fileIngestExtractionInfo(extraction, content),
		Stage:      fileIngestStageParse,
	}
}

func (s *dataSourceService) extractDatabaseIngestionTextToRecords(ctx context.Context, tableCtx databaseIngestionTableContext, accountID string, req dto.ExtractTextToTableRecordsRequest) dto.ExtractTextToTableRecordsResponse {
	content := strings.TrimSpace(req.Content)
	if content == "" {
		errMsg := "parsed content is empty"
		return dto.ExtractTextToTableRecordsResponse{
			FileID:      req.FileID,
			Records:     nil,
			Columns:     tableCtx.Columns.Columns,
			Message:     "",
			ContentHash: req.ContentHash,
			Stage:       fileIngestStageRecognition,
			Error:       &errMsg,
		}
	}

	contentHash := fileIngestContentHash(req.Content)
	records, fieldExtraction, err := s.convertFileContentToRecords(ctx, tableCtx.LLMOrganizationID, accountID, req.Content, tableCtx.Columns, req.Prompt, req.Model)
	if err != nil {
		errMsg := fmt.Sprintf("failed to convert file content to records: %v", err)
		return dto.ExtractTextToTableRecordsResponse{
			FileID:      req.FileID,
			Records:     nil,
			Columns:     tableCtx.Columns.Columns,
			Message:     "",
			ContentHash: contentHash,
			Stage:       fileIngestStageRecognition,
			Error:       &errMsg,
		}
	}

	message := fmt.Sprintf("Successfully parsed %d records from file", len(records))
	if len(records) == 0 {
		message = "no records recognized from file"
	}
	return dto.ExtractTextToTableRecordsResponse{
		FileID:          req.FileID,
		Records:         records,
		Columns:         tableCtx.Columns.Columns,
		Message:         message,
		FieldExtraction: fieldExtraction,
		ContentHash:     contentHash,
		Stage:           fileIngestStageRecognition,
	}
}

func (s *dataSourceService) processDatabaseIngestionFile(ctx context.Context, tableCtx databaseIngestionTableContext, accountID, tableID, fileID string, prompt *string, modelSpec *dto.ModelSpec) dto.FileIngestResult {
	parseResult := s.parseDatabaseIngestionFileForTable(ctx, accountID, fileID)
	logDatabaseIngestionFileParseResult(ctx, tableID, parseResult)
	if parseResult.Error != nil {
		return dto.FileIngestResult{
			FileID:     parseResult.FileID,
			FileName:   parseResult.FileName,
			Records:    nil,
			Message:    "",
			Content:    parseResult.Content,
			Extraction: parseResult.Extraction,
			Stage:      fileIngestStageParse,
			Error:      parseResult.Error,
		}
	}

	recognitionResult := s.extractDatabaseIngestionTextToRecords(ctx, tableCtx, accountID, dto.ExtractTextToTableRecordsRequest{
		FileID:  fileID,
		TableID: tableID,
		Content: parseResult.Content,
		ContentHash: func() string {
			if parseResult.Extraction == nil {
				return ""
			}
			return parseResult.Extraction.ContentHash
		}(),
		Prompt: prompt,
		Model:  modelSpec,
	})
	logDatabaseIngestionTextRecognitionResult(ctx, tableID, modelSpec, recognitionResult)
	extraction := databaseIngestionExtractionInfoFromDTO(parseResult.Extraction)
	if recognitionResult.Error != nil {
		updateFileIngestAttemptResult(&extraction, fileIngestAttemptMethodForExtraction(extraction), databaseIngestionAttemptResultError, *recognitionResult.Error, 0)
		return dto.FileIngestResult{
			FileID:     fileID,
			FileName:   parseResult.FileName,
			Records:    nil,
			Message:    "",
			Content:    parseResult.Content,
			Extraction: fileIngestExtractionInfo(extraction, parseResult.Content),
			Stage:      fileIngestStageRecognition,
			Error:      recognitionResult.Error,
		}
	}

	if len(recognitionResult.Records) > 0 {
		updateFileIngestAttemptResult(&extraction, fileIngestAttemptMethodForExtraction(extraction), databaseIngestionAttemptResultRecords, "", len(recognitionResult.Records))
	} else {
		updateFileIngestAttemptResult(&extraction, fileIngestAttemptMethodForExtraction(extraction), databaseIngestionAttemptResultNoRecords, "no_records_recognized", 0)
	}

	return normalizeFileIngestResult(dto.FileIngestResult{
		FileID:          fileID,
		FileName:        parseResult.FileName,
		Records:         recognitionResult.Records,
		Message:         recognitionResult.Message,
		Content:         parseResult.Content,
		Extraction:      fileIngestExtractionInfo(extraction, parseResult.Content),
		FieldExtraction: recognitionResult.FieldExtraction,
		Stage:           fileIngestStageRecognition,
		Error:           nil,
	})
}

func databaseIngestionExtractionInfoFromDTO(info *dto.FileIngestExtractionInfo) databaseIngestionExtractionResult {
	if info == nil {
		return databaseIngestionExtractionResult{}
	}
	return databaseIngestionExtractionResult{
		PrimaryStrategy: info.PrimaryStrategy,
		ActualStrategy:  info.ActualStrategy,
		FallbackReason:  info.FallbackReason,
		SourceType:      info.SourceType,
		Attempts:        append([]dto.FileIngestAttempt(nil), info.Attempts...),
	}
}

func normalizeFileIngestResult(result dto.FileIngestResult) dto.FileIngestResult {
	if result.Error == nil && len(result.Records) == 0 {
		if strings.TrimSpace(result.Content) == "" {
			errMsg := "no records recognized from file"
			result.Error = &errMsg
			result.Message = ""
		} else if strings.TrimSpace(result.Message) == "" ||
			strings.HasPrefix(result.Message, "Successfully parsed 0 records") {
			result.Message = "no records recognized from file"
		}
	}
	return result
}

func logDatabaseIngestionFileParseResult(ctx context.Context, tableID string, result dto.ParseFileForTableIngestResponse) {
	contentHash := ""
	sourceType := ""
	var attempts []dto.FileIngestAttempt
	if result.Extraction != nil {
		contentHash = result.Extraction.ContentHash
		sourceType = result.Extraction.SourceType
		attempts = result.Extraction.Attempts
	}
	errorMessage := ""
	if result.Error != nil {
		errorMessage = *result.Error
	}
	logger.InfoContext(ctx, "database ingestion file parse stage completed",
		"table_id", tableID,
		"file_id", result.FileID,
		"file_name", result.FileName,
		"stage", fileIngestStageParse,
		"error", errorMessage,
		"content_chars", len([]rune(result.Content)),
		"content_hash", contentHash,
		"source_type", sourceType,
		"attempts", attempts,
	)
}

func logDatabaseIngestionTextRecognitionResult(ctx context.Context, tableID string, modelSpec *dto.ModelSpec, result dto.ExtractTextToTableRecordsResponse) {
	errorMessage := ""
	if result.Error != nil {
		errorMessage = *result.Error
	}
	modelName := ""
	modelProvider := ""
	if modelSpec != nil {
		modelName = modelSpec.Name
		modelProvider = modelSpec.Provider
	}
	logger.InfoContext(ctx, "database ingestion text recognition stage completed",
		"table_id", tableID,
		"file_id", result.FileID,
		"stage", fileIngestStageRecognition,
		"error", errorMessage,
		"record_count", len(result.Records),
		"content_hash", result.ContentHash,
		"model_provider", modelProvider,
		"model_name", modelName,
	)
}

func logDatabaseIngestionFileResult(ctx context.Context, tableID string, result dto.FileIngestResult) {
	primaryStrategy := ""
	actualStrategy := ""
	fallbackReason := ""
	sourceType := ""
	contentHash := ""
	var attempts []dto.FileIngestAttempt
	if result.Extraction != nil {
		primaryStrategy = result.Extraction.PrimaryStrategy
		actualStrategy = result.Extraction.ActualStrategy
		fallbackReason = result.Extraction.FallbackReason
		sourceType = result.Extraction.SourceType
		contentHash = result.Extraction.ContentHash
		attempts = result.Extraction.Attempts
	}
	if result.Error == nil && len(result.Records) > 0 {
		logger.InfoContext(ctx, "database ingestion file produced importable records",
			"table_id", tableID,
			"file_id", result.FileID,
			"file_name", result.FileName,
			"stage", result.Stage,
			"record_count", len(result.Records),
			"content_chars", len([]rune(result.Content)),
			"content_hash", contentHash,
			"source_type", sourceType,
			"primary_strategy", primaryStrategy,
			"actual_strategy", actualStrategy,
			"fallback_reason", fallbackReason,
			"attempts", attempts,
		)
		return
	}

	errorMessage := ""
	if result.Error != nil {
		errorMessage = *result.Error
	}
	logger.WarnContext(ctx, "database ingestion file produced no importable records",
		"table_id", tableID,
		"file_id", result.FileID,
		"file_name", result.FileName,
		"stage", result.Stage,
		"error", errorMessage,
		"record_count", len(result.Records),
		"content_chars", len([]rune(result.Content)),
		"content_hash", contentHash,
		"source_type", sourceType,
		"primary_strategy", primaryStrategy,
		"actual_strategy", actualStrategy,
		"fallback_reason", fallbackReason,
		"attempts", attempts,
	)
}

// BatchIngestFileToTable processes multiple files and converts their content to table records
func (s *dataSourceService) BatchIngestFileToTable(ctx context.Context, organizationID, accountID string, req dto.BatchIngestFileToTableRequest) (dto.BatchIngestFileToTableResponse, error) {
	tableCtx, err := s.prepareDatabaseIngestionTable(ctx, organizationID, req.TableID)
	if err != nil {
		return dto.BatchIngestFileToTableResponse{}, err
	}

	// Process each file with concurrency control
	results := make(map[string]dto.FileIngestResult)
	successCount := 0

	// Use a semaphore to limit concurrency to 2 goroutines
	const maxConcurrency = 2
	semaphore := make(chan struct{}, maxConcurrency)

	// Channel to collect results
	type result struct {
		fileID string
		result dto.FileIngestResult
		err    error
	}
	resultChan := make(chan result, len(req.FileIDs))

	// Process files concurrently
	for _, fileID := range req.FileIDs {
		// Acquire semaphore
		semaphore <- struct{}{}

		go func(id string) {
			// Release semaphore when done
			defer func() { <-semaphore }()

			fileResult := s.processDatabaseIngestionFile(ctx, tableCtx, accountID, req.TableID, id, req.Prompt, req.Model)
			resultChan <- result{
				fileID: id,
				result: fileResult,
				err:    nil,
			}
		}(fileID)
	}

	// Close the result channel after all goroutines are launched
	// Wait for all goroutines to finish by filling the semaphore
	for i := 0; i < cap(semaphore); i++ {
		semaphore <- struct{}{}
	}

	// Now we know all goroutines have finished, close the result channel
	close(resultChan)

	// Collect results
	for res := range resultChan {
		fileResult := normalizeFileIngestResult(res.result)
		results[res.fileID] = fileResult
		if fileResult.Error == nil && len(fileResult.Records) > 0 {
			successCount++
		}
		logDatabaseIngestionFileResult(ctx, req.TableID, fileResult)
	}

	totalCount := len(req.FileIDs)
	failedCount := totalCount - successCount
	message := fmt.Sprintf("Successfully processed %d out of %d files", successCount, totalCount)
	switch {
	case totalCount == 0:
		message = "No files were provided"
	case failedCount == 0:
		message = fmt.Sprintf("Successfully processed %d out of %d files", successCount, totalCount)
	case successCount == 0:
		message = fmt.Sprintf("Failed to process %d files", totalCount)
	default:
		message = fmt.Sprintf("Processed %d out of %d files; %d failed", successCount, totalCount, failedCount)
	}

	return dto.BatchIngestFileToTableResponse{
		Results:      results,
		Columns:      tableCtx.Columns.Columns,
		Message:      message,
		TotalCount:   totalCount,
		SuccessCount: successCount,
		FailedCount:  failedCount,
	}, nil
}

// GetTablePrompt gets a prompt by table ID or returns a default one if it doesn't exist
func (s *dataSourceService) GetTablePrompt(ctx context.Context, tableID string, lang string) (*model.TablePrompt, error) {
	// Check if table exists
	table, err := s.tableRepo.FindByID(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("failed to find table: %w", err)
	}
	if table == nil {
		return nil, fmt.Errorf("%w: %s", errDataSourceTableNotFound, tableID)
	}

	// Try to find existing prompt
	p, err := s.promptRepo.FindByTableID(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("failed to find prompt: %w", err)
	}

	// If prompt exists, return it
	if p != nil {
		return p, nil
	}

	// Use the provided language parameter directly
	// Get the appropriate default user ingest prompt template based on language
	var defaultUserPromptTmpl *prompt.Template
	switch lang {
	case "zh":
		defaultUserPromptTmpl, err = prompt.GetTemplate(prompt.DatasourceDefaultUserIngestZh)
	default:
		defaultUserPromptTmpl, err = prompt.GetTemplate(prompt.DatasourceDefaultUserIngestEn)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get default user ingest prompt template: %w", err)
	}

	// Render the default user prompt
	defaultUserPrompt, err := defaultUserPromptTmpl.Render(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to render default user prompt: %w", err)
	}

	defaultPrompt := &model.TablePrompt{
		TableID:   tableID,
		Prompt:    defaultUserPrompt,
		CreatedBy: "", // Empty as this is a default prompt
		UpdatedBy: "", // Empty as this is a default prompt
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return defaultPrompt, nil
}

// UpsertTablePrompt updates a table prompt, or creates it if it doesn't exist
func (s *dataSourceService) UpsertTablePrompt(ctx context.Context, tableID string, req dto.UpdateTablePromptRequest) (*model.TablePrompt, error) {
	// Check if table exists
	table, err := s.tableRepo.FindByID(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("failed to find table: %w", err)
	}
	if table == nil {
		return nil, fmt.Errorf("table with id '%s' not found", tableID)
	}

	// Check if prompt exists
	prompt, err := s.promptRepo.FindByTableID(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("failed to find prompt: %w", err)
	}

	// If prompt doesn't exist, create a new one
	if prompt == nil {
		newPrompt := &model.TablePrompt{
			TableID:   tableID,
			Prompt:    req.Prompt,
			CreatedBy: req.UpdatedBy,
			UpdatedBy: req.UpdatedBy,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := s.promptRepo.Create(ctx, newPrompt); err != nil {
			return nil, fmt.Errorf("failed to create prompt: %w", err)
		}

		return newPrompt, nil
	}

	// Update existing prompt
	prompt.Prompt = req.Prompt
	prompt.UpdatedBy = req.UpdatedBy
	prompt.UpdatedAt = time.Now()

	if err := s.promptRepo.Update(ctx, prompt); err != nil {
		return nil, fmt.Errorf("failed to update prompt: %w", err)
	}

	return prompt, nil
}

// DeleteTablePrompt deletes a table prompt by table ID
func (s *dataSourceService) DeleteTablePrompt(ctx context.Context, tableID string) error {
	// Check if table exists
	table, err := s.tableRepo.FindByID(ctx, tableID)
	if err != nil {
		return fmt.Errorf("failed to find table: %w", err)
	}
	if table == nil {
		return fmt.Errorf("table with id '%s' not found", tableID)
	}

	// Delete prompt by table ID
	err = s.promptRepo.DeleteByTableID(ctx, tableID)
	if err != nil {
		return fmt.Errorf("failed to delete prompt: %w", err)
	}

	return nil
}

// UpdateDataSource updates an existing data source
func (s *dataSourceService) UpdateDataSource(ctx context.Context, organizationID, id, accountID string, req dto.UpdateDataSourceRequest) (*dto.DataSourceResponse, error) {
	// Find the data source
	dataSource, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return nil, fmt.Errorf("data source with id '%s' not found", id)
	}

	// Check permissions - only the creator or an admin can update the data source
	if dataSource.CreatedBy != accountID {
		// Check if the user is an admin in this tenant
		isAdmin, err := s.accountService.CheckOrganizationpAdminByWorkspace(ctx, organizationID, accountID)
		if err != nil {
			return nil, fmt.Errorf("failed to check admin status: %w", err)
		}
		if !isAdmin {
			return nil, fmt.Errorf("only the creator or an admin can update the data source")
		}
	}

	// Update fields if provided
	if req.Name != nil {
		// Check if another data source with the same name already exists
		existing, err := s.repo.FindByOrganizationAndName(ctx, organizationID, *req.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing data source: %w", err)
		}
		if existing != nil && existing.ID != id {
			return nil, fmt.Errorf("data source with name '%s' already exists", *req.Name)
		}
		dataSource.Name = *req.Name
	}

	if req.Description != nil {
		dataSource.Description = *req.Description
	}

	if req.Permission != nil {
		dataSource.Permission = *req.Permission
	}

	if req.WorkspaceID != nil {
		dataSource.WorkspaceID = req.WorkspaceID
	}

	if req.IconType != nil {
		dataSource.IconType = req.IconType
	}

	if req.Icon != nil {
		dataSource.Icon = req.Icon
	}

	if req.IconBackground != nil {
		dataSource.IconBackground = req.IconBackground
	}

	// Update timestamps
	dataSource.UpdatedBy = accountID
	dataSource.UpdatedAt = time.Now()

	// Save updated data source
	if err := s.repo.Update(ctx, dataSource); err != nil {
		return nil, fmt.Errorf("failed to update data source: %w", err)
	}

	return dto.ConvertDataSourceModelToResponse(dataSource), nil
}

func (s *dataSourceService) normalizeDataType(dataType string) string {
	switch dataType {
	case "double precision", "float8":
		return "double"
	case "real", "float4":
		return "float"
	case "integer", "int", "int4":
		return "int"
	case "bigint", "int8":
		return "bigint"
	case "smallint", "int2":
		return "smallint"
	case "boolean", "bool":
		return "boolean"
	case "character varying", "varchar":
		return "varchar"
	case "timestamp without time zone":
		return "timestamp"
	case "timestamp with time zone":
		return "timestamptz"
	default:
		return dataType
	}
}

func (s *dataSourceService) getDefaultForType(colType string, isRequired bool) (interface{}, *string) {
	if !isRequired {
		return nil, nil
	}

	normalizedType := s.normalizeDataType(colType)

	switch normalizedType {
	case "varchar", "text", "char", "character":
		return "", nil
	case "int", "integer", "smallint", "bigint":
		return 0, nil
	case "float", "double", "real", "numeric", "decimal", "double precision":
		return 0.0, nil
	case "boolean", "bool":
		return false, nil
	case "timestamp", "timestamptz", "date", "time":
		expr := "expression"
		return "CURRENT_TIMESTAMP", &expr
	case "uuid":
		expr := "expression"
		return "gen_random_uuid()", &expr
	case "json", "jsonb":
		return "{}", nil
	default:
		if strings.HasSuffix(normalizedType, "[]") {
			return "{}", nil
		}
		return nil, nil
	}
}

// logSQLOperation logs the SQL statement for audit purposes
func (s *dataSourceService) logSQLOperation(ctx context.Context, organizationID, dataSourceID, tableID, dataSourceName, tableName, accountID, operation, sqlStatement string) error {
	now := time.Now()
	return s.logSQLOperationWithResult(ctx, organizationID, dataSourceID, tableID, dataSourceName, tableName, accountID, operation, sqlStatement, now, now, nil, guard.Result{}, false)
}

func (s *dataSourceService) auditSQLOperation(ctx context.Context, organizationID, dataSourceID, tableID, dataSourceName, tableName, accountID, operation, sqlStatement string, execute func() error) error {
	start := time.Now()
	guardResult, guarded, guardErr := s.evaluateGuardForAuditedSQL(ctx, dataSourceID, sqlStatement)
	var err error
	if guardErr != nil {
		err = guardErr
	} else {
		err = execute()
	}
	end := time.Now()
	if logErr := s.logSQLOperationWithResult(ctx, organizationID, dataSourceID, tableID, dataSourceName, tableName, accountID, operation, sqlStatement, start, end, err, guardResult, guarded); logErr != nil {
		logger.ErrorContext(ctx, "failed to audit sql operation", "data_source_id", dataSourceID, "table_id", tableID, logErr)
	}
	return err
}

func (s *dataSourceService) evaluateGuardForAuditedSQL(ctx context.Context, dataSourceID, sqlStatement string) (guard.Result, bool, error) {
	if dataSourceID == "" || s.repo == nil {
		return guard.Result{}, false, nil
	}
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return guard.Result{}, false, fmt.Errorf("failed to find data source for sql guard: %w", err)
	}
	if dataSource == nil {
		return guard.Result{}, false, fmt.Errorf("data source with id '%s' not found for sql guard", dataSourceID)
	}
	policy, err := guard.ParsePolicyJSON([]byte(dataSource.GuardPolicy))
	if err != nil {
		return guard.Result{}, false, fmt.Errorf("failed to parse sql guard policy: %w", err)
	}
	result := guard.Check(sqlStatement, policy)
	if result.Action == guard.ActionDeny {
		return result, true, &guard.DeniedError{Result: result}
	}
	return result, true, nil
}

func (s *dataSourceService) logSQLOperationWithResult(ctx context.Context, organizationID, dataSourceID, tableID, dataSourceName, tableName, accountID, operation, sqlStatement string, start, end time.Time, execErr error, guardResult guard.Result, guarded bool) error {
	now := time.Now()
	workspaceID := organizationID
	if dataSourceID != "" && s.repo != nil {
		if dataSource, err := s.repo.FindByID(ctx, dataSourceID); err == nil && dataSource != nil {
			workspaceID = auditWorkspaceID(organizationID, dataSource.WorkspaceID)
		}
	}
	durationMS := end.Sub(start).Milliseconds()
	status := string(model.OperationStatusSuccess)
	var errorMessage *string
	if execErr != nil {
		status = string(model.OperationStatusFailed)
		msg := execErr.Error()
		errorMessage = &msg
	}

	// Create operation log entry
	log := &model.DataSourceSQLOperation{
		OrganizationID: organizationID,
		WorkspaceID:    &workspaceID,
		DataSourceID:   dataSourceID,
		TableID:        nil, // Initialize as nil
		TableName:      nil, // Initialize as nil
		DataSourceName: nil, // Initialize as nil
		SqlStatement:   sqlStatement,
		OperationType:  operation,
		ClientType:     string(audit.ClientTypeAPI),
		DurationMS:     &durationMS,
		ErrorMessage:   errorMessage,
		ExecutedAt:     &start,
		StartTime:      start,
		EndTime:        end,
		Status:         status,
		CreatedBy:      accountID,
		CreatedAt:      now,
	}
	if guarded {
		verdict := string(guardResult.Verdict)
		action := string(guardResult.Action)
		log.GuardVerdict = &verdict
		log.GuardAction = &action
		if reasons, err := json.Marshal(guardResult.Reasons); err == nil {
			log.GuardReasons = reasons
		}
		if policy, err := json.Marshal(guardResult.Policy); err == nil {
			log.GuardPolicy = policy
		}
	}

	// Only set TableID if it's not empty
	if tableID != "" {
		log.TableID = &tableID
	}

	// Only set TableName if it's not empty
	if tableName != "" {
		log.TableName = &tableName
	}

	// Only set DataSourceName if it's not empty
	if dataSourceName != "" {
		log.DataSourceName = &dataSourceName
	}

	// Save to database
	if err := s.sqlOperationRepo.Create(ctx, log); err != nil {
		// Log the error but don't fail the operation
		logger.ErrorContext(ctx, "failed to log sql operation", "data_source_id", dataSourceID, "table_id", tableID, err)
		return err
	}

	return nil
}

// ListOperationLogsByDataSourceID lists operation logs for a specific data source
func (s *dataSourceService) ListOperationLogsByDataSourceID(ctx context.Context, organizationID, dataSourceID string, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	// Validate data source exists and belongs to organization
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return nil, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}
	if dataSource.OrganizationID != organizationID {
		return nil, fmt.Errorf("data source does not belong to organization")
	}

	// Get logs from repository
	logs, err := s.sqlOperationRepo.ListByDataSourceID(ctx, dataSourceID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list operation logs: %w", err)
	}

	return logs, nil
}

// ListOperationLogsByDataSourceIDWithFilters lists operation logs for a specific data source with filters
func (s *dataSourceService) ListOperationLogsByDataSourceIDWithFilters(ctx context.Context, organizationID, dataSourceID string, filters dto.SQLOperationFilter, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	// Validate data source exists and belongs to organization
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return nil, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}
	if dataSource.OrganizationID != organizationID {
		return nil, fmt.Errorf("data source does not belong to organization")
	}

	// Convert DTO filter to repository filter
	repoFilter := dto.SQLOperationFilter{
		TableID:       filters.TableID,
		CreatedBy:     filters.CreatedBy,
		OperationType: filters.OperationType,
		Status:        filters.Status,
	}

	// Use time values directly as they are already time.Time
	repoFilter.CreatedAtGTE = filters.CreatedAtGTE
	repoFilter.CreatedAtLTE = filters.CreatedAtLTE

	// Get logs from repository
	logs, err := s.sqlOperationRepo.ListByDataSourceIDWithFilters(ctx, dataSourceID, repoFilter, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list operation logs: %w", err)
	}

	return logs, nil
}

// CountOperationLogsByDataSourceID counts operation logs for a specific data source
func (s *dataSourceService) CountOperationLogsByDataSourceID(ctx context.Context, organizationID, dataSourceID string) (int64, error) {
	// Validate data source exists and belongs to organization
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return 0, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}
	if dataSource.OrganizationID != organizationID {
		return 0, fmt.Errorf("data source does not belong to organization")
	}

	// Get count from repository
	count, err := s.sqlOperationRepo.CountByDataSourceID(ctx, dataSourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to count operation logs: %w", err)
	}

	return count, nil
}

func (s *dataSourceService) ListSQLAuditByWorkspace(ctx context.Context, organizationID, workspaceID string, filters dto.SQLAuditFilter, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	logs, err := s.sqlOperationRepo.ListAuditByWorkspace(ctx, organizationID, workspaceID, filters, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list sql audit records: %w", err)
	}
	return logs, nil
}

func (s *dataSourceService) CountSQLAuditByWorkspace(ctx context.Context, organizationID, workspaceID string, filters dto.SQLAuditFilter) (int64, error) {
	count, err := s.sqlOperationRepo.CountAuditByWorkspace(ctx, organizationID, workspaceID, filters)
	if err != nil {
		return 0, fmt.Errorf("failed to count sql audit records: %w", err)
	}
	return count, nil
}

func (s *dataSourceService) GetSQLAuditDetail(ctx context.Context, organizationID, workspaceID, operationID string) (*model.DataSourceSQLOperation, error) {
	record, err := s.sqlOperationRepo.FindAuditByWorkspaceAndID(ctx, organizationID, workspaceID, operationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sql audit record: %w", err)
	}
	return record, nil
}

// GenerateTableTemplateExcel generates an Excel template for a table
func (s *dataSourceService) GenerateTableTemplateExcel(ctx context.Context, organizationID, dataSourceID, tableID string) ([]byte, error) {
	// Get table columns without system fields
	columnsResp, err := s.GetTableColumns(ctx, organizationID, dataSourceID, tableID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get table columns: %w", err)
	}

	if len(columnsResp.Columns) == 0 {
		return nil, fmt.Errorf("no columns found for the table")
	}

	// Create Excel file
	f := excelize.NewFile()
	defer func() {
		_ = f.Close()
	}()

	// Set default sheet name
	sheetName := "Sheet1"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	// Write column headers (first row)
	for colIdx, col := range columnsResp.Columns {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		f.SetCellValue(sheetName, cell, col.Name)
	}

	// Write example values (second row)
	for colIdx, col := range columnsResp.Columns {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 2)
		f.SetCellValue(sheetName, cell, s.getExampleValueForType(col.Type))
	}

	// Write to buffer and return bytes
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write Excel to buffer: %w", err)
	}

	return buffer.Bytes(), nil
}

// getExampleValueForType returns an example value for a given data type
func (s *dataSourceService) getExampleValueForType(dataType string) string {
	switch dataType {
	case "varchar", "character varying", "text":
		return "text"
	case "int", "integer", "bigint", "smallint":
		return "123"
	case "float", "double", "real", "numeric", "decimal", "double precision":
		return "123.45"
	case "boolean", "bool":
		return "true"
	case "timestamp", "timestamptz", "timestamp without time zone", "timestamp with time zone":
		return "2025-01-01 12:00:00"
	case "date":
		return "2023-01-01"
	case "time":
		return "12:00:00"
	case "uuid":
		return "550e8400-e29b-41d4-a716-446655440000"
	case "json", "jsonb":
		return `{"key": "value"}`
	default:
		// For array types or other complex types
		if strings.HasSuffix(dataType, "[]") {
			return "[value1, value2]"
		}
		return "text"
	}
}

// ImportTableRecords imports records from an Excel file
func (s *dataSourceService) ImportTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, file io.Reader, fileName string, skipUnmatchedColumns bool) (dto.ImportRecordResponse, error) {
	// 1. Validate data source and table existence
	_, _, err := s.validateDataSourceAndTable(ctx, organizationID, dataSourceID, tableID)
	if err != nil {
		return dto.ImportRecordResponse{}, err
	}

	// 2. Get table columns for validation
	columnsResp, err := s.GetTableColumns(ctx, organizationID, dataSourceID, tableID, false)
	if err != nil {
		return dto.ImportRecordResponse{}, fmt.Errorf("failed to get table columns: %w", err)
	}

	// 3. Parse Excel file
	records, err := s.parseExcelFile(file, fileName, columnsResp.Columns, skipUnmatchedColumns)
	if err != nil {
		return dto.ImportRecordResponse{}, fmt.Errorf("failed to parse Excel file: %w", err)
	}

	// 4. Convert to AddRecordRequest format
	addReq := dto.AddRecordRequest{
		Records: records,
	}

	// 5. Add records to table
	result, err := s.AddTableRecords(ctx, organizationID, dataSourceID, tableID, accountID, addReq)
	if err != nil {
		return dto.ImportRecordResponse{}, fmt.Errorf("failed to add records: %w", err)
	}

	// 6. Build response
	response := dto.ImportRecordResponse{
		AffectedRows: int(result.AffectedRows),
		FailedCount:  0,
		TotalCount:   len(records),
		FailedItems:  []dto.ImportFailedItem{},
	}

	return response, nil
}

func (s *dataSourceService) ImportTableRecordsFromUploadFile(ctx context.Context, organizationID, dataSourceID, tableID, accountID, uploadFileID string, skipUnmatchedColumns bool) (dto.ImportRecordResponse, error) {
	fileInfo, err := s.fileService.GetFileByID(ctx, uploadFileID)
	if err != nil {
		return dto.ImportRecordResponse{}, fmt.Errorf("failed to get file: %w", err)
	}
	if err := s.ensureExcelImportFileReadable(ctx, organizationID, accountID, fileInfo); err != nil {
		return dto.ImportRecordResponse{}, err
	}
	content, err := s.fileService.DownloadFile(ctx, uploadFileID)
	if err != nil {
		return dto.ImportRecordResponse{}, fmt.Errorf("failed to download file: %w", err)
	}
	return s.ImportTableRecords(ctx, organizationID, dataSourceID, tableID, accountID, bytes.NewReader(content), fileInfo.Name, skipUnmatchedColumns)
}

func (s *dataSourceService) AnalyzeExcelImport(ctx context.Context, organizationID, dataSourceID, accountID string, req dto.AnalyzeExcelImportRequest) (dto.AnalyzeExcelImportData, error) {
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return dto.AnalyzeExcelImportData{}, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil || dataSource.OrganizationID != organizationID {
		return dto.AnalyzeExcelImportData{}, fmt.Errorf("data source not found")
	}

	fileInfo, err := s.fileService.GetFileByID(ctx, req.UploadFileID)
	if err != nil {
		return dto.AnalyzeExcelImportData{}, fmt.Errorf("failed to get file: %w", err)
	}
	if err := s.ensureExcelImportFileReadable(ctx, organizationID, accountID, fileInfo); err != nil {
		return dto.AnalyzeExcelImportData{}, err
	}
	content, err := s.fileService.DownloadFile(ctx, req.UploadFileID)
	if err != nil {
		return dto.AnalyzeExcelImportData{}, fmt.Errorf("failed to download file: %w", err)
	}

	workbook, err := excelimportsvc.ParseWorkbook(fileInfo.Name, content)
	if err != nil {
		return dto.AnalyzeExcelImportData{}, err
	}

	sampleSize := 0
	if req.SampleSize != nil {
		sampleSize = *req.SampleSize
	}
	analysis, err := excelimportsvc.AnalyzeWorkbook(workbook, excelimportsvc.AnalyzeOptions{
		SheetName:  req.SheetName,
		HeaderRow:  req.HeaderRow,
		SampleSize: sampleSize,
	})
	if err != nil {
		return dto.AnalyzeExcelImportData{}, err
	}

	resp := dto.AnalyzeExcelImportData{}
	resp.Source.FileName = fileInfo.Name
	resp.Source.SourceType = workbook.SourceType
	resp.Source.Sheets = analysis.Sheets
	resp.Selection.SheetName = analysis.Selection.SheetName
	resp.Selection.HeaderRow = analysis.Selection.HeaderRow
	resp.Selection.StartRow = analysis.Selection.StartRow
	resp.Columns = analysis.Columns
	resp.PreviewRows = analysis.PreviewRows
	resp.Warnings = analysis.Warnings

	schemaSnapshot := mustJSON(resp.Columns)
	previewSnapshot := mustJSON(resp.PreviewRows)
	job := &excelimportmodel.ImportJob{
		OrganizationID:  organizationID,
		WorkspaceID:     dataSource.WorkspaceID,
		DataSourceID:    dataSourceID,
		UploadFileID:    &req.UploadFileID,
		SourceType:      workbook.SourceType,
		SourceFileName:  fileInfo.Name,
		Status:          string(dto.ExcelImportStatusNeedsReview),
		TotalRows:       analysis.TotalRows,
		ValidRows:       analysis.ValidRows,
		SheetName:       &analysis.Selection.SheetName,
		HeaderRow:       &analysis.Selection.HeaderRow,
		StartRow:        &analysis.Selection.StartRow,
		SchemaSnapshot:  schemaSnapshot,
		PreviewSnapshot: previewSnapshot,
		CreatedBy:       accountID,
		UpdatedBy:       accountID,
	}
	jobRepo := excelimportrepo.NewJobRepository(s.db)
	if err := jobRepo.Create(ctx, job); err != nil {
		return dto.AnalyzeExcelImportData{}, fmt.Errorf("failed to create import job: %w", err)
	}
	resp.JobID = job.ID
	return resp, nil
}

func (s *dataSourceService) ConfirmExcelImport(ctx context.Context, organizationID, dataSourceID, accountID, jobID string, req dto.ConfirmExcelImportRequest) (dto.ConfirmExcelImportData, error) {
	jobRepo := excelimportrepo.NewJobRepository(s.db)
	errorRepo := excelimportrepo.NewErrorRepository(s.db)
	job, err := jobRepo.FindByID(ctx, jobID)
	if err != nil {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("failed to find import job: %w", err)
	}
	if job == nil || job.OrganizationID != organizationID || job.DataSourceID != dataSourceID {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("import job not found")
	}

	if err := excelimportsvc.ValidateImportSchema(req); err != nil {
		return dto.ConfirmExcelImportData{}, err
	}

	claimed, err := jobRepo.MarkImporting(ctx, job.ID, organizationID, dataSourceID, accountID)
	if err != nil {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("failed to update import job: %w", err)
	}
	if !claimed {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("import job status %q cannot be confirmed", job.Status)
	}
	job, err = jobRepo.FindByID(ctx, jobID)
	if err != nil {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("failed to reload import job: %w", err)
	}
	if job == nil {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("import job not found")
	}

	failImport := func(cause error, cleanupTableID *string) (dto.ConfirmExcelImportData, error) {
		if cleanupTableID != nil && *cleanupTableID != "" {
			job.TableID = cleanupTableID
			if cleanupErr := s.DeleteTable(ctx, organizationID, dataSourceID, *cleanupTableID, accountID); cleanupErr != nil {
				logger.WarnContext(ctx, "failed to clean up table after excel import failure", "job_id", job.ID, "table_id", *cleanupTableID, cleanupErr)
			} else {
				job.TableID = nil
			}
		}
		job.Status = string(dto.ExcelImportStatusFailed)
		job.ErrorSummary = mustJSON(map[string]string{"message": cause.Error()})
		job.UpdatedBy = accountID
		if updateErr := jobRepo.Update(ctx, job); updateErr != nil {
			return dto.ConfirmExcelImportData{}, fmt.Errorf("%w; also failed to mark import job failed: %v", cause, updateErr)
		}
		return dto.ConfirmExcelImportData{}, cause
	}

	createImportTable := func() (*model.Table, error) {
		description := ""
		if req.Table.Description != nil {
			description = *req.Table.Description
		}
		table, err := s.CreateTable(ctx, organizationID, dataSourceID, accountID, dto.CreateTableRequest{
			Name:        req.Table.Name,
			Description: description,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create table: %w", err)
		}

		columns := excelImportTableColumns(req.Columns)
		if len(columns) > 0 {
			if err := s.UpdateTableColumns(ctx, organizationID, dataSourceID, table.ID, accountID, dto.UpdateTableColumnsRequest{Columns: columns}); err != nil {
				return table, fmt.Errorf("failed to create columns: %w", err)
			}
		}
		return table, nil
	}

	if job.UploadFileID == nil || *job.UploadFileID == "" {
		return failImport(fmt.Errorf("import job has no upload file"), nil)
	}
	fileInfo, err := s.fileService.GetFileByID(ctx, *job.UploadFileID)
	if err != nil {
		return failImport(fmt.Errorf("failed to get file: %w", err), nil)
	}
	if err := s.ensureExcelImportFileReadable(ctx, organizationID, accountID, fileInfo); err != nil {
		return failImport(err, nil)
	}
	content, err := s.fileService.DownloadFile(ctx, *job.UploadFileID)
	if err != nil {
		return failImport(fmt.Errorf("failed to download file: %w", err), nil)
	}
	workbook, err := excelimportsvc.ParseWorkbook(job.SourceFileName, content)
	if err != nil {
		return failImport(err, nil)
	}

	validation, err := excelimportsvc.ValidateRows(workbook, req)
	if err != nil {
		return failImport(err, nil)
	}
	if req.Options.ErrorPolicy == "fail_fast" && len(validation.Errors) > 0 {
		importErrors := buildExcelImportErrors(job.ID, validation.Errors)
		if err := errorRepo.DeleteByJobID(ctx, job.ID); err != nil {
			return dto.ConfirmExcelImportData{}, fmt.Errorf("failed to clear import errors: %w", err)
		}
		if err := errorRepo.CreateBatch(ctx, importErrors); err != nil {
			return dto.ConfirmExcelImportData{}, fmt.Errorf("failed to save import errors: %w", err)
		}
		job.TotalRows = validation.TotalRows
		job.ValidRows = len(validation.Records)
		job.FailedRows = countExcelImportFailedRows(validation.Errors)
		return failImport(fmt.Errorf("import stopped after the first invalid row"), nil)
	}

	table, err := createImportTable()
	if err != nil {
		return failImport(err, excelImportCleanupTableID(table))
	}

	importedRows := 0
	batchSize := req.Options.BatchSize
	if batchSize <= 0 {
		batchSize = 500
	}
	for start := 0; start < len(validation.Records); start += batchSize {
		end := start + batchSize
		if end > len(validation.Records) {
			end = len(validation.Records)
		}
		result, err := s.AddTableRecords(ctx, organizationID, dataSourceID, table.ID, accountID, dto.AddRecordRequest{Records: validation.Records[start:end]})
		if err != nil {
			return failImport(fmt.Errorf("failed to insert records: %w", err), &table.ID)
		}
		if result.AffectedRows > 0 {
			importedRows += int(result.AffectedRows)
		} else {
			importedRows += end - start
		}
	}

	importErrors := buildExcelImportErrors(job.ID, validation.Errors)
	if err := errorRepo.DeleteByJobID(ctx, job.ID); err != nil {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("failed to clear import errors: %w", err)
	}
	if err := errorRepo.CreateBatch(ctx, importErrors); err != nil {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("failed to save import errors: %w", err)
	}

	status := dto.ExcelImportStatusCompleted
	if len(validation.Errors) > 0 {
		status = dto.ExcelImportStatusPartialFailed
	}
	job.TableID = &table.ID
	job.Status = string(status)
	job.TotalRows = validation.TotalRows
	job.ValidRows = len(validation.Records)
	job.ImportedRows = importedRows
	job.FailedRows = countExcelImportFailedRows(validation.Errors)
	job.SchemaSnapshot = mustJSON(req.Columns)
	job.ErrorSummary = mustJSON(validation.Errors)
	job.UpdatedBy = accountID
	if err := jobRepo.Update(ctx, job); err != nil {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("failed to finalize import job: %w", err)
	}

	return dto.ConfirmExcelImportData{
		JobID:        job.ID,
		TableID:      table.ID,
		Status:       status,
		TotalRows:    validation.TotalRows,
		ImportedRows: importedRows,
		FailedRows:   countExcelImportFailedRows(validation.Errors),
		FailedItems:  validation.Errors,
	}, nil
}

func excelImportCleanupTableID(table *model.Table) *string {
	if table == nil || table.ID == "" {
		return nil
	}
	return &table.ID
}

func excelImportTableColumns(columns []dto.InferredExcelColumn) []dto.TableColumn {
	out := make([]dto.TableColumn, 0, len(columns))
	for _, col := range columns {
		if col.Enabled != nil && !*col.Enabled {
			continue
		}
		desc := strings.TrimSpace(col.Description)
		displayName := strings.TrimSpace(col.DisplayName)
		sourceColumn := strings.TrimSpace(col.SourceColumn)
		tableColumn := dto.TableColumn{
			Name:        strings.TrimSpace(col.Name),
			Description: &desc,
			Type:        strings.TrimSpace(col.Type),
			IsRequired:  col.IsRequired,
		}
		if displayName != "" {
			tableColumn.DisplayName = &displayName
		}
		if sourceColumn != "" {
			tableColumn.SourceColumnName = &sourceColumn
		}
		out = append(out, tableColumn)
	}
	return out
}

func (s *dataSourceService) ensureExcelImportFileReadable(ctx context.Context, organizationID, accountID string, uploadFile *dto.UploadFile) error {
	if uploadFile == nil {
		return fmt.Errorf("file not found")
	}
	if uploadFile.OrganizationID != "" && uploadFile.OrganizationID != organizationID {
		return fmt.Errorf("file not found")
	}

	workspaceID := excelImportUploadFileWorkspaceID(uploadFile)
	if workspaceID == "" {
		return nil
	}
	if s.organizationService == nil {
		return fmt.Errorf("file permission service unavailable")
	}
	hasPermission, err := s.organizationService.CheckWorkspacePermission(ctx, organizationID, workspaceID, accountID, workspace_model.WorkspacePermissionFileDownload)
	if err != nil {
		return fmt.Errorf("failed to check file permission: %w", err)
	}
	if !hasPermission {
		return fmt.Errorf("permission denied to read file")
	}
	return nil
}

func excelImportUploadFileWorkspaceID(uploadFile *dto.UploadFile) string {
	if uploadFile.WorkspaceID != nil && *uploadFile.WorkspaceID != "" {
		return *uploadFile.WorkspaceID
	}
	if uploadFile.TeamTenantID != nil && *uploadFile.TeamTenantID != "" {
		return *uploadFile.TeamTenantID
	}
	return ""
}

func countExcelImportFailedRows(failedItems []dto.ExcelImportFailedItem) int {
	rows := make(map[int]struct{})
	for _, item := range failedItems {
		rows[item.RowIndex] = struct{}{}
	}
	return len(rows)
}

func buildExcelImportErrors(jobID string, failedItems []dto.ExcelImportFailedItem) []excelimportmodel.ImportError {
	importErrors := make([]excelimportmodel.ImportError, 0, len(failedItems))
	for _, item := range failedItems {
		importErrors = append(importErrors, excelimportmodel.ImportError{
			JobID:        jobID,
			RowIndex:     item.RowIndex,
			ColumnName:   item.ColumnName,
			RawValue:     item.RawValue,
			ErrorCode:    item.ErrorCode,
			ErrorMessage: item.ErrorMessage,
		})
	}
	return importErrors
}

func (s *dataSourceService) GetExcelImportJob(ctx context.Context, organizationID, dataSourceID, jobID string) (*dto.ExcelImportJobResponse, error) {
	jobRepo := excelimportrepo.NewJobRepository(s.db)
	job, err := jobRepo.FindByID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job == nil || job.OrganizationID != organizationID || job.DataSourceID != dataSourceID {
		return nil, nil
	}
	return convertExcelImportJob(job), nil
}

func (s *dataSourceService) ListExcelImportErrors(ctx context.Context, organizationID, dataSourceID, jobID string, limit, offset int) (dto.ExcelImportErrorList, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	job, err := s.GetExcelImportJob(ctx, organizationID, dataSourceID, jobID)
	if err != nil {
		return dto.ExcelImportErrorList{}, err
	}
	if job == nil {
		return dto.ExcelImportErrorList{}, fmt.Errorf("import job not found")
	}
	errorRepo := excelimportrepo.NewErrorRepository(s.db)
	rows, total, err := errorRepo.ListByJobID(ctx, jobID, limit, offset)
	if err != nil {
		return dto.ExcelImportErrorList{}, err
	}
	data := make([]dto.ExcelImportFailedItem, 0, len(rows))
	for _, row := range rows {
		data = append(data, dto.ExcelImportFailedItem{
			RowIndex:     row.RowIndex,
			ColumnName:   row.ColumnName,
			RawValue:     row.RawValue,
			ErrorCode:    row.ErrorCode,
			ErrorMessage: row.ErrorMessage,
		})
	}
	return dto.ExcelImportErrorList{
		Data:     data,
		HasMore:  int64(offset+len(data)) < total,
		Limit:    limit,
		Offset:   offset,
		TotalNum: total,
	}, nil
}

func mustJSON(value interface{}) datatypes.JSON {
	data, err := json.Marshal(value)
	if err != nil {
		return datatypes.JSON([]byte("null"))
	}
	return datatypes.JSON(data)
}

func convertExcelImportJob(job *excelimportmodel.ImportJob) *dto.ExcelImportJobResponse {
	return &dto.ExcelImportJobResponse{
		ID:             job.ID,
		OrganizationID: job.OrganizationID,
		WorkspaceID:    job.WorkspaceID,
		DataSourceID:   job.DataSourceID,
		TableID:        job.TableID,
		UploadFileID:   job.UploadFileID,
		SourceType:     job.SourceType,
		SourceFileName: job.SourceFileName,
		Status:         dto.ExcelImportStatus(job.Status),
		TotalRows:      job.TotalRows,
		ValidRows:      job.ValidRows,
		ImportedRows:   job.ImportedRows,
		FailedRows:     job.FailedRows,
		SheetName:      job.SheetName,
		HeaderRow:      job.HeaderRow,
		StartRow:       job.StartRow,
		CreatedBy:      job.CreatedBy,
		UpdatedBy:      job.UpdatedBy,
		CreatedAt:      job.CreatedAt,
		UpdatedAt:      job.UpdatedAt,
	}
}

// parseExcelFile parses an Excel file and converts it to records
func (s *dataSourceService) parseExcelFile(file io.Reader, fileName string, columns []dto.TableColumn, skipUnmatchedColumns bool) ([]map[string]interface{}, error) {
	// Read the file content
	fileContent, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	workbook, err := excelimportsvc.ParseWorkbook(fileName, fileContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workbook: %w", err)
	}
	sheet, err := excelimportsvc.RecommendedSheet(workbook)
	if err != nil {
		return nil, err
	}
	rows := sheet.Rows

	if len(rows) < 2 {
		return nil, fmt.Errorf("Excel file must contain at least header row and one data row")
	}

	// First row is header
	header := rows[0]

	columnMap := make(map[string]dto.TableColumn)
	for _, col := range columns {
		columnMap[col.Name] = col
	}

	type matchedColumn struct {
		index int
		name  string
		info  dto.TableColumn
	}
	matchedColumns := make([]matchedColumn, 0, len(header))
	matchedColumnNames := make(map[string]struct{})
	for colIndex, headerName := range header {
		columnInfo, exists := columnMap[headerName]
		if !exists {
			if skipUnmatchedColumns {
				continue
			}
			return nil, fmt.Errorf("column '%s' does not exist in table", headerName)
		}
		matchedColumns = append(matchedColumns, matchedColumn{index: colIndex, name: headerName, info: columnInfo})
		matchedColumnNames[headerName] = struct{}{}
	}
	if len(matchedColumns) == 0 {
		return nil, fmt.Errorf("no matching columns found in Excel header")
	}
	if missing := missingRequiredImportColumns(columns, matchedColumnNames); len(missing) > 0 {
		return nil, fmt.Errorf("missing required columns: %s", strings.Join(missing, ", "))
	}

	// Convert data rows to records
	var records []map[string]interface{}
	for rowIndex, row := range rows[1:] { // Skip header row
		if len(row) == 0 {
			continue // Skip empty rows
		}

		record := make(map[string]interface{})
		for _, matched := range matchedColumns {
			if matched.index >= len(row) {
				continue
			}

			// Convert cell value based on column type
			convertedValue, err := s.convertCellValue(row[matched.index], matched.info.Type, matched.info.IsRequired)
			if err != nil {
				return nil, fmt.Errorf("error converting value in row %d, column '%s': %w", rowIndex+2, matched.name, err)
			}

			record[matched.name] = convertedValue
		}

		// Validate required fields
		for _, col := range columns {
			_, exists := record[col.Name]
			if !exists && col.IsRequired {
				return nil, fmt.Errorf("required field '%s' is missing in row %d", col.Name, rowIndex+2)
			}
		}

		if len(record) == 0 {
			continue
		}

		records = append(records, record)
	}

	return records, nil
}

func missingRequiredImportColumns(columns []dto.TableColumn, matchedColumnNames map[string]struct{}) []string {
	missing := make([]string, 0)
	for _, col := range columns {
		if !col.IsRequired {
			continue
		}
		if _, exists := matchedColumnNames[col.Name]; !exists {
			missing = append(missing, col.Name)
		}
	}
	return missing
}

// convertCellValue converts a cell value to the appropriate type
func (s *dataSourceService) convertCellValue(value string, dataType string, isRequired bool) (interface{}, error) {
	// If value is empty
	if value == "" {
		if isRequired {
			return nil, fmt.Errorf("value is required")
		}
		return nil, nil
	}

	// Convert based on data type
	switch dataType {
	case "varchar", "character varying", "text", "char", "character":
		return value, nil

	case "int", "integer", "bigint", "smallint":
		intVal, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("cannot convert '%s' to integer", value)
		}
		return intVal, nil

	case "float", "double", "real", "numeric", "decimal", "double precision":
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("cannot convert '%s' to float", value)
		}
		return floatVal, nil

	case "boolean", "bool":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			// Also try to parse common boolean representations
			switch strings.ToLower(value) {
			case "yes", "y", "1", "on":
				return true, nil
			case "no", "n", "0", "off":
				return false, nil
			default:
				return nil, fmt.Errorf("cannot convert '%s' to boolean", value)
			}
		}
		return boolVal, nil

	case "timestamp", "timestamptz", "timestamp without time zone", "timestamp with time zone":
		if normalized, ok := normalizeLocalTimestampValue(value); ok {
			return normalized, nil
		}
		if parsedTime, err := time.Parse(time.RFC3339, value); err == nil {
			return parsedTime, nil
		}
		return nil, fmt.Errorf("cannot convert '%s' to timestamp", value)

	case "date":
		parsedDate, err := time.Parse("2006-01-02", value)
		if err != nil {
			return nil, fmt.Errorf("cannot convert '%s' to date", value)
		}
		return parsedDate, nil

	case "time":
		parsedTime, err := time.Parse("15:04:05", value)
		if err != nil {
			return nil, fmt.Errorf("cannot convert '%s' to time", value)
		}
		return parsedTime, nil

	case "uuid":
		_, err := uuid.Parse(value)
		if err != nil {
			return nil, fmt.Errorf("cannot convert '%s' to UUID", value)
		}
		return value, nil

	default:
		// For array types or other complex types, treat as string
		if strings.HasSuffix(dataType, "[]") {
			return value, nil
		}
		return value, nil
	}
}

func normalizeLocalTimestampValue(value string) (string, bool) {
	for _, format := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		parsedTime, err := time.ParseInLocation(format, value, time.Local)
		if err != nil {
			continue
		}
		if format == "2006-01-02" {
			return parsedTime.Format("2006-01-02 00:00:00"), true
		}
		return parsedTime.Format("2006-01-02 15:04:05"), true
	}
	return "", false
}

// CountOperationLogsByDataSourceIDWithFilters counts operation logs for a specific data source with filters
func (s *dataSourceService) CountOperationLogsByDataSourceIDWithFilters(ctx context.Context, organizationID, dataSourceID string, filters dto.SQLOperationFilter) (int64, error) {
	// Validate data source exists and belongs to organization
	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil {
		return 0, fmt.Errorf("data source with id '%s' not found", dataSourceID)
	}
	if dataSource.OrganizationID != organizationID {
		return 0, fmt.Errorf("data source does not belong to organization")
	}

	// Convert DTO filter to repository filter
	repoFilter := dto.SQLOperationFilter{
		TableID:       filters.TableID,
		CreatedBy:     filters.CreatedBy,
		OperationType: filters.OperationType,
		Status:        filters.Status,
	}

	// Use time values directly as they are already time.Time
	repoFilter.CreatedAtGTE = filters.CreatedAtGTE
	repoFilter.CreatedAtLTE = filters.CreatedAtLTE

	// Get count from repository
	count, err := s.sqlOperationRepo.CountByDataSourceIDWithFilters(ctx, dataSourceID, repoFilter)
	if err != nil {
		return 0, fmt.Errorf("failed to count operation logs: %w", err)
	}

	return count, nil
}
