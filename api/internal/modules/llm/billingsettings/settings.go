package billingsettings

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// KeyPointsPerUSD is the llm_billing_settings key holding the
	// Console-authoritative discount rate: how many points one USD of metered
	// usage costs.
	KeyPointsPerUSD = "points_per_usd"
	// DefaultPointsPerUSD is the compiled-in fallback used before the first
	// catalog sync and while Console is unreachable. It matches the historical
	// effective rate so reading it is behavior-neutral.
	DefaultPointsPerUSD int64 = 1000
)

// Setting is one row of public.llm_billing_settings.
type Setting struct {
	Key       string    `gorm:"column:key;primaryKey;size:64"`
	Value     string    `gorm:"column:value;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

// TableName pins the model to the migration-owned settings table.
func (Setting) TableName() string { return "llm_billing_settings" }

// Store is Pre's local read cache of billing settings. The catalogsync
// synchronizer is its only writer.
type Store struct {
	db           *gorm.DB
	mu           sync.RWMutex
	pointsPerUSD int64
	hasValue     bool
}

var (
	defaultMu    sync.RWMutex
	defaultStore *Store
)

// Init loads settings from the database, installs the resulting store as the
// process default, and returns it. Load failures fall back to
// DefaultPointsPerUSD with a warning instead of failing startup.
func Init(ctx context.Context, db *gorm.DB) *Store {
	store := &Store{db: db, pointsPerUSD: DefaultPointsPerUSD}
	store.load(ctx)

	defaultMu.Lock()
	defaultStore = store
	defaultMu.Unlock()

	return store
}

// Default returns the process default store, or a read-only fallback yielding
// DefaultPointsPerUSD before Init has run.
func Default() *Store {
	defaultMu.RLock()
	store := defaultStore
	defaultMu.RUnlock()
	if store == nil {
		return &Store{pointsPerUSD: DefaultPointsPerUSD}
	}
	return store
}

func (s *Store) load(ctx context.Context) {
	if s == nil || s.db == nil {
		return
	}

	var setting Setting
	if err := s.db.WithContext(ctx).Where("key = ?", KeyPointsPerUSD).Limit(1).Find(&setting).Error; err != nil {
		log.Printf("[billingsettings] load %s failed, using default %d: %v", KeyPointsPerUSD, DefaultPointsPerUSD, err)
		return
	}
	if setting.Key == "" {
		log.Printf("[billingsettings] %s missing in llm_billing_settings, using default %d", KeyPointsPerUSD, DefaultPointsPerUSD)
		return
	}
	rate, err := strconv.ParseInt(strings.TrimSpace(setting.Value), 10, 64)
	if err != nil || rate <= 0 {
		log.Printf("[billingsettings] %s=%q is not a positive integer, using default %d", KeyPointsPerUSD, setting.Value, DefaultPointsPerUSD)
		return
	}

	s.mu.Lock()
	s.pointsPerUSD = rate
	s.hasValue = true
	s.mu.Unlock()
}

// PointsPerUSD returns the cached rate. It never returns a non-positive value.
func (s *Store) PointsPerUSD() int64 {
	if s == nil {
		return DefaultPointsPerUSD
	}
	s.mu.RLock()
	rate := s.pointsPerUSD
	s.mu.RUnlock()
	if rate <= 0 {
		return DefaultPointsPerUSD
	}
	return rate
}

// HasValue reports whether a valid rate was loaded or written.
func (s *Store) HasValue() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasValue
}

// StoreCatalogRate persists the rate delivered by the published catalog and
// refreshes the cache. A non-positive rate (an older Console that does not send
// the field yet) is ignored so a mixed-version rollout cannot zero the rate.
func (s *Store) StoreCatalogRate(ctx context.Context, rate int64) error {
	if s == nil || s.db == nil || rate <= 0 {
		return nil
	}

	value := strconv.FormatInt(rate, 10)
	now := time.Now()
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"value":      value,
			"updated_at": now,
		}),
	}).Create(&Setting{Key: KeyPointsPerUSD, Value: value, UpdatedAt: now}).Error
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.pointsPerUSD = rate
	s.hasValue = true
	s.mu.Unlock()
	return nil
}
