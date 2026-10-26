package socialops

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type PublicationObservation struct {
	Publication Publication `json:"publication"`
	Metric      *Metric     `json:"metric"`
}

// ListPublicationObservations reads the durable source of weekly review.
// Missing samples stay absent; this path never observes the live platform.
func (s *Service) ListPublicationObservations(ctx context.Context, scope Scope, since, until time.Time, after uuid.UUID, limit int) ([]PublicationObservation, error) {
	if !validScope(scope) || since.IsZero() || until.IsZero() || !until.After(since) || until.Sub(since) > 31*24*time.Hour || limit < 1 || limit > 100 {
		return nil, invalid()
	}
	query := scoped(s.db.WithContext(ctx), scope).Where("state IN ? AND published_at >= ? AND published_at < ?", []string{"verified", "archived"}, since.UTC(), until.UTC())
	if after != uuid.Nil {
		previous, err := s.GetPublication(ctx, scope, after)
		if err != nil {
			return nil, err
		}
		if previous.PublishedAt == nil || previous.PublishedAt.Before(since) || !previous.PublishedAt.Before(until) {
			return nil, invalid()
		}
		query = query.Where("(published_at > ? OR (published_at = ? AND id > ?))", previous.PublishedAt, previous.PublishedAt, previous.ID)
	}
	var publications []Publication
	if err := query.Order("published_at,id").Limit(limit).Find(&publications).Error; err != nil {
		return nil, err
	}
	result := make([]PublicationObservation, 0, len(publications))
	if len(publications) == 0 {
		return result, nil
	}
	ids := make([]uuid.UUID, 0, len(publications))
	for _, p := range publications {
		ids = append(ids, p.ID)
	}
	var metrics []Metric
	if err := s.db.WithContext(ctx).Where("publication_id IN ?", ids).Find(&metrics).Error; err != nil {
		return nil, err
	}
	samples := map[uuid.UUID]Metric{}
	for _, metric := range metrics {
		samples[metric.PublicationID] = metric
	}
	for _, p := range publications {
		item := PublicationObservation{Publication: p}
		if metric, ok := samples[p.ID]; ok {
			sample := metric
			item.Metric = &sample
		}
		result = append(result, item)
	}
	return result, nil
}
