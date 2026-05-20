package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	consoleintf "github.com/zgiai/ginext/internal/infra/platform/console"
	channelmodel "github.com/zgiai/ginext/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/ginext/internal/modules/llm/channel/repository"
	officialmodel "github.com/zgiai/ginext/internal/modules/llm/officialmodel"
	"github.com/zgiai/ginext/internal/modules/llm/shared"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeConsoleProvider struct {
	listCalls int
	baseURL   string
	channels  []*consoleintf.PlatformChannelInfo
}

type unavailableConsoleProvider struct {
	fakeConsoleProvider
}

func (f *fakeConsoleProvider) IsAvailable() bool { return true }
func (f *fakeConsoleProvider) GetMode() string   { return "CLOUD" }
func (f *fakeConsoleProvider) GetBaseURL() string {
	if f.baseURL == "" {
		return "https://console.example.com"
	}
	return f.baseURL
}
func (f *fakeConsoleProvider) RegisterOrganization(context.Context, *consoleintf.RegisterOrganizationRequest) error {
	return nil
}
func (f *fakeConsoleProvider) CheckQuota(context.Context, *consoleintf.CheckQuotaRequest) (*consoleintf.CheckQuotaResponse, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) ReportUsage(context.Context, *consoleintf.ReportUsageRequest) error {
	return nil
}
func (f *fakeConsoleProvider) ListPlatformChannelModels(context.Context) (*consoleintf.PlatformChannelModelsResponse, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) ListPlatformChannels(context.Context) (*consoleintf.PlatformChannelsResponse, error) {
	f.listCalls++
	return &consoleintf.PlatformChannelsResponse{Channels: f.channels}, nil
}
func (f *fakeConsoleProvider) UpdatePlatformChannel(context.Context, string, *consoleintf.UpdatePlatformChannelRequest) error {
	return nil
}
func (f *fakeConsoleProvider) ListCreditProducts(context.Context) ([]*consoleintf.CreditProductInfo, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) PaymentCheckout(context.Context, *consoleintf.PaymentCheckoutRequest) (*consoleintf.PaymentCheckoutResponse, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) GetPaymentWallet(context.Context, string, string) (*consoleintf.PaymentWalletResponse, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) GetPaymentOrder(context.Context, string, string) (*consoleintf.PaymentOrderResponse, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) ListPaymentOrders(context.Context, *consoleintf.ListPaymentOrdersRequest) (*consoleintf.ListPaymentOrdersResponse, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) ListPaymentPurchaseRecords(context.Context, *consoleintf.ListPaymentPurchaseRecordsRequest) (*consoleintf.ListPaymentPurchaseRecordsResponse, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) ExportPaymentPurchaseRecords(context.Context, *consoleintf.ListPaymentPurchaseRecordsRequest) (*consoleintf.PaymentPurchaseRecordsExportFile, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) CancelPaymentOrder(context.Context, *consoleintf.CancelPaymentOrderRequest) error {
	return nil
}
func (f *fakeConsoleProvider) SubmitBankTransfer(context.Context, *consoleintf.SubmitBankTransferRequest) (*consoleintf.BankTransferRequestResponse, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) ListBankTransfers(context.Context, *consoleintf.ListBankTransfersRequest) (*consoleintf.ListBankTransfersResponse, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) GetBankTransfer(context.Context, *consoleintf.GetBankTransferRequest) (*consoleintf.BankTransferRequestResponse, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) CancelBankTransfer(context.Context, *consoleintf.CancelBankTransferRequest) error {
	return nil
}
func (f *fakeConsoleProvider) ReviewBankTransfer(context.Context, *consoleintf.ReviewBankTransferRequest) error {
	return nil
}
func (f *fakeConsoleProvider) UploadBankTransferVoucher(context.Context, *consoleintf.UploadBankTransferVoucherRequest) (*consoleintf.UploadBankTransferVoucherResponse, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) GetBankTransferVoucher(context.Context, string) (*consoleintf.BankTransferVoucherFile, error) {
	return nil, nil
}
func (f *fakeConsoleProvider) NotifyOfficialSignup(context.Context, *consoleintf.NotifyOfficialSignupRequest) (*consoleintf.NotifyOfficialSignupResponse, error) {
	return &consoleintf.NotifyOfficialSignupResponse{}, nil
}

func (f *unavailableConsoleProvider) IsAvailable() bool { return false }

func setupOfficialChannelTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:official_channel_snapshot_%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil && strings.Contains(err.Error(), "requires cgo") {
		t.Skip("sqlite driver requires cgo in this environment")
	}
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&officialmodel.Snapshot{}))
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_routes (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			type TEXT NOT NULL,
			user_credential_id TEXT NULL,
			name TEXT NOT NULL DEFAULT '',
			models JSON NULL,
			native_protocols JSON NULL,
			api_base_url TEXT NULL,
			provider TEXT NULL,
			model_maps JSON NULL,
			param_override JSON NULL,
			header_override JSON NULL,
			validation_report JSON NULL,
			tags JSON NULL,
			description TEXT NULL,
			priority INTEGER NOT NULL DEFAULT 0,
			weight INTEGER NOT NULL DEFAULT 1,
			is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			is_official BOOLEAN NOT NULL DEFAULT FALSE,
			auto_ban BOOLEAN NOT NULL DEFAULT FALSE,
			sync_mode TEXT NOT NULL DEFAULT 'snapshot',
			last_synced_at DATETIME NULL,
			balance DECIMAL(15,4) NOT NULL DEFAULT 0,
			currency TEXT NOT NULL DEFAULT 'USD',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at DATETIME NULL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_credentials (
			id TEXT PRIMARY KEY,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			deleted_at DATETIME NULL
		)
	`).Error)

	return db
}

func TestInitOfficialChannelCreatesRouteWithoutPersistingModels(t *testing.T) {
	db := setupOfficialChannelTestDB(t)
	repo := channelrepo.NewTenantRouteRepository(db)
	consoleProvider := &fakeConsoleProvider{
		channels: []*consoleintf.PlatformChannelInfo{
			{ID: "ch-1", Models: []string{"gpt-4o", "gpt-4.1"}},
		},
	}
	svc := &channelService{
		tenantRouteRepo: repo,
		db:              db,
		consoleProvider: consoleProvider,
	}

	_, err := officialmodel.SyncFromChannels(context.Background(), db, []officialmodel.UpstreamChannel{
		{ID: "ch-1", Models: []string{"gpt-4o", "gpt-4.1"}},
	}, officialmodel.SyncMeta{})
	require.NoError(t, err)

	orgID := uuid.New()
	require.NoError(t, svc.InitOfficialChannel(context.Background(), orgID))

	var route channelmodel.LLMRoute
	require.NoError(t, db.Where("organization_id = ? AND is_official = ?", orgID, true).First(&route).Error)
	require.Equal(t, shared.RouteTypeZGICloud, route.Type)
	require.Empty(t, route.Models)
	require.Zero(t, consoleProvider.listCalls)
}

func TestInitOfficialChannelCreatesRouteWithoutSnapshot(t *testing.T) {
	db := setupOfficialChannelTestDB(t)
	repo := channelrepo.NewTenantRouteRepository(db)
	svc := &channelService{
		tenantRouteRepo: repo,
		db:              db,
		consoleProvider: &fakeConsoleProvider{},
	}

	orgID := uuid.New()
	require.NoError(t, svc.InitOfficialChannel(context.Background(), orgID))

	var route channelmodel.LLMRoute
	require.NoError(t, db.Where("organization_id = ? AND is_official = ?", orgID, true).First(&route).Error)
	require.Equal(t, shared.RouteTypeZGICloud, route.Type)
	require.Empty(t, route.Models)
}

func TestInitOfficialChannelNoOpWhenConsoleUnavailable(t *testing.T) {
	db := setupOfficialChannelTestDB(t)
	repo := channelrepo.NewTenantRouteRepository(db)
	svc := &channelService{
		tenantRouteRepo: repo,
		db:              db,
		consoleProvider: &unavailableConsoleProvider{},
	}

	require.NoError(t, svc.InitOfficialChannel(context.Background(), uuid.New()))

	var count int64
	require.NoError(t, db.Table("llm_routes").Count(&count).Error)
	require.Zero(t, count)
}

func TestOfficialRouteHydratesModelsAfterSnapshotArrives(t *testing.T) {
	db := setupOfficialChannelTestDB(t)
	repo := channelrepo.NewTenantRouteRepository(db)
	svc := &channelService{
		tenantRouteRepo: repo,
		db:              db,
		consoleProvider: &fakeConsoleProvider{},
	}

	orgID := uuid.New()
	require.NoError(t, svc.InitOfficialChannel(context.Background(), orgID))

	_, err := officialmodel.SyncFromChannels(context.Background(), db, []officialmodel.UpstreamChannel{
		{ID: "ch-1", Models: []string{"gpt-4o", "gpt-4.1"}},
	}, officialmodel.SyncMeta{})
	require.NoError(t, err)

	routes, err := repo.GetEnabledRoutes(context.Background(), orgID)
	require.NoError(t, err)
	require.Len(t, routes, 1)
	require.ElementsMatch(t, []string{"gpt-4o", "gpt-4.1"}, routes[0].GetEffectiveModels())
}

func TestGetPlatformChannelReadsModelCountFromSnapshot(t *testing.T) {
	db := setupOfficialChannelTestDB(t)
	repo := channelrepo.NewTenantRouteRepository(db)
	svc := &channelService{
		tenantRouteRepo: repo,
		db:              db,
		consoleProvider: &fakeConsoleProvider{},
	}

	orgID := uuid.New()
	require.NoError(t, db.Create(&channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  orgID,
		Type:            shared.RouteTypeZGICloud,
		Name:            "ZGI Cloud",
		ChannelProvider: "zgi-cloud",
		APIBaseURL:      "https://console.example.com/v1/internal",
		IsEnabled:       true,
		IsOfficial:      true,
		Priority:        200,
		Weight:          100,
	}).Error)

	_, err := officialmodel.SyncFromChannels(context.Background(), db, []officialmodel.UpstreamChannel{
		{ID: "ch-1", Models: []string{"gpt-4o", "gpt-4.1", "o1"}},
	}, officialmodel.SyncMeta{})
	require.NoError(t, err)

	view, err := svc.GetPlatformChannel(context.Background(), orgID)
	require.NoError(t, err)
	require.NotNil(t, view)
	require.Equal(t, 3, view.ModelCount)
}
