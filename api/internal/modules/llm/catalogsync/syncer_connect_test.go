package catalogsync

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type connectTestClient struct {
	watchCalled bool
}

func (c *connectTestClient) GetPublishedCatalog(context.Context, *pb.GetPublishedCatalogRequest) (*pb.GetPublishedCatalogResponse, error) {
	inputPrice := 14.0
	outputPrice := 70.0
	return &pb.GetPublishedCatalogResponse{
		Version:     17,
		PublishedAt: 1760000000000,
		Providers: []*pb.CatalogProvider{{
			Provider: "doubao",
		}},
		Models: []*pb.CatalogModel{{
			Provider:    "doubao",
			Model:       "doubao-seedance-2-0-260128",
			Currency:    "CNY",
			InputPrice:  &inputPrice,
			OutputPrice: &outputPrice,
			PricingJson: `{"token_tiers":[{"min_input_tokens":0,"input_price_per_million":14,"output_price_per_million":70}]}`,
		}},
	}, nil
}

func (c *connectTestClient) WatchCatalogPublishes(context.Context, *pb.WatchCatalogPublishesRequest) (catalogPublishStream, error) {
	c.watchCalled = true
	return connectTestStream{}, nil
}

type connectTestStream struct{}

func (connectTestStream) Recv() (*pb.CatalogPublishEvent, error) { return nil, io.EOF }

func TestConnectContinuesWatchingWhenCatalogReapplyFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SyncState{}))
	require.NoError(t, db.Create(&SyncState{
		SyncKey:            defaultSyncKey,
		LastAppliedVersion: 17,
	}).Error)

	client := &connectTestClient{}
	syncer := &Synchronizer{
		db: db,
		newClient: func(string) (catalogRPCClient, io.Closer, error) {
			return client, noopCloser{}, nil
		},
	}

	_, closer, stream, err := syncer.connect(context.Background())
	require.NoError(t, err)
	require.NotNil(t, stream)
	require.NoError(t, closer.Close())
	require.True(t, client.watchCalled, "watch subscription must continue after reapply failure")
}
