package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
)

type PlaygroundRunListFilter = repository.PlaygroundRunListFilter

type PlaygroundProviderSummary struct {
	ProviderKey     string     `json:"provider_key"`
	AdapterName     string     `json:"adapter_name,omitempty"`
	EngineName      string     `json:"engine_name,omitempty"`
	RunCount        int        `json:"run_count"`
	SuccessCount    int        `json:"success_count"`
	DegradedCount   int        `json:"degraded_count"`
	FailedCount     int        `json:"failed_count"`
	FallbackCount   int        `json:"fallback_count"`
	AvgDurationMS   float64    `json:"avg_duration_ms"`
	AvgTextLength   float64    `json:"avg_text_length"`
	AvgElementCount float64    `json:"avg_element_count"`
	EstimatedCost   float64    `json:"estimated_cost"`
	CostCurrency    string     `json:"cost_currency"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
}

type PlaygroundRunService interface {
	Create(ctx context.Context, item *model.PlaygroundRun) error
	GetByID(ctx context.Context, id uuid.UUID, filter PlaygroundRunListFilter) (*model.PlaygroundRun, error)
	GetByShareToken(ctx context.Context, token string) (*model.PlaygroundRun, error)
	List(ctx context.Context, filter PlaygroundRunListFilter) ([]*model.PlaygroundRun, error)
	CompareBySourceHash(ctx context.Context, filter PlaygroundRunListFilter) ([]*model.PlaygroundRun, error)
	GetProviderSummary(ctx context.Context, filter PlaygroundRunListFilter) ([]PlaygroundProviderSummary, error)
	EnableShare(ctx context.Context, id uuid.UUID, filter PlaygroundRunListFilter) (*model.PlaygroundRun, error)
}

type playgroundRunService struct {
	repo repository.PlaygroundRunRepository
}

func NewPlaygroundRunService(repo repository.PlaygroundRunRepository) PlaygroundRunService {
	return &playgroundRunService{repo: repo}
}

func (s *playgroundRunService) Create(ctx context.Context, item *model.PlaygroundRun) error {
	if item.ShareToken == "" {
		item.ShareToken = newShareToken()
	}
	item.IsShareEnabled = false
	return s.repo.Create(ctx, item)
}

func (s *playgroundRunService) GetByID(ctx context.Context, id uuid.UUID, filter PlaygroundRunListFilter) (*model.PlaygroundRun, error) {
	return s.repo.GetByID(ctx, id, normalizePlaygroundListFilter(filter))
}

func (s *playgroundRunService) GetByShareToken(ctx context.Context, token string) (*model.PlaygroundRun, error) {
	return s.repo.GetByShareToken(ctx, token)
}

func (s *playgroundRunService) List(ctx context.Context, filter PlaygroundRunListFilter) ([]*model.PlaygroundRun, error) {
	return s.repo.List(ctx, normalizePlaygroundListFilter(filter))
}

func (s *playgroundRunService) CompareBySourceHash(ctx context.Context, filter PlaygroundRunListFilter) ([]*model.PlaygroundRun, error) {
	return s.repo.List(ctx, normalizePlaygroundListFilter(filter))
}

func (s *playgroundRunService) GetProviderSummary(ctx context.Context, filter PlaygroundRunListFilter) ([]PlaygroundProviderSummary, error) {
	filter = normalizePlaygroundListFilter(filter)
	if filter.Limit < 100 {
		filter.Limit = 100
	}
	items, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	type accumulator struct {
		summary           PlaygroundProviderSummary
		totalDurationMS   int
		durationCount     int
		totalTextLength   int
		totalElementCount int
	}

	accumulators := map[string]*accumulator{}
	order := []string{}
	for _, item := range items {
		if item == nil {
			continue
		}
		key := item.FinalProviderKey
		if key == "" {
			key = item.RequestedProviderKey
		}
		if key == "" {
			key = "unknown"
		}
		acc, ok := accumulators[key]
		if !ok {
			acc = &accumulator{
				summary: PlaygroundProviderSummary{
					ProviderKey:  key,
					AdapterName:  item.AdapterName,
					EngineName:   item.EngineName,
					CostCurrency: "USD",
				},
			}
			accumulators[key] = acc
			order = append(order, key)
		}
		acc.summary.RunCount++
		if item.FallbackUsed {
			acc.summary.FallbackCount++
		}
		if item.DurationMS != nil {
			acc.totalDurationMS += *item.DurationMS
			acc.durationCount++
		}
		acc.totalTextLength += readSummaryInt(item.QualitySummaryJSON, "text_length")
		acc.totalElementCount += readSummaryInt(item.QualitySummaryJSON, "element_count")
		if acc.summary.LastRunAt == nil || item.CreatedAt.After(*acc.summary.LastRunAt) {
			runAt := item.CreatedAt
			acc.summary.LastRunAt = &runAt
		}
		switch item.Status {
		case "succeeded":
			acc.summary.SuccessCount++
		case "degraded":
			acc.summary.DegradedCount++
		default:
			acc.summary.FailedCount++
		}
	}

	out := make([]PlaygroundProviderSummary, 0, len(order))
	for _, key := range order {
		acc := accumulators[key]
		if acc == nil {
			continue
		}
		if acc.durationCount > 0 {
			acc.summary.AvgDurationMS = float64(acc.totalDurationMS) / float64(acc.durationCount)
		}
		if acc.summary.RunCount > 0 {
			acc.summary.AvgTextLength = float64(acc.totalTextLength) / float64(acc.summary.RunCount)
			acc.summary.AvgElementCount = float64(acc.totalElementCount) / float64(acc.summary.RunCount)
		}
		out = append(out, acc.summary)
	}
	return out, nil
}

func (s *playgroundRunService) EnableShare(ctx context.Context, id uuid.UUID, filter PlaygroundRunListFilter) (*model.PlaygroundRun, error) {
	return s.repo.SetShareEnabled(ctx, id, normalizePlaygroundListFilter(filter), true)
}

func normalizePlaygroundListFilter(filter PlaygroundRunListFilter) PlaygroundRunListFilter {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 500 {
		filter.Limit = 500
	}
	return filter
}

func newShareToken() string {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return uuid.NewString()
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
