package socialops

import (
	"context"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestPublicationHistoryRestoresSamplesAndPreservesMissing(t *testing.T) {
	s, scope, p, gateway := publishFixture(t)
	done, err := s.Publish(context.Background(), scope, p.ID, gateway, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	since, until := done.PublishedAt.Add(-time.Hour), done.PublishedAt.Add(time.Hour)
	rows, err := s.ListPublicationObservations(context.Background(), scope, since, until, uuid.Nil, 10)
	if err != nil || len(rows) != 1 || rows[0].Publication.ID != p.ID || rows[0].Metric != nil {
		t.Fatal(rows, err)
	}
	now := done.PublishedAt.Add(7 * 24 * time.Hour)
	s.now = func() time.Time { return now }
	count := int64(19)
	if _, err = s.RecordMetric(context.Background(), scope, p.ID, map[string]*int64{"likes": &count, "clicks": nil}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.ListPublicationObservations(context.Background(), scope, since, until, uuid.Nil, 10)
	if err != nil || rows[0].Metric == nil || rows[0].Metric.ValuesJSON != `{"clicks":null,"likes":19}` {
		t.Fatal(rows, err)
	}
	rows, err = s.ListPublicationObservations(context.Background(), scope, since, until, p.ID, 10)
	if err != nil || len(rows) != 0 {
		t.Fatal("cursor repeated item", rows, err)
	}
	foreign := scope
	foreign.OwnerID = uuid.New()
	if _, err = s.ListPublicationObservations(context.Background(), foreign, since, until, p.ID, 10); err == nil {
		t.Fatal("cross-scope cursor accepted")
	}
}
