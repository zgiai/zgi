package service

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/dto"
	modelpricing "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/pricing"
)

type pricingIssueRow struct {
	ID                uuid.UUID `gorm:"column:id"`
	Provider          string    `gorm:"column:provider"`
	Model             string    `gorm:"column:name"`
	ModelName         string    `gorm:"column:display_name"`
	UseCases          string    `gorm:"column:use_cases"`
	Pricing           []byte    `gorm:"column:pricing"`
	InputPrice          *float64  `gorm:"column:input_price"`
	OutputPrice         *float64  `gorm:"column:output_price"`
	CachedInputPrice    *float64  `gorm:"column:cached_input_price"`
	CacheReadPrice      *float64  `gorm:"column:cache_read_price"`
	CacheWritePrice     *float64  `gorm:"column:cache_write_price"`
	CacheWrite5mPrice   *float64  `gorm:"column:cache_write_5m_price"`
	CacheWrite1hPrice   *float64  `gorm:"column:cache_write_1h_price"`
}

func (s *modelService) ListPricingIssues(ctx context.Context) (*dto.PricingIssuesListResponse, error) {
	var rows []pricingIssueRow
	if err := s.db.WithContext(ctx).
		Table("llm_models").
		Select("id, provider, name, display_name, use_cases, pricing, input_price, output_price, cached_input_price, cache_read_price, cache_write_price, cache_write_5m_price, cache_write_1h_price").
		Where("deleted_at IS NULL AND is_active = ?", true).
		Order("provider ASC, name ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]dto.PricingIssueResponse, 0)
	for _, row := range rows {
		useCases := decodePricingIssueUseCases(row.UseCases)
		if len(useCases) == 0 {
			continue
		}
		cacheRead := row.CacheReadPrice
		if cacheRead == nil {
			cacheRead = row.CachedInputPrice
		}
		scalars := modelpricing.LegacyScalarPrices{
			InputPrice:        row.InputPrice,
			OutputPrice:       row.OutputPrice,
			CachedInputPrice:  row.CachedInputPrice,
			CacheReadPrice:    cacheRead,
			CacheWritePrice:   row.CacheWritePrice,
			CacheWrite5mPrice: row.CacheWrite5mPrice,
			CacheWrite1hPrice: row.CacheWrite1hPrice,
		}
		missing := modelpricing.MissingPricingForUseCases(useCases, json.RawMessage(row.Pricing), scalars)
		if len(missing) == 0 {
			continue
		}
		issueItems := make([]dto.PricingIssueItem, 0, len(missing))
		for _, item := range missing {
			issueItems = append(issueItems, dto.PricingIssueItem{
				UseCase:   item.UseCase,
				Operation: item.Operation,
				Meter:     item.Meter,
			})
		}
		items = append(items, dto.PricingIssueResponse{
			ID:         row.ID,
			Provider:   row.Provider,
			Model:      row.Model,
			ModelName:  row.ModelName,
			UseCases:   useCases,
			Missing:    issueItems,
			Configured: modelpricing.PricingHasConfiguredRules(json.RawMessage(row.Pricing)),
		})
	}

	return &dto.PricingIssuesListResponse{
		Items: items,
		Total: len(items),
	}, nil
}

func decodePricingIssueUseCases(raw string) []string {
	if raw == "" {
		return nil
	}
	var decoded []string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	return decoded
}
