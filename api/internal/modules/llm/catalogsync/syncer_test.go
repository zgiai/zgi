package catalogsync

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	pb "github.com/zgiai/ginext/pkg/rpc/v1"
	"gorm.io/gorm"
)

type fakeCatalogRPCClient struct {
	mu        sync.Mutex
	stream    *fakeCatalogPublishStream
	watchReqs []*pb.WatchCatalogPublishesRequest
	getCalls  int
	snapshots map[int64]*pb.GetPublishedCatalogResponse
}

func (f *fakeCatalogRPCClient) WatchCatalogPublishes(ctx context.Context, req *pb.WatchCatalogPublishesRequest) (catalogPublishStream, error) {
	f.mu.Lock()
	f.watchReqs = append(f.watchReqs, req)
	f.mu.Unlock()
	return f.stream, nil
}

func (f *fakeCatalogRPCClient) GetPublishedCatalog(ctx context.Context, req *pb.GetPublishedCatalogRequest) (*pb.GetPublishedCatalogResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++

	resp, ok := f.snapshots[req.GetVersion()]
	if !ok {
		return nil, fmt.Errorf("snapshot version %d not found", req.GetVersion())
	}
	return resp, nil
}

func (f *fakeCatalogRPCClient) GetCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getCalls
}

func (f *fakeCatalogRPCClient) LastSeenVersion() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.watchReqs) == 0 {
		return -1
	}
	return f.watchReqs[len(f.watchReqs)-1].GetLastSeenVersion()
}

type fakeCatalogPublishStream struct {
	ctx    context.Context
	events chan *pb.CatalogPublishEvent
}

func (f *fakeCatalogPublishStream) Recv() (*pb.CatalogPublishEvent, error) {
	select {
	case <-f.ctx.Done():
		return nil, f.ctx.Err()
	case event, ok := <-f.events:
		if !ok {
			return nil, io.EOF
		}
		return event, nil
	}
}

func setupCatalogSyncerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:catalog_syncer_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SyncState{}))
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_providers (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			provider_name TEXT NOT NULL,
			logo_url TEXT,
			website TEXT,
			pricing_url TEXT,
			country_code TEXT,
			founded_year INTEGER DEFAULT 0,
			tagline TEXT,
			description TEXT,
			metadata TEXT DEFAULT '{}',
			is_active BOOLEAN DEFAULT true,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_catalogsync_provider ON llm_providers(provider)`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_models (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			name TEXT NOT NULL,
			display_name TEXT NOT NULL,
			family TEXT,
			family_name TEXT,
			type TEXT,
			status TEXT,
			tagline TEXT,
			is_flagship BOOLEAN DEFAULT false,
			is_recommended BOOLEAN DEFAULT false,
			is_featured BOOLEAN DEFAULT false,
			is_new BOOLEAN DEFAULT false,
			access_type TEXT,
			currency TEXT,
			is_active BOOLEAN DEFAULT true,
			context_window INTEGER,
			max_output_tokens INTEGER,
			input_price TEXT,
			output_price TEXT,
			cached_input_price TEXT,
			use_cases TEXT,
			input_modalities TEXT DEFAULT '[]',
			output_modalities TEXT DEFAULT '[]',
			supported_parameters BLOB DEFAULT '[]',
			config_parameters BLOB DEFAULT '[]',
			knowledge_cutoff TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_catalogsync_model ON llm_models(provider, name)`).Error)
	return db
}

func TestSynchronizerDoesNotPullBeforePublishSignal(t *testing.T) {
	db := setupCatalogSyncerTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := &fakeCatalogPublishStream{
		ctx:    ctx,
		events: make(chan *pb.CatalogPublishEvent, 1),
	}
	client := &fakeCatalogRPCClient{
		stream:    stream,
		snapshots: map[int64]*pb.GetPublishedCatalogResponse{},
	}

	syncer := newSynchronizerWithClient(db, client)
	require.NoError(t, syncer.Start(ctx))

	require.Never(t, func() bool {
		return client.GetCalls() > 0
	}, 300*time.Millisecond, 20*time.Millisecond)
}

