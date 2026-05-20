package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	pconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	"gorm.io/gorm"
)

func TestNewLLMModule_StartsOfficialModelSynchronizerInCloudMode(t *testing.T) {
	setWireTestConfig(t, "CLOUD", false)

	db := &gorm.DB{}

	type call struct {
		edition string
		addr    string
	}

	var got *call
	var catalogStarted bool
	var localStarted bool
	previous := ensureOfficialModelSyncStarted
	previousCatalog := ensureCatalogSyncStarted
	previousLocal := ensureLocalModelMetaSyncStarted
	ensureOfficialModelSyncStarted = func(_ context.Context, _ *gorm.DB, edition string, grpcAddr string) {
		got = &call{
			edition: edition,
			addr:    grpcAddr,
		}
	}
	ensureCatalogSyncStarted = func(_ context.Context, _ *gorm.DB, edition string, grpcAddr string) {
		catalogStarted = edition == "CLOUD" && grpcAddr == "grpc.console:50051"
	}
	ensureLocalModelMetaSyncStarted = func(_ context.Context, _ *gorm.DB, edition string) {
		localStarted = true
	}
	t.Cleanup(func() {
		ensureOfficialModelSyncStarted = previous
		ensureCatalogSyncStarted = previousCatalog
		ensureLocalModelMetaSyncStarted = previousLocal
	})

	NewLLMModule(db, nil, nil, nil, nil, pconsole.NewStandalone())

	require.NotNil(t, got)
	require.Equal(t, "CLOUD", got.edition)
	require.Equal(t, "grpc.console:50051", got.addr)
	require.True(t, catalogStarted)
	require.False(t, localStarted)
}

func TestNewLLMModule_StrictSyncModeDoesNotAffectSynchronizerStartup(t *testing.T) {
	setWireTestConfig(t, "CLOUD", true)

	db := &gorm.DB{}

	var started bool
	var catalogStarted bool
	var localStarted bool
	previous := ensureOfficialModelSyncStarted
	previousCatalog := ensureCatalogSyncStarted
	previousLocal := ensureLocalModelMetaSyncStarted
	ensureOfficialModelSyncStarted = func(_ context.Context, _ *gorm.DB, edition string, grpcAddr string) {
		started = edition == "CLOUD" && grpcAddr == "grpc.console:50051"
	}
	ensureCatalogSyncStarted = func(_ context.Context, _ *gorm.DB, edition string, grpcAddr string) {
		catalogStarted = edition == "CLOUD" && grpcAddr == "grpc.console:50051"
	}
	ensureLocalModelMetaSyncStarted = func(_ context.Context, _ *gorm.DB, edition string) {
		localStarted = true
	}
	t.Cleanup(func() {
		ensureOfficialModelSyncStarted = previous
		ensureCatalogSyncStarted = previousCatalog
		ensureLocalModelMetaSyncStarted = previousLocal
	})

	NewLLMModule(db, nil, nil, nil, nil, pconsole.NewStandalone())

	require.True(t, started)
	require.True(t, catalogStarted)
	require.False(t, localStarted)
}

func TestNewLLMModule_StartsLocalModelMetaSynchronizerOutsideCloudMode(t *testing.T) {
	setWireTestConfig(t, "LOCAL", false)

	db := &gorm.DB{}

	var localEdition string
	var officialStarted bool
	var catalogStarted bool

	previousOfficial := ensureOfficialModelSyncStarted
	previousCatalog := ensureCatalogSyncStarted
	previousLocal := ensureLocalModelMetaSyncStarted
	ensureOfficialModelSyncStarted = func(_ context.Context, _ *gorm.DB, edition string, grpcAddr string) {
		officialStarted = true
	}
	ensureCatalogSyncStarted = func(_ context.Context, _ *gorm.DB, edition string, grpcAddr string) {
		catalogStarted = true
	}
	ensureLocalModelMetaSyncStarted = func(_ context.Context, _ *gorm.DB, edition string) {
		localEdition = edition
	}
	t.Cleanup(func() {
		ensureOfficialModelSyncStarted = previousOfficial
		ensureCatalogSyncStarted = previousCatalog
		ensureLocalModelMetaSyncStarted = previousLocal
	})

	NewLLMModule(db, nil, nil, nil, nil, pconsole.NewStandalone())

	require.Equal(t, "LOCAL", localEdition)
	require.False(t, officialStarted)
	require.False(t, catalogStarted)
}

func setWireTestConfig(t *testing.T, edition string, strictSync bool) {
	t.Helper()
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Platform: config.PlatformConfig{Edition: edition},
		Console:  config.ConsoleConfig{GRPCAddr: "grpc.console:50051"},
		LLM:      config.LLMConfig{OfficialModelStrictSync: strictSync},
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})
}
