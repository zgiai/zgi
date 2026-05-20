package officialmodel

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/config"
	consoleintf "github.com/zgiai/zgi/api/internal/infra/platform/console"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"gorm.io/gorm"
)

const (
	SourceKeyZGICloud = "ZGI_CLOUD"

	CheckStatusAccepted = "accepted"
	CheckStatusRejected = "rejected"

	RejectReasonEmptyModels    = "empty_models"
	RejectReasonAbnormalShrink = "abnormal_shrink"

	defaultMinAcceptRatio = 0.7
)

type Snapshot struct {
	SourceKey          string     `gorm:"column:source_key;type:varchar(50);primaryKey"`
	EffectiveModels    []string   `gorm:"column:effective_models;type:jsonb;serializer:json;default:'[]'"`
	LatestModels       []string   `gorm:"column:latest_models;type:jsonb;serializer:json;default:'[]'"`
	PreviousModels     []string   `gorm:"column:previous_models;type:jsonb;serializer:json;default:'[]'"`
	LatestEventVersion int64      `gorm:"column:latest_event_version;not null;default:0"`
	LatestSyncedAt     *time.Time `gorm:"column:latest_synced_at"`
	EffectiveUpdatedAt *time.Time `gorm:"column:effective_updated_at"`
	LastCheckStatus    string     `gorm:"column:last_check_status;type:varchar(20);not null;default:'accepted'"`
	LastRejectReason   string     `gorm:"column:last_reject_reason;type:text"`
	CreatedAt          time.Time  `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt          time.Time  `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP"`
}

func (Snapshot) TableName() string {
	return "llm_official_model_snapshots"
}

type UpstreamChannel struct {
	ID     string
	Models []string
}

type SyncMeta struct {
	Version  int64
	SyncedAt time.Time
}

func SyncFromConsoleProvider(ctx context.Context, db *gorm.DB, provider consoleintf.ConsoleProvider) (*Snapshot, error) {
	if provider == nil || !provider.IsAvailable() {
		return nil, nil
	}

	resp, err := provider.ListPlatformChannels(ctx)
	if err != nil {
		return nil, err
	}

	channels := make([]UpstreamChannel, 0, len(resp.Channels))
	for _, ch := range resp.Channels {
		if ch == nil {
			continue
		}
		channels = append(channels, UpstreamChannel{
			ID:     ch.ID,
			Models: ch.Models,
		})
	}

	return SyncFromChannels(ctx, db, channels, SyncMeta{
		SyncedAt: time.Now().UTC(),
	})
}

func SyncFromChannels(ctx context.Context, db *gorm.DB, channels []UpstreamChannel, meta SyncMeta) (*Snapshot, error) {
	if db == nil {
		return nil, errors.New("official model snapshot requires a database")
	}

	aggregated := aggregateModels(channels)
	now := meta.SyncedAt.UTC()
	if meta.SyncedAt.IsZero() {
		now = time.Now().UTC()
	}

	var snapshot Snapshot
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.
			Where("source_key = ?", SourceKeyZGICloud).
			First(&snapshot).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			snapshot = Snapshot{
				SourceKey:       SourceKeyZGICloud,
				LastCheckStatus: CheckStatusAccepted,
			}
		}

		snapshot.LatestModels = aggregated
		snapshot.LatestEventVersion = meta.Version
		snapshot.LatestSyncedAt = &now

		if accepted, reason := shouldAccept(snapshot.EffectiveModels, aggregated); accepted {
			snapshot.PreviousModels = cloneStrings(snapshot.EffectiveModels)
			snapshot.EffectiveModels = cloneStrings(aggregated)
			snapshot.LastCheckStatus = CheckStatusAccepted
			snapshot.LastRejectReason = ""
			snapshot.EffectiveUpdatedAt = &now
		} else {
			snapshot.LastCheckStatus = CheckStatusRejected
			snapshot.LastRejectReason = reason
		}

		return tx.WithContext(ctx).Save(&snapshot).Error
	})
	if err != nil {
		return nil, err
	}

	return &snapshot, nil
}

func GetEffectiveModels(ctx context.Context, db *gorm.DB) ([]string, error) {
	if db == nil {
		return nil, errors.New("official model snapshot requires a database")
	}

	var snapshot Snapshot
	err := db.WithContext(ctx).
		Where("source_key = ?", SourceKeyZGICloud).
		First(&snapshot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []string{}, nil
		}
		return nil, err
	}

	return cloneStrings(snapshot.EffectiveModels), nil
}

func HasEffectiveModels(ctx context.Context, db *gorm.DB) (bool, error) {
	models, err := GetEffectiveModels(ctx, db)
	if err != nil {
		return false, err
	}
	return len(models) > 0, nil
}

func HydrateRoute(ctx context.Context, db *gorm.DB, route *channelmodel.LLMRoute) error {
	if route == nil || !route.IsOfficial {
		return nil
	}

	models, err := GetEffectiveModels(ctx, db)
	if err != nil {
		return err
	}
	route.Models = models
	return nil
}

func HydrateRoutes(ctx context.Context, db *gorm.DB, routes []*channelmodel.LLMRoute) error {
	if len(routes) == 0 {
		return nil
	}

	models, err := GetEffectiveModels(ctx, db)
	if err != nil {
		return err
	}

	for _, route := range routes {
		if route != nil && route.IsOfficial {
			route.Models = cloneStrings(models)
		}
	}

	return nil
}

func HydrateRouteValues(ctx context.Context, db *gorm.DB, routes []channelmodel.LLMRoute) error {
	if len(routes) == 0 {
		return nil
	}

	models, err := GetEffectiveModels(ctx, db)
	if err != nil {
		return err
	}

	for i := range routes {
		if routes[i].IsOfficial {
			routes[i].Models = cloneStrings(models)
		}
	}

	return nil
}

func ContainsModel(models []string, modelName string) bool {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return false
	}

	for _, model := range models {
		if model == "*" || model == target {
			return true
		}
	}
	return false
}

func aggregateModels(channels []UpstreamChannel) []string {
	seen := make(map[string]struct{})
	aggregated := make([]string, 0)

	for _, ch := range channels {
		for _, model := range ch.Models {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			if _, ok := seen[model]; ok {
				continue
			}
			seen[model] = struct{}{}
			aggregated = append(aggregated, model)
		}
	}

	sort.Strings(aggregated)
	return aggregated
}

func shouldAccept(current, latest []string) (bool, string) {
	if !isOfficialModelStrictSyncEnabled() {
		return true, ""
	}
	if len(current) == 0 {
		return true, ""
	}
	if len(latest) == 0 {
		return false, RejectReasonEmptyModels
	}

	minAllowed := int(math.Ceil(float64(len(current)) * defaultMinAcceptRatio))
	if len(latest) < minAllowed {
		return false, RejectReasonAbnormalShrink
	}

	return true, ""
}

func isOfficialModelStrictSyncEnabled() bool {
	return config.Current().LLM.OfficialModelStrictSync
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
