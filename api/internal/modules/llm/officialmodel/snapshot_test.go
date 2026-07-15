package officialmodel

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	consoleintf "github.com/zgiai/zgi/api/internal/infra/platform/console"
	llmcache "github.com/zgiai/zgi/api/internal/modules/llm/cache"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type consoleProviderStub struct {
	consoleintf.ConsoleProvider
	channels []*consoleintf.PlatformChannelInfo
}

func (f *consoleProviderStub) IsAvailable() bool { return true }

func (f *consoleProviderStub) ListPlatformChannels(context.Context) (*consoleintf.PlatformChannelsResponse, error) {
	return &consoleintf.PlatformChannelsResponse{Channels: f.channels}, nil
}

type channelRPCClientStub struct {
	channels []*pb.OfficialChannel
}

func (f *channelRPCClientStub) ListOfficialChannels(context.Context, *pb.ListOfficialChannelsRequest) (*pb.ListOfficialChannelsResponse, error) {
	return &pb.ListOfficialChannelsResponse{Channels: f.channels}, nil
}

func (f *channelRPCClientStub) WatchChannels(context.Context, *pb.WatchChannelsRequest) (channelEventStream, error) {
	return nil, nil
}

type providerModelJSON struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func TestSyncFromConsoleProviderPreservesProviderModelPairs(t *testing.T) {
	db := openOfficialSnapshotDB(t)
	provider := &consoleProviderStub{channels: []*consoleintf.PlatformChannelInfo{
		{ID: "openai", Provider: "openai", Models: []string{"same-name", "Pro/gpt-4.1"}},
		{ID: "anthropic", Provider: "anthropic", Models: []string{"same-name"}},
	}}

	if _, err := SyncFromConsoleProvider(context.Background(), db, provider); err != nil {
		t.Fatalf("SyncFromConsoleProvider returned error: %v", err)
	}

	assertEffectiveProviderModels(t, db, []providerModelJSON{
		{Provider: "anthropic", Model: "same-name"},
		{Provider: "openai", Model: "Pro/gpt-4.1"},
		{Provider: "openai", Model: "same-name"},
	})
}

func TestSynchronizerFullRefreshPreservesProviderModelPairs(t *testing.T) {
	db := openOfficialSnapshotDB(t)
	client := &channelRPCClientStub{channels: []*pb.OfficialChannel{
		{Id: "openai", Provider: "openai", Models: []string{"same-name"}},
		{Id: "anthropic", Provider: "anthropic", Models: []string{"same-name"}},
	}}
	syncer := newSynchronizerWithClient(db, client)

	if err := syncer.fullRefresh(context.Background(), client, SyncMeta{SyncedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("fullRefresh returned error: %v", err)
	}

	assertEffectiveProviderModels(t, db, []providerModelJSON{
		{Provider: "anthropic", Model: "same-name"},
		{Provider: "openai", Model: "same-name"},
	})
}

func TestHydrateRouteIncludesOfficialProviderModelPairs(t *testing.T) {
	db := openOfficialSnapshotDB(t)
	if _, err := SyncFromChannels(context.Background(), db, []UpstreamChannel{
		{ID: "openai", Provider: "openai", Models: []string{"same-name"}},
		{ID: "anthropic", Provider: "anthropic", Models: []string{"same-name"}},
	}, SyncMeta{}); err != nil {
		t.Fatalf("SyncFromChannels returned error: %v", err)
	}
	route := &channelmodel.LLMRoute{Type: shared.RouteTypeZGICloud, IsOfficial: true}

	if err := HydrateRoute(context.Background(), db, route); err != nil {
		t.Fatalf("HydrateRoute returned error: %v", err)
	}

	want := []channelmodel.ProviderModel{
		{Provider: "anthropic", Model: "same-name"},
		{Provider: "openai", Model: "same-name"},
	}
	if len(route.OfficialProviderModels) != len(want) {
		t.Fatalf("hydrated provider-model pairs = %#v, want %#v", route.OfficialProviderModels, want)
	}
	for i := range want {
		if route.OfficialProviderModels[i] != want[i] {
			t.Fatalf("hydrated provider-model pair %d = %#v, want %#v", i, route.OfficialProviderModels[i], want[i])
		}
	}
}

func TestSyncFromChannelsInvalidatesGlobalCacheOnlyWhenEffectiveProviderModelsChange(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previous := redisutil.GetClient()
	redisutil.SetClient(client)
	t.Cleanup(func() {
		redisutil.SetClient(previous)
		_ = client.Close()
	})

	ctx := context.Background()
	db := openOfficialSnapshotDB(t)
	if _, err := SyncFromChannels(ctx, db, []UpstreamChannel{
		{ID: "one", Provider: "openai", Models: []string{"same-name"}},
	}, SyncMeta{}); err != nil {
		t.Fatalf("initial SyncFromChannels returned error: %v", err)
	}
	initialGeneration := llmcache.GlobalGeneration(ctx)

	if _, err := SyncFromChannels(ctx, db, []UpstreamChannel{
		{ID: "one", Provider: "openai", Models: []string{"same-name"}},
	}, SyncMeta{}); err != nil {
		t.Fatalf("unchanged SyncFromChannels returned error: %v", err)
	}
	if got := llmcache.GlobalGeneration(ctx); got != initialGeneration {
		t.Fatalf("global generation after unchanged mapping = %q, want %q", got, initialGeneration)
	}

	if _, err := SyncFromChannels(ctx, db, []UpstreamChannel{
		{ID: "one", Provider: "anthropic", Models: []string{"same-name"}},
	}, SyncMeta{}); err != nil {
		t.Fatalf("changed SyncFromChannels returned error: %v", err)
	}
	if got := llmcache.GlobalGeneration(ctx); got == initialGeneration {
		t.Fatalf("global generation after provider mapping change = %q, want change from %q", got, initialGeneration)
	}
}

func openOfficialSnapshotDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE llm_official_model_snapshots (
		source_key text PRIMARY KEY,
		effective_models text NOT NULL DEFAULT '[]',
		effective_provider_models text NOT NULL DEFAULT '[]',
		latest_models text NOT NULL DEFAULT '[]',
		previous_models text NOT NULL DEFAULT '[]',
		latest_event_version integer NOT NULL DEFAULT 0,
		latest_synced_at datetime,
		effective_updated_at datetime,
		last_check_status text NOT NULL DEFAULT 'accepted',
		last_reject_reason text,
		created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`).Error; err != nil {
		t.Fatalf("create snapshot table: %v", err)
	}
	return db
}

func assertEffectiveProviderModels(t *testing.T, db *gorm.DB, want []providerModelJSON) {
	t.Helper()
	var raw string
	if err := db.Raw(
		"SELECT effective_provider_models FROM llm_official_model_snapshots WHERE source_key = ?",
		SourceKeyZGICloud,
	).Scan(&raw).Error; err != nil {
		t.Fatalf("load effective provider-model pairs: %v", err)
	}

	var got []providerModelJSON
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode effective provider-model pairs %q: %v", raw, err)
	}
	if len(got) != len(want) {
		t.Fatalf("effective provider-model pairs = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("effective provider-model pair %d = %#v, want %#v; all=%#v", i, got[i], want[i], got)
		}
	}
}
