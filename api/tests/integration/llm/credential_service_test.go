package llm_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/service"
	testutil "github.com/zgiai/zgi/api/tests/llm/shared"
)

func TestTenantCredentialService_Create(t *testing.T) {
	repo := testutil.NewMockTenantCredentialRepository()
	crypto := testutil.NewMockCryptoService()
	svc := service.NewTenantCredentialService(repo, crypto)
	ctx := context.Background()
	tenantID := uuid.New()

	t.Run("success", func(t *testing.T) {
		result, err := svc.Create(ctx, tenantID, &dto.CreateTenantCredentialRequest{
			Name:            "My Key",
			ChannelProvider: "openai",
			APIKey:          "sk-user-key",
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, tenantID, result.OrganizationID)
	})
}

func TestTenantCredentialService_TenantIsolation(t *testing.T) {
	repo := testutil.NewMockTenantCredentialRepository()
	crypto := testutil.NewMockCryptoService()
	svc := service.NewTenantCredentialService(repo, crypto)
	ctx := context.Background()

	tenant1 := uuid.New()
	tenant2 := uuid.New()

	// Create for tenant1
	for i := 0; i < 3; i++ {
		svc.Create(ctx, tenant1, &dto.CreateTenantCredentialRequest{
			Name:            "T1 Key " + string(rune('A'+i)),
			ChannelProvider: "openai",
			APIKey:          "sk-t1-" + string(rune('A'+i)),
		})
	}

	// Create for tenant2
	for i := 0; i < 2; i++ {
		svc.Create(ctx, tenant2, &dto.CreateTenantCredentialRequest{
			Name:            "T2 Key " + string(rune('A'+i)),
			ChannelProvider: "openai",
			APIKey:          "sk-t2-" + string(rune('A'+i)),
		})
	}

	// Verify isolation
	t.Run("tenant1 sees only own", func(t *testing.T) {
		results, total, _ := svc.List(ctx, tenant1, &dto.ListCredentialRequest{Page: 1, PageSize: 10})
		assert.Equal(t, int64(3), total)
		assert.Len(t, results, 3)
	})

	t.Run("tenant2 sees only own", func(t *testing.T) {
		results, total, _ := svc.List(ctx, tenant2, &dto.ListCredentialRequest{Page: 1, PageSize: 10})
		assert.Equal(t, int64(2), total)
		assert.Len(t, results, 2)
	})
}
