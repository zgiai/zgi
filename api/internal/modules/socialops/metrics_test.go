package socialops

import (
	"context"
	"testing"
	"time"
)

type metricGatewaySpy struct {
	gatewaySpy
	observations int
}

func (g *metricGatewaySpy) ObserveMetrics(context.Context, Publication) (map[string]*int64, error) {
	g.observations++
	likes := int64(5)
	return map[string]*int64{"likes": &likes, "impressions": nil}, nil
}
func TestCaptureMetricsReadsOnlyWhenDueAndDoesNotOverwriteSample(t *testing.T) {
	s, scope, p, g := publishFixture(t)
	ctx := context.Background()
	now := s.now()
	gateway := &metricGatewaySpy{gatewaySpy: *g}
	p, err := s.Publish(ctx, scope, p.ID, gateway, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CaptureMetrics(ctx, scope, p.ID, gateway); err == nil || gateway.observations != 0 {
		t.Fatal("early provider metric read", err)
	}
	s.now = func() time.Time { return now.Add(7 * 24 * time.Hour) }
	m, err := s.CaptureMetrics(ctx, scope, p.ID, gateway)
	if err != nil || gateway.observations != 1 || m.Late || m.ValuesJSON != `{"impressions":null,"likes":5}` {
		t.Fatal(m, err)
	}
	later, err := s.CaptureMetrics(ctx, scope, p.ID, gateway)
	if err != nil || gateway.observations != 1 || later.ID != m.ID {
		t.Fatal("re-read cumulative metrics", later, err)
	}
}
