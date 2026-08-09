package service

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/dto"
)

type fakeStatisticsRepository struct {
	modelUsageReq *dto.ModelUsageRequest
}

func (f *fakeStatisticsRepository) GetModelUsage(_ context.Context, _ string, req *dto.ModelUsageRequest) (*dto.ModelUsageResponse, error) {
	f.modelUsageReq = req
	return &dto.ModelUsageResponse{
		Summary: dto.ModelUsageSummary{TotalPoints: 1},
	}, nil
}

func (f *fakeStatisticsRepository) GetInvocationLog(context.Context, string, *dto.InvocationLogRequest) (*dto.InvocationLogResponse, error) {
	return &dto.InvocationLogResponse{}, nil
}

func (f *fakeStatisticsRepository) GetWorkspaceQuota(context.Context, string, *dto.WorkspaceQuotaRequest) (*dto.WorkspaceQuotaResponse, error) {
	return &dto.WorkspaceQuotaResponse{}, nil
}

func TestGetModelUsage_PassesUnixSecondsToRepository(t *testing.T) {
	repo := &fakeStatisticsRepository{}
	svc := NewStatisticsService(repo)

	resp, err := svc.GetModelUsage(context.Background(), "org-1", &dto.ModelUsageRequest{
		StartTime: 1710000000,
		EndTime:   1710086400,
	})
	if err != nil {
		t.Fatalf("GetModelUsage returned error: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected non-nil response")
	}
	if repo.modelUsageReq == nil {
		t.Fatalf("expected repository to receive request")
	}
	if repo.modelUsageReq.StartTime != 1710000000 {
		t.Fatalf("start_time = %d, want 1710000000", repo.modelUsageReq.StartTime)
	}
	if repo.modelUsageReq.EndTime != 1710086400 {
		t.Fatalf("end_time = %d, want 1710086400", repo.modelUsageReq.EndTime)
	}
}

func TestGetModelUsage_RejectsMillisecondTimestamp(t *testing.T) {
	repo := &fakeStatisticsRepository{}
	svc := NewStatisticsService(repo)

	_, err := svc.GetModelUsage(context.Background(), "org-1", &dto.ModelUsageRequest{
		StartTime: 1710000000000,
		EndTime:   1710086400,
	})
	if err == nil {
		t.Fatalf("expected validation error for millisecond timestamp")
	}
	if repo.modelUsageReq != nil {
		t.Fatalf("expected repository not to be called on invalid timestamp")
	}
}

func TestGetInvocationLog_ValidatesCursorAndDefaultsLimit(t *testing.T) {
	repo := &fakeStatisticsRepository{}
	svc := NewStatisticsService(repo)
	req := &dto.InvocationLogRequest{StartTime: 1710000000, EndTime: 1710086400}
	if _, err := svc.GetInvocationLog(context.Background(), "org-1", req); err != nil {
		t.Fatalf("GetInvocationLog returned error: %v", err)
	}
	if req.Limit != 20 {
		t.Fatalf("limit = %d, want 20", req.Limit)
	}

	cursorTime := "2024-03-09T16:00:00.123456789Z"
	_, err := svc.GetInvocationLog(context.Background(), "org-1", &dto.InvocationLogRequest{
		StartTime: 1710000000, EndTime: 1710086400, CursorTime: &cursorTime,
	})
	if err == nil {
		t.Fatal("expected incomplete cursor to fail")
	}

	blankCursorID := "  "
	_, err = svc.GetInvocationLog(context.Background(), "org-1", &dto.InvocationLogRequest{
		StartTime: 1710000000, EndTime: 1710086400, CursorTime: &cursorTime, CursorID: &blankCursorID,
	})
	if err == nil {
		t.Fatal("expected blank cursor id to fail")
	}
}
