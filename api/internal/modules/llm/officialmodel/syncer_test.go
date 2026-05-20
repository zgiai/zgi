package officialmodel

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pb "github.com/zgiai/ginext/pkg/rpc/v1"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeChannelRPCClient struct {
	mu            sync.Mutex
	listResponses [][]*pb.OfficialChannel
	listCalls     int
	stream        *fakeChannelEventStream
}

func (f *fakeChannelRPCClient) ListOfficialChannels(context.Context, *pb.ListOfficialChannelsRequest) (*pb.ListOfficialChannelsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.listCalls
	if idx >= len(f.listResponses) {
		idx = len(f.listResponses) - 1
	}
	f.listCalls++
	return &pb.ListOfficialChannelsResponse{Channels: f.listResponses[idx]}, nil
}

func (f *fakeChannelRPCClient) WatchChannels(context.Context, *pb.WatchChannelsRequest) (channelEventStream, error) {
	return f.stream, nil
}

func (f *fakeChannelRPCClient) ListCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls
}

type fakeChannelEventStream struct {
	ctx    context.Context
	events chan *pb.ChannelEvent
}

func (f *fakeChannelEventStream) Recv() (*pb.ChannelEvent, error) {
	select {
	case <-f.ctx.Done():
		return nil, f.ctx.Err()
	case event := <-f.events:
		return event, nil
	}
}

func setupSyncerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:official_syncer_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Snapshot{}))
	return db
}

func TestSynchronizerRefreshesByFullSnapshotOnNonSnapshotEvents(t *testing.T) {
	db := setupSyncerTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := &fakeChannelEventStream{
		ctx:    ctx,
		events: make(chan *pb.ChannelEvent, 4),
	}
	client := &fakeChannelRPCClient{
		listResponses: [][]*pb.OfficialChannel{
			{
				{Id: "ch-1", Models: []string{"gpt-4o", "gpt-4.1"}},
			},
			{
				{Id: "ch-1", Models: []string{"gpt-4o", "gpt-4.1", "o1"}},
			},
		},
		stream: stream,
	}

	syncer := newSynchronizerWithClient(db, client)
	require.NoError(t, syncer.Start(ctx))

	stream.events <- &pb.ChannelEvent{Type: pb.ChannelEvent_SNAPSHOT}
	stream.events <- &pb.ChannelEvent{Type: pb.ChannelEvent_UPDATED, Version: 9, Timestamp: time.Now().UnixMilli()}

	require.Eventually(t, func() bool {
		models, err := GetEffectiveModels(context.Background(), db)
		if err != nil {
			return false
		}
		return len(models) == 3 && ContainsModel(models, "o1")
	}, 3*time.Second, 20*time.Millisecond)

	models, err := GetEffectiveModels(context.Background(), db)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"gpt-4o", "gpt-4.1", "o1"}, models)
	require.Equal(t, 2, client.ListCallCount(), "snapshot event should not trigger an extra full reload")
}
