package socialops

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"testing"
	"time"
)

func configuredPublicationFixture(t *testing.T, maximum int) (*Service, Scope, Configuration) {
	t.Helper()
	s, scope, _, _ := fixture(t)
	cfg := validConfiguration("x-social", RequirementRequired)
	cfg.Scope, cfg.Revision, cfg.Sources, cfg.BrandArtifactRefs = scope, 1, []SourceReference{{URL: "https://example.com/product"}}, nil
	cfg.Platforms[0].MaxDailyPublications = maximum
	savePolicyConfiguration(t, s, cfg, 0)
	return s, scope, cfg
}
func savePolicyConfiguration(t *testing.T, s *Service, cfg Configuration, expected int64) {
	t.Helper()
	b, _ := json.Marshal(cfg)
	a, err := s.PutArtifact(context.Background(), cfg.Scope, "application/json", b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveRecord(context.Background(), cfg.Scope, "configuration", "current", expected, a.ID); err != nil {
		t.Fatal(err)
	}
}
func policyPublication(t *testing.T, s *Service, cfg Configuration, key, text string) Publication {
	t.Helper()
	b, _ := json.Marshal(DraftBundle{Action: "post", English: text, Chinese: "中文内部稿"})
	a, err := s.PutArtifact(context.Background(), cfg.Scope, "application/json", b)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.PreparePublication(context.Background(), PublicationRequest{Scope: cfg.Scope, ConnectionID: cfg.Platforms[0].ConnectionID, AccountID: cfg.Platforms[0].AccountID, ContentKey: key, Platform: "x", DraftArtifactID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.ReviewPublication(context.Background(), cfg.Scope, p.ID, p.Revision, a.Digest)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestConfiguredReservationEnforcesDailyAccountLimit(t *testing.T) {
	s, scope, cfg := configuredPublicationFixture(t, 1)
	first := policyPublication(t, s, cfg, "first", "first body")
	second := policyPublication(t, s, cfg, "second", "second body")
	if _, err := s.ReservePublication(context.Background(), scope, first.ID, first.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReservePublication(context.Background(), scope, second.ID, second.Revision); err == nil {
		t.Fatal("daily limit bypassed")
	}
	now := s.now()
	s.now = func() time.Time { return now.Add(24 * time.Hour) }
	if _, err := s.ReservePublication(context.Background(), scope, second.ID, second.Revision); err != nil {
		t.Fatal("new day blocked", err)
	}
}
func TestConfiguredReservationRejectsChangedPolicyAndDuplicateBody(t *testing.T) {
	s, scope, cfg := configuredPublicationFixture(t, 10)
	first := policyPublication(t, s, cfg, "first", "same body")
	duplicate := policyPublication(t, s, cfg, "duplicate", " same   body ")
	stale := policyPublication(t, s, cfg, "stale", "different body")
	if _, err := s.ReservePublication(context.Background(), scope, first.ID, first.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReservePublication(context.Background(), scope, duplicate.ID, duplicate.Revision); err == nil {
		t.Fatal("duplicate text bypassed under new key")
	}
	cfg.Revision = 2
	cfg.EmployeeVersionID = uuid.New()
	savePolicyConfiguration(t, s, cfg, 1)
	if _, err := s.ReservePublication(context.Background(), scope, stale.ID, stale.Revision); err == nil {
		t.Fatal("stale policy published")
	}
}

func TestConfiguredDispatchRejectsChangedPolicyOrDifferentDay(t *testing.T) {
	for _, mode := range []string{"revision", "day"} {
		t.Run(mode, func(t *testing.T) {
			s, scope, cfg := configuredPublicationFixture(t, 2)
			p := policyPublication(t, s, cfg, "first", "body")
			p, err := s.ReservePublication(context.Background(), scope, p.ID, p.Revision)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "revision" {
				cfg.Revision = 2
				savePolicyConfiguration(t, s, cfg, 1)
			} else {
				now := s.now()
				s.now = func() time.Time { return now.Add(24 * time.Hour) }
			}
			if _, err = s.StartDispatch(context.Background(), scope, p.ID, p.Revision); err == nil {
				t.Fatal("stale reservation dispatched")
			}
		})
	}
}

func TestPublishSafelyRenewsUnsubmittedPreviousDayReservation(t *testing.T) {
	s, scope, cfg := configuredPublicationFixture(t, 1)
	p := policyPublication(t, s, cfg, "first", "body")
	p, err := s.ReservePublication(context.Background(), scope, p.ID, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	now := s.now().Add(24 * time.Hour)
	s.now = func() time.Time { return now }
	gateway := &gatewaySpy{post: RemotePost{ID: "remote1", URL: "https://x.com/account/status/remote1", Text: "body", PublishedAt: now}}
	completed, err := s.Publish(context.Background(), scope, p.ID, gateway, cfg.Timezone)
	if err != nil || completed.State != "archived" || gateway.sends != 1 {
		t.Fatal(completed, err, gateway.sends)
	}
}

func TestConfiguredArchiveTimezoneCannotBeOverriddenByCaller(t *testing.T) {
	s, scope, cfg := configuredPublicationFixture(t, 1)
	cfg.Revision = 2
	cfg.Timezone = "America/Los_Angeles"
	savePolicyConfiguration(t, s, cfg, 1)
	p := policyPublication(t, s, cfg, "timezone", "body")
	now := s.now()
	gateway := &gatewaySpy{post: RemotePost{ID: "zone1", URL: "https://x.com/account/status/zone1", Text: "body", PublishedAt: now}}
	completed, err := s.Publish(context.Background(), scope, p.ID, gateway, "UTC")
	if err != nil || completed.ArchiveDate != "2026-10-19" {
		t.Fatal("caller changed archive date", completed.ArchiveDate, err)
	}
}
