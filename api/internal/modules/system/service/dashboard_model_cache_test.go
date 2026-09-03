package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/system/model"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

type dashboardAvailableModelsSequence struct {
	calls int
}

func (s *dashboardAvailableModelsSequence) ListAvailable(context.Context, uuid.UUID, string, string) ([]*llmmodelsvc.AvailableModel, error) {
	s.calls++
	if s.calls == 1 {
		return []*llmmodelsvc.AvailableModel{}, nil
	}
	return []*llmmodelsvc.AvailableModel{{Name: "ready-model", UseCases: []string{"text-chat"}}}, nil
}

func TestDashboardStatsUsesShortNegativeCacheForZeroAvailableModels(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	previousRedis := redisutil.GetClient()
	redisutil.SetClient(redisClient)
	t.Cleanup(func() { redisutil.SetClient(previousRedis) })

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	models := &dashboardAvailableModelsSequence{}
	svc := NewDashboardServiceWithAvailableModels(db, models).(*dashboardService)
	svc.tableCache["chat_runtime_conversations"] = false
	scopes := model.DashboardWorkspaceScopes{}
	organizationID := uuid.NewString()

	first, err := svc.GetDashboardStats(t.Context(), organizationID, "account-1", scopes)
	require.NoError(t, err)
	require.Zero(t, first.Models.Total)

	second, err := svc.GetDashboardStats(t.Context(), organizationID, "account-1", scopes)
	require.NoError(t, err)
	require.Zero(t, second.Models.Total)
	require.Equal(t, 1, models.calls, "an immediate repeat should use the bounded negative cache")

	redisServer.FastForward(dashboardZeroModelCacheTTL + time.Second)
	third, err := svc.GetDashboardStats(t.Context(), organizationID, "account-1", scopes)
	require.NoError(t, err)
	require.EqualValues(t, 1, third.Models.Total)
	require.Equal(t, 2, models.calls, "models must become visible after the short negative cache expires")
}
