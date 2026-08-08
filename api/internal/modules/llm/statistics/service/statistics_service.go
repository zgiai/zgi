package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/repository"
	"gorm.io/gorm"
)

const maxUnixSeconds = int64(9999999999)

type statisticsServiceImpl struct {
	statisticsRepo repository.StatisticsRepository
}

func (s *statisticsServiceImpl) GetInvocationContentSettings(ctx context.Context, organizationID string) (*dto.InvocationContentSettings, error) {
	cfg := appconfig.Current().LLMInvocationContent
	enabled, err := s.statisticsRepo.GetInvocationContentSettings(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get invocation content settings: %w", err)
	}
	return &dto.InvocationContentSettings{Available: cfg.Available, Enabled: cfg.Available && enabled, MaxBytes: cfg.MaxBytes, RetentionDays: cfg.RetentionDays}, nil
}

func (s *statisticsServiceImpl) UpdateInvocationContentSettings(ctx context.Context, organizationID string, req *dto.UpdateInvocationContentSettingsRequest) (*dto.InvocationContentSettings, error) {
	cfg := appconfig.Current().LLMInvocationContent
	if req.Enabled && !cfg.Available {
		return nil, ErrInvocationContentUnavailable
	}
	if err := s.statisticsRepo.UpdateInvocationContentSettings(ctx, organizationID, req.Enabled); err != nil {
		return nil, fmt.Errorf("failed to update invocation content settings: %w", err)
	}
	return &dto.InvocationContentSettings{Available: cfg.Available, Enabled: req.Enabled, MaxBytes: cfg.MaxBytes, RetentionDays: cfg.RetentionDays}, nil
}

func (s *statisticsServiceImpl) GetInvocationContent(ctx context.Context, organizationID, accountID, invocationID string) (*dto.InvocationContentDetail, error) {
	if !appconfig.Current().LLMInvocationContent.Available {
		return nil, ErrInvocationContentUnavailable
	}
	invocationID = strings.TrimSpace(invocationID)
	if invocationID == "" || len(invocationID) > 100 {
		return nil, ErrInvocationContentNotFound
	}
	result, err := s.statisticsRepo.GetInvocationContent(ctx, organizationID, accountID, invocationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvocationContentNotFound
		}
		return nil, fmt.Errorf("failed to get invocation content: %w", err)
	}
	return result, nil
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

func (s *statisticsServiceImpl) GetInvocationLog(ctx context.Context, organizationID string, req *dto.InvocationLogRequest) (*dto.InvocationLogResponse, error) {
	startTime := req.StartTime
	endTime := req.EndTime
	if err := validateUnixSecondRange(&startTime, &endTime); err != nil {
		return nil, err
	}
	if req.Limit == 0 {
		req.Limit = 20
	}
	if (req.CursorTime == nil) != (req.CursorID == nil) {
		return nil, ErrInvalidCursor
	}
	if req.CursorTime != nil {
		if strings.TrimSpace(*req.CursorID) == "" {
			return nil, ErrInvalidCursor
		}
		cursorAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*req.CursorTime))
		if err != nil || cursorAt.IsZero() {
			return nil, ErrInvalidCursor
		}
	}

	resp, err := s.statisticsRepo.GetInvocationLog(ctx, organizationID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get invocation log: %w", err)
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
