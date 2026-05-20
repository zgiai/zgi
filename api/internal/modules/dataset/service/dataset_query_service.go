package service

import (
	"context"
	"time"

	"github.com/zgiai/ginext/internal/dto"
	dataset_model "github.com/zgiai/ginext/internal/modules/dataset/model"
	"github.com/zgiai/ginext/internal/modules/dataset/repository"
)

type DatasetQueryService interface {
	CreateDatasetQuery(ctx context.Context, req *CreateDatasetQueryRequest) (*dataset_model.DatasetQuery, error)

	// GetDatasetQueries retrieves dataset queries with pagination.
	// When QueryType is not specified in the request, it returns "single" and "batch_saved" query types
	// by default, excluding "batch" type which represents individual queries within batch testing.
	GetDatasetQueries(ctx context.Context, req *GetDatasetQueriesRequest) (*GetDatasetQueriesResponse, error)

	GetDatasetQueryByID(ctx context.Context, id string) (*dataset_model.DatasetQuery, error)

	DeleteDatasetQuery(ctx context.Context, id string) error

	GetDatasetQueryStats(ctx context.Context, datasetID string) (*DatasetQueryStats, error)

	SaveBatchHitTestingResults(ctx context.Context, req *SaveBatchHitTestingResultsRequest) error
}

type CreateDatasetQueryRequest struct {
	DatasetID     string  `json:"dataset_id" binding:"required"`
	Content       string  `json:"content" binding:"required"`
	Source        string  `json:"source" binding:"required"`
	SourceAppID   *string `json:"source_app_id,omitempty"`
	CreatedByRole string  `json:"created_by_role" binding:"required"`
	CreatedBy     string  `json:"created_by" binding:"required"`

	Results     *dto.HitTestingResponse `json:"results,omitempty"`
	ElapsedTime *float64                `json:"elapsed_time,omitempty"`
	HitCount    *int                    `json:"hit_count,omitempty"`
	QueryType   string                  `json:"query_type"`
	BatchTaskID *string                 `json:"batch_task_id,omitempty"`
	BatchName   *string                 `json:"batch_name,omitempty"`
}

type GetDatasetQueriesRequest struct {
	DatasetID      string `json:"dataset_id" binding:"required"`
	Page           int    `json:"page"`
	Limit          int    `json:"limit"`
	AccountID      string `json:"account_id" binding:"required"`
	OrganizationID string `json:"organization_id" binding:"required"`

	QueryType *string `json:"query_type,omitempty"` // single, batch_saved.
	// When not specified, both "single" and "batch_saved" types are returned,
	// but "batch" type (individual queries in batch testing) are excluded by default.
}

type GetDatasetQueriesResponse struct {
	Data    []*dataset_model.DatasetQuery `json:"data"`
	HasMore bool                          `json:"has_more"`
	Limit   int                           `json:"limit"`
	Total   int64                         `json:"total"`
	Page    int                           `json:"page"`
}

type DatasetQueryStats struct {
	TotalQueries int64 `json:"total_queries"`
	TodayQueries int64 `json:"today_queries"`
	WeekQueries  int64 `json:"week_queries"`
}

type SaveBatchHitTestingResultsRequest struct {
	BatchTaskID    string     `json:"batch_task_id"`
	BatchName      string     `json:"batch_name" binding:"required"`
	DatasetID      string     `json:"dataset_id" binding:"required"`
	AccountID      string     `json:"account_id" binding:"required"`
	OrganizationID string     `json:"organization_id" binding:"required"`
	CreatedBy      string     `json:"created_by" binding:"required"`
	StartedAt      *time.Time `json:"-"`
	FinishedAt     *time.Time `json:"-"`
}

type datasetQueryServiceImpl struct {
	queryRepo      repository.DatasetQueryRepository
	datasetService DatasetService
}

func NewDatasetQueryService(
	queryRepo repository.DatasetQueryRepository,
	datasetService DatasetService,
) DatasetQueryService {
	return &datasetQueryServiceImpl{
		queryRepo:      queryRepo,
		datasetService: datasetService,
	}
}

func (s *datasetQueryServiceImpl) CreateDatasetQuery(ctx context.Context, req *CreateDatasetQueryRequest) (*dataset_model.DatasetQuery, error) {
	datasetQuery := &dataset_model.DatasetQuery{
		DatasetID:     req.DatasetID,
		Content:       req.Content,
		Source:        req.Source,
		SourceAppID:   req.SourceAppID,
		CreatedByRole: req.CreatedByRole,
		CreatedBy:     req.CreatedBy,
		Results:       req.Results,
		ElapsedTime:   req.ElapsedTime,
		HitCount:      req.HitCount,
		QueryType:     req.QueryType,
		BatchTaskID:   req.BatchTaskID,
		BatchName:     req.BatchName,
	}

	if err := s.queryRepo.Create(ctx, datasetQuery); err != nil {
		return nil, err
	}

	return datasetQuery, nil
}

func (s *datasetQueryServiceImpl) GetDatasetQueries(ctx context.Context, req *GetDatasetQueriesRequest) (*GetDatasetQueriesResponse, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	// Get queries with the specified filter
	// When req.QueryType is nil, the repository will return both "single" and "batch_saved" types
	// but exclude "batch" type (individual queries in batch testing) by default
	queries, total, err := s.queryRepo.GetByDatasetID(ctx, req.DatasetID, page, limit, req.QueryType)
	if err != nil {
		return nil, err
	}

	hasMore := int64(page*limit) < total

	return &GetDatasetQueriesResponse{
		Data:    queries,
		HasMore: hasMore,
		Limit:   limit,
		Total:   total,
		Page:    page,
	}, nil
}

func (s *datasetQueryServiceImpl) GetDatasetQueryByID(ctx context.Context, id string) (*dataset_model.DatasetQuery, error) {
	return s.queryRepo.GetByID(ctx, id)
}

func (s *datasetQueryServiceImpl) DeleteDatasetQuery(ctx context.Context, id string) error {
	return s.queryRepo.Delete(ctx, id)
}

func (s *datasetQueryServiceImpl) GetDatasetQueryStats(ctx context.Context, datasetID string) (*DatasetQueryStats, error) {
	return &DatasetQueryStats{
		TotalQueries: 0,
		TodayQueries: 0,
		WeekQueries:  0,
	}, nil
}

func (s *datasetQueryServiceImpl) SaveBatchHitTestingResults(ctx context.Context, req *SaveBatchHitTestingResultsRequest) error {
	// todo: check task status

	var elapsed *float64
	if req.StartedAt != nil && req.FinishedAt != nil {
		elapsedTime := float64(req.FinishedAt.Sub(*req.StartedAt).Microseconds()) / 1000.0
		elapsed = &elapsedTime
	}

	batchQuery := &CreateDatasetQueryRequest{
		DatasetID:     req.DatasetID,
		Content:       req.BatchName,
		Source:        "batch_hit_testing",
		CreatedByRole: "account",
		CreatedBy:     req.CreatedBy,
		QueryType:     "batch_saved",
		BatchTaskID:   &req.BatchTaskID,
		BatchName:     &req.BatchName,
		ElapsedTime:   elapsed,
	}

	_, err := s.CreateDatasetQuery(ctx, batchQuery)
	return err
}
