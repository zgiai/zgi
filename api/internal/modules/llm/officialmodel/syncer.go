package officialmodel

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/logger"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

const defaultReconnectDelay = 2 * time.Second

type channelEventStream interface {
	Recv() (*pb.ChannelEvent, error)
}

type channelRPCClient interface {
	ListOfficialChannels(ctx context.Context, req *pb.ListOfficialChannelsRequest) (*pb.ListOfficialChannelsResponse, error)
	WatchChannels(ctx context.Context, req *pb.WatchChannelsRequest) (channelEventStream, error)
}

type grpcChannelClient struct {
	client pb.ChannelServiceClient
}

func (c *grpcChannelClient) ListOfficialChannels(ctx context.Context, req *pb.ListOfficialChannelsRequest) (*pb.ListOfficialChannelsResponse, error) {
	return c.client.ListOfficialChannels(ctx, req)
}

func (c *grpcChannelClient) WatchChannels(ctx context.Context, req *pb.WatchChannelsRequest) (channelEventStream, error) {
	return c.client.WatchChannels(ctx, req)
}

type Synchronizer struct {
	db             *gorm.DB
	grpcAddr       string
	newClient      func(addr string) (channelRPCClient, io.Closer, error)
	reconnectDelay time.Duration
	now            func() time.Time
}

func NewSynchronizer(db *gorm.DB, grpcAddr string) *Synchronizer {
	return &Synchronizer{
		db:             db,
		grpcAddr:       strings.TrimSpace(grpcAddr),
		newClient:      newGRPCChannelClient,
		reconnectDelay: defaultReconnectDelay,
		now:            time.Now,
	}
}

func newSynchronizerWithClient(db *gorm.DB, client channelRPCClient) *Synchronizer {
	return &Synchronizer{
		db:       db,
		grpcAddr: "in-memory",
		newClient: func(string) (channelRPCClient, io.Closer, error) {
			return client, noopCloser{}, nil
		},
		reconnectDelay: 10 * time.Millisecond,
		now:            time.Now,
	}
}

func (s *Synchronizer) Start(ctx context.Context) error {
	if s == nil || s.db == nil || s.grpcAddr == "" {
		return nil
	}

	client, closer, stream, err := s.connect(ctx)
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return err
	}

	go s.run(ctx, client, closer, stream)
	return nil
}

func (s *Synchronizer) run(ctx context.Context, client channelRPCClient, closer io.Closer, stream channelEventStream) {
	currentClient := client
	currentCloser := closer
	currentStream := stream

	for {
		err := s.consumeStream(ctx, currentClient, currentStream)
		if currentCloser != nil {
			_ = currentCloser.Close()
		}
		if ctx.Err() != nil {
			return
		}

		logger.WarnContext(ctx, "Official model watch stream ended, reconnecting", zap.Error(err))

		for {
			if !sleepContext(ctx, s.reconnectDelay) {
				return
			}

			currentClient, currentCloser, currentStream, err = s.connect(ctx)
			if err == nil {
				break
			}
			if currentCloser != nil {
				_ = currentCloser.Close()
			}
			logger.WarnContext(ctx, "Official model synchronizer reconnect failed",
				zap.String("grpc_addr", s.grpcAddr),
				zap.Error(err),
			)
			if ctx.Err() != nil {
				return
			}
		}
	}
}

func (s *Synchronizer) connect(ctx context.Context) (channelRPCClient, io.Closer, channelEventStream, error) {
	client, closer, err := s.newClient(s.grpcAddr)
	if err != nil {
		return nil, closer, nil, err
	}

	stream, err := client.WatchChannels(ctx, &pb.WatchChannelsRequest{})
	if err != nil {
		return nil, closer, nil, err
	}

	// Use a full snapshot as the only write-source of truth.
	// The watch stream is treated as an invalidation signal, not a delta payload.
	if err := s.fullRefresh(ctx, client, SyncMeta{SyncedAt: s.now().UTC()}); err != nil {
		return nil, closer, nil, err
	}

	return client, closer, stream, nil
}

func (s *Synchronizer) consumeStream(ctx context.Context, client channelRPCClient, stream channelEventStream) error {
	for {
		event, err := stream.Recv()
		if err != nil {
			return err
		}
		// Console sends SNAPSHOT frames on stream establishment. We consume them to keep
		// the stream healthy, but do not write from them because connect() already rebuilt
		// local state from ListOfficialChannels immediately before entering the watch loop.
		if event == nil || event.GetType() == pb.ChannelEvent_SNAPSHOT {
			continue
		}

		meta := SyncMeta{
			Version: event.GetVersion(),
		}
		if ts := event.GetTimestamp(); ts > 0 {
			meta.SyncedAt = time.UnixMilli(ts).UTC()
		}

		// Any non-SNAPSHOT event is only a "data changed" signal. Because the official
		// model set is small, we rebuild from a fresh full snapshot instead of applying
		// incremental patches locally.
		if err := s.fullRefresh(ctx, client, meta); err != nil {
			return err
		}
	}
}

func (s *Synchronizer) fullRefresh(ctx context.Context, client channelRPCClient, meta SyncMeta) error {
	resp, err := client.ListOfficialChannels(ctx, &pb.ListOfficialChannelsRequest{})
	if err != nil {
		return err
	}

	channels := make([]UpstreamChannel, 0, len(resp.GetChannels()))
	for _, ch := range resp.GetChannels() {
		if ch == nil {
			continue
		}
		channels = append(channels, UpstreamChannel{
			ID:     ch.GetId(),
			Models: ch.GetModels(),
		})
	}

	if meta.SyncedAt.IsZero() {
		meta.SyncedAt = s.now().UTC()
	}

	_, err = SyncFromChannels(ctx, s.db, channels, meta)
	return err
}

func newGRPCChannelClient(addr string) (channelRPCClient, io.Closer, error) {
	dialOptions := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	dialOptions = append(dialOptions, observability.GRPCDialOptions()...)
	conn, err := grpc.Dial(addr, dialOptions...)
	if err != nil {
		return nil, nil, err
	}
	return &grpcChannelClient{client: pb.NewChannelServiceClient(conn)}, conn, nil
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
