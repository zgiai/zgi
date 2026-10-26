package socialops

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

func configurationBinding(cfg Configuration, connection string, account, platform string) (PlatformBinding, bool) {
	for _, b := range cfg.Platforms {
		if b.ConnectionID.String() == connection && b.AccountID == account && b.Platform == platform {
			return b, true
		}
	}
	return PlatformBinding{}, false
}

// Standalone journal users may have no SOP configuration. The employee adapter
// requires a configured run; a publication that already bound a policy can
// never fall back to this compatibility path.
func (s *Service) publicationConfigurationSnapshot(ctx context.Context, r PublicationRequest) (int64, string, error) {
	_, err := s.GetRecord(ctx, r.Scope, "configuration", "current")
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", err
	}
	cfg, _, err := s.LoadCurrentConfiguration(ctx, r.Scope)
	if err != nil {
		return 0, "", err
	}
	if _, ok := configurationBinding(cfg, r.ConnectionID.String(), r.AccountID, r.Platform); !ok {
		return 0, "", invalid()
	}
	return cfg.Revision, cfg.Timezone, nil
}

func (s *Service) reserveWithPolicy(ctx context.Context, p Publication, revision int64) (Publication, error) {
	var result Publication
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record Record
		err := scoped(tx, p.Scope).Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind = ? AND record_key = ?", "configuration", "current").Take(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && p.ConfigurationRevision == 0 {
			result, err = (&Service{db: tx, root: s.root, now: s.now}).transition(ctx, p.Scope, p.ID, revision, "reviewed", "reserved", map[string]any{"reserved_at": s.now().UTC()})
			return err
		}
		if err != nil {
			return err
		}
		local := &Service{db: tx, root: s.root, now: s.now}
		cfg, _, err := local.LoadCurrentConfiguration(ctx, p.Scope)
		if err != nil {
			return err
		}
		if cfg.Revision != p.ConfigurationRevision {
			return conflict()
		}
		binding, ok := configurationBinding(cfg, p.ConnectionID.String(), p.AccountID, p.Platform)
		if !ok {
			return invalid()
		}
		bundle, err := local.draftBundle(ctx, p.Scope, p)
		if err != nil {
			return err
		}
		if err = local.requireApprovedImageInspections(ctx, p.Scope, p.ConfigurationRevision, bundle.Images, true); err != nil {
			return err
		}
		now := s.now().UTC()
		location, err := time.LoadLocation(cfg.Timezone)
		if err != nil {
			return invalid()
		}
		day := now.In(location)
		start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
		end := start.AddDate(0, 0, 1)
		occupied := func() *gorm.DB {
			return scoped(tx.Model(&Publication{}), p.Scope).Where("connection_id = ? AND account_id = ? AND state IN ?", p.ConnectionID, p.AccountID, []string{"reserved", "dispatching", "outcome_unknown", "remote_created", "verified", "archived"})
		}
		if bundle.Action != "reply" {
			var count int64
			if err = occupied().Where("reserved_at >= ? AND reserved_at < ? AND publication_action <> ?", start.UTC(), end.UTC(), "reply").Count(&count).Error; err != nil {
				return err
			}
			if count >= int64(binding.MaxDailyPublications) {
				return conflict()
			}
		}
		// Exact normalized public payload deduplication complements editorial topic
		// review. Target/action remain part of identity; different replies are not
		// conflated solely because they use the same short answer.
		payload, _ := json.Marshal(struct {
			Action, Target, English string
		}{Action: bundle.Action, Target: bundle.TargetID, English: strings.Join(strings.Fields(bundle.English), " ")})
		digest := hash(payload)
		var duplicates int64
		if err = occupied().Where("content_digest = ? AND reserved_at >= ?", digest, now.AddDate(0, 0, -14)).Count(&duplicates).Error; err != nil {
			return err
		}
		if duplicates != 0 {
			return conflict()
		}
		result, err = local.transition(ctx, p.Scope, p.ID, revision, "reviewed", "reserved", map[string]any{"reserved_at": now, "content_digest": digest, "publication_action": bundle.Action})
		return err
	})
	return result, err
}

// A previous-day reservation proves no send began only while its CAS state is
// still reserved. Return it to reviewed so today's quota is admitted anew.
func (s *Service) refreshUnsubmittedReservation(ctx context.Context, p Publication) (Publication, error) {
	if p.State != "reserved" || p.ConfigurationRevision == 0 {
		return p, nil
	}
	result := p
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record Record
		if err := scoped(tx, p.Scope).Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind = ? AND record_key = ?", "configuration", "current").Take(&record).Error; err != nil {
			return err
		}
		if record.Revision != p.ConfigurationRevision {
			return conflict()
		}
		local := &Service{db: tx, root: s.root, now: s.now}
		cfg, _, err := local.LoadCurrentConfiguration(ctx, p.Scope)
		if err != nil {
			return err
		}
		location, err := time.LoadLocation(cfg.Timezone)
		if err != nil || p.ReservedAt == nil {
			return invalid()
		}
		if p.ReservedAt.In(location).Format("2006-01-02") == s.now().In(location).Format("2006-01-02") {
			return nil
		}
		if p.ReservedAt.After(s.now()) {
			return conflict()
		}
		result, err = local.transition(ctx, p.Scope, p.ID, p.Revision, "reserved", "reviewed", nil)
		return err
	})
	return result, err
}
