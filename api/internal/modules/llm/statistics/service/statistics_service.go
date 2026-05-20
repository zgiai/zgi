package service

import (
	"context"
	"fmt"

	"github.com/zgiai/ginext/internal/modules/llm/statistics/dto"
	"github.com/zgiai/ginext/internal/modules/llm/statistics/repository"
)

const maxUnixSeconds = int64(9999999999)

type statisticsServiceImpl struct {
	statisticsRepo repository.StatisticsRepository
}

func NewStatisticsService(statisticsRepo repository.StatisticsRepository) StatisticsService {
	return &statisticsServiceImpl{
		statisticsRepo: statisticsRepo,
	}
}

func (s *statisticsServiceImpl) GetModelUsage(ctx context.Context, organizationID string, req *dto.ModelUsageRequest) (*dto.ModelUsageResponse, error) {
	startTime := req.StartTime
	endTime := req.EndTime
	if err := validateUnixSecondRange(&startTime, &endTime); err != nil {
		return nil, err
	}

	resp, err := s.statisticsRepo.GetModelUsage(ctx, organizationID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get model usage: %w", err)
	}

	return resp, nil
}

func (s *statisticsServiceImpl) GetWorkspaceQuota(ctx context.Context, organizationID string, req *dto.WorkspaceQuotaRequest) (*dto.WorkspaceQuotaResponse, error) {
	resp, err := s.statisticsRepo.GetWorkspaceQuota(ctx, organizationID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace quota: %w", err)
	}

	return resp, nil
}

func validateUnixSecondRange(startTimestamp, endTimestamp *int64) error {
	if err := validateUnixSecond(startTimestamp); err != nil {
		return err
	}
	if err := validateUnixSecond(endTimestamp); err != nil {
		return err
	}
	if startTimestamp != nil && endTimestamp != nil && *endTimestamp < *startTimestamp {
		return ErrInvalidTimestampRange
	}
	return nil
}

func validateUnixSecond(ts *int64) error {
	if ts == nil {
		return nil
	}
	if *ts <= 0 || *ts > maxUnixSeconds {
		return ErrInvalidTimestamp
	}
	return nil
}
