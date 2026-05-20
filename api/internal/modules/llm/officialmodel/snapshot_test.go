package officialmodel

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSnapshotTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Snapshot{}))

	return db
}

func setSnapshotTestStrictMode(t *testing.T, enabled bool) {
	t.Helper()
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		LLM: config.LLMConfig{OfficialModelStrictSync: enabled},
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})
}

func TestSyncFromChannelsCreatesInitialEffectiveSnapshot(t *testing.T) {
	setSnapshotTestStrictMode(t, false)
	db := setupSnapshotTestDB(t)
	now := time.Unix(1_700_000_000, 0).UTC()

	snapshot, err := SyncFromChannels(context.Background(), db, []UpstreamChannel{
		{ID: "ch-1", Models: []string{"gpt-4o", "gpt-4.1"}},
		{ID: "ch-2", Models: []string{"gpt-4o-mini", "gpt-4o"}},
	}, SyncMeta{
		Version:  12,
		SyncedAt: now,
	})
	require.NoError(t, err)
	require.Equal(t, CheckStatusAccepted, snapshot.LastCheckStatus)
	require.ElementsMatch(t, []string{"gpt-4o", "gpt-4.1", "gpt-4o-mini"}, snapshot.EffectiveModels)
	require.ElementsMatch(t, snapshot.EffectiveModels, snapshot.LatestModels)
	require.Empty(t, snapshot.PreviousModels)
	require.Equal(t, int64(12), snapshot.LatestEventVersion)
	require.NotNil(t, snapshot.EffectiveUpdatedAt)
	require.Equal(t, now, snapshot.LatestSyncedAt.UTC())
}

func TestSyncFromChannelsAcceptsAbnormalShrinkWhenStrictModeDisabled(t *testing.T) {
	setSnapshotTestStrictMode(t, false)
	db := setupSnapshotTestDB(t)
	ctx := context.Background()

	_, err := SyncFromChannels(ctx, db, []UpstreamChannel{
		{ID: "ch-1", Models: []string{"gpt-4o", "gpt-4.1", "o1", "o3-mini"}},
	}, SyncMeta{
		Version:  1,
		SyncedAt: time.Unix(1_700_000_000, 0).UTC(),
	})
	require.NoError(t, err)

	snapshot, err := SyncFromChannels(ctx, db, []UpstreamChannel{
		{ID: "ch-1", Models: []string{"gpt-4o"}},
	}, SyncMeta{
		Version:  2,
		SyncedAt: time.Unix(1_700_000_600, 0).UTC(),
	})
	require.NoError(t, err)
	require.Equal(t, CheckStatusAccepted, snapshot.LastCheckStatus)
	require.Empty(t, snapshot.LastRejectReason)
	require.ElementsMatch(t, []string{"gpt-4o"}, snapshot.EffectiveModels)
	require.ElementsMatch(t, []string{"gpt-4o"}, snapshot.LatestModels)
	require.ElementsMatch(t, []string{"gpt-4o", "gpt-4.1", "o1", "o3-mini"}, snapshot.PreviousModels)
}

func TestSyncFromChannelsRejectsAbnormalShrinkAndKeepsEffectiveSnapshotWhenStrictModeEnabled(t *testing.T) {
	setSnapshotTestStrictMode(t, true)
	db := setupSnapshotTestDB(t)
	ctx := context.Background()

	_, err := SyncFromChannels(ctx, db, []UpstreamChannel{
		{ID: "ch-1", Models: []string{"gpt-4o", "gpt-4.1", "o1", "o3-mini"}},
	}, SyncMeta{
		Version:  1,
		SyncedAt: time.Unix(1_700_000_000, 0).UTC(),
	})
	require.NoError(t, err)

	snapshot, err := SyncFromChannels(ctx, db, []UpstreamChannel{
		{ID: "ch-1", Models: []string{"gpt-4o"}},
	}, SyncMeta{
		Version:  2,
		SyncedAt: time.Unix(1_700_000_600, 0).UTC(),
	})
	require.NoError(t, err)
	require.Equal(t, CheckStatusRejected, snapshot.LastCheckStatus)
	require.Equal(t, RejectReasonAbnormalShrink, snapshot.LastRejectReason)
	require.ElementsMatch(t, []string{"gpt-4o", "gpt-4.1", "o1", "o3-mini"}, snapshot.EffectiveModels)
	require.ElementsMatch(t, []string{"gpt-4o"}, snapshot.LatestModels)
	require.Empty(t, snapshot.PreviousModels)
}

func TestSyncFromChannelsAcceptsEmptyModelsWhenStrictModeDisabled(t *testing.T) {
	setSnapshotTestStrictMode(t, false)
	db := setupSnapshotTestDB(t)
	ctx := context.Background()

	_, err := SyncFromChannels(ctx, db, []UpstreamChannel{
		{ID: "ch-1", Models: []string{"gpt-4o", "gpt-4.1"}},
	}, SyncMeta{
		Version:  1,
		SyncedAt: time.Unix(1_700_000_000, 0).UTC(),
	})
	require.NoError(t, err)

	snapshot, err := SyncFromChannels(ctx, db, nil, SyncMeta{
		Version:  2,
		SyncedAt: time.Unix(1_700_000_600, 0).UTC(),
	})
	require.NoError(t, err)
	require.Equal(t, CheckStatusAccepted, snapshot.LastCheckStatus)
	require.Empty(t, snapshot.LastRejectReason)
	require.Empty(t, snapshot.EffectiveModels)
	require.Empty(t, snapshot.LatestModels)
	require.ElementsMatch(t, []string{"gpt-4o", "gpt-4.1"}, snapshot.PreviousModels)
}

func TestSyncFromChannelsRejectsEmptyModelsWhenStrictModeEnabled(t *testing.T) {
	setSnapshotTestStrictMode(t, true)
	db := setupSnapshotTestDB(t)
	ctx := context.Background()

	_, err := SyncFromChannels(ctx, db, []UpstreamChannel{
		{ID: "ch-1", Models: []string{"gpt-4o", "gpt-4.1"}},
	}, SyncMeta{
		Version:  1,
		SyncedAt: time.Unix(1_700_000_000, 0).UTC(),
	})
	require.NoError(t, err)

	snapshot, err := SyncFromChannels(ctx, db, nil, SyncMeta{
		Version:  2,
		SyncedAt: time.Unix(1_700_000_600, 0).UTC(),
	})
	require.NoError(t, err)
	require.Equal(t, CheckStatusRejected, snapshot.LastCheckStatus)
	require.Equal(t, RejectReasonEmptyModels, snapshot.LastRejectReason)
	require.ElementsMatch(t, []string{"gpt-4o", "gpt-4.1"}, snapshot.EffectiveModels)
	require.Empty(t, snapshot.LatestModels)
	require.Empty(t, snapshot.PreviousModels)
}
