package socialops

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MetricsGateway interface {
	ObserveMetrics(context.Context, Publication) (map[string]*int64, error)
}

// CaptureMetrics uses a real platform observation and preserves the first due
// sample. Subsequent weekly runs do not replace seven-day counts with newer
// cumulative values. No model-authored metric values enter this tool path.
func (s *Service) CaptureMetrics(ctx context.Context, scope Scope, id uuid.UUID, gateway PublicationGateway) (Metric, error) {
	p, err := s.GetPublication(ctx, scope, id)
	if err != nil {
		return Metric{}, err
	}
	var existing Metric
	err = s.db.WithContext(ctx).Where("publication_id = ?", p.ID).Take(&existing).Error
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return Metric{}, err
	}
	if p.PublishedAt == nil || (p.State != "verified" && p.State != "archived") || s.now().Before(p.PublishedAt.Add(7*24*time.Hour)) {
		return Metric{}, invalid()
	}
	reader, ok := gateway.(MetricsGateway)
	if !ok {
		return Metric{}, invalid()
	}
	values, err := reader.ObserveMetrics(ctx, p)
	if err != nil {
		return Metric{}, err
	}
	return s.RecordMetric(ctx, scope, p.ID, values)
}