func TestSynchronizerReconnectsAndAppliesReplayEvent(t *testing.T) {
	db := setupCatalogSyncerTestDB(t)
	require.NoError(t, db.Create(&SyncState{
		SyncKey:            defaultSyncKey,
		LastAppliedVersion: 5,
	}).Error)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream1 := &fakeCatalogPublishStream{
		ctx:    ctx,
		events: make(chan *pb.CatalogPublishEvent),
	}
	close(stream1.events)

	stream2 := &fakeCatalogPublishStream{
		ctx:    ctx,
		events: make(chan *pb.CatalogPublishEvent, 1),
	}
	secondClient := &fakeCatalogRPCClient{
		stream: stream2,
		snapshots: map[int64]*pb.GetPublishedCatalogResponse{
			8: {
				Version:     8,
				PublishedAt: time.Now().UnixMilli(),
			},
		},
	}

	clients := []catalogRPCClient{
		&fakeCatalogRPCClient{stream: stream1, snapshots: map[int64]*pb.GetPublishedCatalogResponse{}},
		secondClient,
	}

	var mu sync.Mutex
	idx := 0
	syncer := NewSynchronizer(db, "in-memory")
	syncer.newClient = func(string) (catalogRPCClient, io.Closer, error) {
		mu.Lock()
		defer mu.Unlock()
		client := clients[idx]
		if idx < len(clients)-1 {
			idx++
		}
		return client, noopCloser{}, nil
	}
	syncer.reconnectDelay = 10 * time.Millisecond

	require.NoError(t, syncer.Start(ctx))

	stream2.events <- &pb.CatalogPublishEvent{
		Version:     8,
		PublishedAt: time.Now().UnixMilli(),
	}

	require.Eventually(t, func() bool {
		var state SyncState
		if err := db.First(&state, "sync_key = ?", defaultSyncKey).Error; err != nil {
			return false
		}
		return state.LastAppliedVersion == 8
	}, 3*time.Second, 20*time.Millisecond)

	require.Equal(t, int64(5), secondClient.LastSeenVersion())
	require.Equal(t, 1, secondClient.GetCalls())
}

func TestSynchronizerInvalidConfigParametersJSONFallsBackToEmptyArray(t *testing.T) {
	db := setupCatalogSyncerTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := &fakeCatalogPublishStream{
		ctx:    ctx,
		events: make(chan *pb.CatalogPublishEvent, 1),
	}
	client := &fakeCatalogRPCClient{
		stream: stream,
		snapshots: map[int64]*pb.GetPublishedCatalogResponse{
			9: {
				Version:     9,
				PublishedAt: time.Now().UnixMilli(),
				Providers: []*pb.CatalogProvider{
					{
						Provider:     "openai",
						ProviderName: "OpenAI",
					},
				},
				Models: []*pb.CatalogModel{
					{
						Provider:             "openai",
						Model:                "gpt-4.1",
						ModelName:            "GPT-4.1",
						Type:                 "llm",
						Status:               "active",
						IsActive:             true,
						ConfigParametersJson: `{"bad":"shape"}`,
					},
				},
			},
		},
	}

	syncer := newSynchronizerWithClient(db, client)
	require.NoError(t, syncer.Start(ctx))

	stream.events <- &pb.CatalogPublishEvent{
		Version:     9,
		PublishedAt: time.Now().UnixMilli(),
	}

	require.Eventually(t, func() bool {
		var raw string
		err := db.Raw(`SELECT config_parameters FROM llm_models WHERE provider = ? AND name = ?`, "openai", "gpt-4.1").Scan(&raw).Error
		return err == nil && raw == "[]"
	}, 3*time.Second, 20*time.Millisecond)
}
