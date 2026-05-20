package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	channeldto "github.com/zgiai/ginext/internal/modules/llm/channel/dto"
	channelrepo "github.com/zgiai/ginext/internal/modules/llm/channel/repository"
	"gorm.io/gorm"
)

func seedPrivateRouteForAggregatedList(
	t *testing.T,
	db *gorm.DB,
	routeID uuid.UUID,
	orgID uuid.UUID,
	name string,
	balance int64,
) {
	t.Helper()

	if err := db.Exec(
		`INSERT INTO llm_routes(
			id,
			organization_id,
			type,
			user_credential_id,
			name,
			models,
			api_base_url,
			provider,
			model_maps,
			param_override,
			header_override,
			validation_report,
			tags,
			description,
			priority,
			weight,
			is_enabled,
			is_official,
			auto_ban,
			sync_mode,
			balance,
			currency,
			created_at,
			updated_at,
			deleted_at
		) VALUES (?, ?, 'PRIVATE', NULL, ?, '["gpt-4o-mini"]', 'https://api.openai.com/v1', 'openai', '{}', '{}', '{}', '{}', '[]', '', 10, 1, TRUE, FALSE, FALSE, 'snapshot', ?, 'USD', ?, ?, NULL)`,
		routeID.String(),
		orgID.String(),
		name,
		balance,
		time.Now(),
		time.Now(),
	).Error; err != nil {
		t.Fatalf("failed to seed private route: %v", err)
	}
}

func TestListRoutesAggregated_UsesChannelWalletBalanceOnly(t *testing.T) {
	db := openCreateRouteInitialFundsTestDB(t)
	svc := &channelService{
		tenantRouteRepo: channelrepo.NewTenantRouteRepository(db),
		db:              db,
	}

	orgID := uuid.New()
	walletBackedRouteID := uuid.New()
	snapshotFallbackRouteID := uuid.New()

	seedPrivateRouteForAggregatedList(t, db, walletBackedRouteID, orgID, "wallet-backed", 100)
	seedPrivateRouteForAggregatedList(t, db, snapshotFallbackRouteID, orgID, "snapshot-only", 80)
	seedChannelWallet(t, db, walletBackedRouteID, orgID, 150, "ACTIVE")

	resp, err := svc.ListRoutesAggregated(context.Background(), orgID, &channeldto.ListRoutesAggregatedRequest{
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListRoutesAggregated returned error: %v", err)
	}

	if resp.Total != 2 {
		t.Fatalf("total = %d, want 2", resp.Total)
	}

	fundsByName := make(map[string]int64, len(resp.Channels))
	for _, channel := range resp.Channels {
		fundsByName[channel.Name] = channel.RemainingFunds
	}

	if fundsByName["wallet-backed"] != 150 {
		t.Fatalf("wallet-backed remaining_funds = %d, want 150", fundsByName["wallet-backed"])
	}
	if fundsByName["snapshot-only"] != 0 {
		t.Fatalf("snapshot-only remaining_funds = %d, want 0", fundsByName["snapshot-only"])
	}
}
