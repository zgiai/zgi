package service

import (
	"context"
	"testing"

	"github.com/zgiai/ginext/internal/modules/llm/statistics/dto"
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
