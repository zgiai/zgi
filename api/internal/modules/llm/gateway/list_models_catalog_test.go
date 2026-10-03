package gateway

import (
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

// TestListAvailableModelsListsEnabledRoutableModelsOnly pins the /v1/models
// contract that the G2 retest questioned: the directory is the organization's
// enabled models that a route can serve over at least one public protocol.
// A routable model the organization disabled (or never enabled) stays out,
// matching the tenant model view's Callable flag, and a model without a channel
// is skipped without failing the directory.
func TestListAvailableModelsListsEnabledRoutableModelsOnly(t *testing.T) {
	setGatewayConsoleAPIURL(t, "https://console-api.zgi.im")
	db := openGatewayCatalogDB(t)
	require.NoError(t, db.Exec(`CREATE TABLE organizations (id TEXT PRIMARY KEY)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE llm_model_configs (
		id TEXT PRIMARY KEY,
		organization_id TEXT NOT NULL,
		model_id TEXT NOT NULL,
		is_enabled BOOLEAN,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME
	)`).Error)

	organizationID := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO organizations (id) VALUES (?)`, organizationID.String()).Error)

	const (
		enabledModel      = "enabled-routable"
		disabledModel     = "disabled-routable"
		unconfiguredModel = "unconfigured-routable"
		channellessModel  = "enabled-channelless"
		protocollessModel = "enabled-no-protocol"
	)
	enabled := map[string]*bool{
		enabledModel:      boolPtr(true),
		disabledModel:     boolPtr(false),
		unconfiguredModel: nil,
		channellessModel:  boolPtr(true),
		protocollessModel: boolPtr(true),
	}
	for name, isEnabled := range enabled {
		modelID := uuid.New()
		insertGatewayCatalogModel(t, db, modelID, "supplier-"+name, name)
		if isEnabled != nil {
			require.NoError(t, db.Exec(
				`INSERT INTO llm_model_configs (id, organization_id, model_id, is_enabled) VALUES (?, ?, ?, ?)`,
				uuid.NewString(), organizationID.String(), modelID.String(), *isEnabled,
			).Error)
		}
	}

	routed := []string{enabledModel, disabledModel, unconfiguredModel, protocollessModel}
	providerModels := make([]channelmodel.ProviderModel, 0, len(routed))
	for _, name := range routed {
		providerModels = append(providerModels, channelmodel.ProviderModel{Provider: "supplier-" + name, Model: name})
	}
	route := &channelmodel.LLMRoute{
		ID:                     uuid.New(),
		OrganizationID:         organizationID,
		Type:                   shared.RouteTypeZGICloud,
		ChannelProvider:        "zgi-cloud",
		Models:                 routed,
		OfficialProviderModels: providerModels,
		OfficialModelProtocols: map[string][]string{
			enabledModel:      {protocolOpenAIChatCompletions},
			disabledModel:     {protocolOpenAIChatCompletions},
			unconfiguredModel: {protocolOpenAIChatCompletions},
			protocollessModel: {},
		},
		IsEnabled:  true,
		IsOfficial: true,
	}
	service := &llmGatewayServiceImpl{
		db: db,
		channelRouter: &ChannelRouter{
			db:                      db,
			organizationIDRouteRepo: &fakeCandidateRouteRepo{routes: []*channelmodel.LLMRoute{route}},
			strategyFactory:         NewStrategyFactory(),
		},
	}

	models, err := service.ListAvailableModels(t.Context(), &apikeymodel.TenantAPIKey{OrganizationID: organizationID.String()})
	require.NoError(t, err)

	listed := make([]string, 0, len(models))
	for _, model := range models {
		listed = append(listed, model.ID)
	}
	sort.Strings(listed)
	require.Equal(t, []string{enabledModel}, listed)
}
