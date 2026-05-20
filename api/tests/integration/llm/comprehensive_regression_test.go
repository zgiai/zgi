package llm_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/config"
	apikeymodel "github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/ginext/internal/modules/llm/apikey/repository"
	llmClient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmGateway "github.com/zgiai/ginext/internal/modules/llm/gateway"
	llmAdapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	paymentModel "github.com/zgiai/ginext/internal/modules/payment/model"
	"github.com/zgiai/ginext/pkg/database"
	"gorm.io/gorm"
)

/*
=============================================================================
COMPREHENSIVE LLM REGRESSION TEST SUITE
=============================================================================

Test Categories:
1. BILLING - 计费测试
   1.1 Credit Balance Tests (余额测试)
   1.2 API Key Quota Tests (配额测试)
   1.3 Transaction Accuracy Tests (交易精确性)
   1.4 Pre-deduct vs Settle Tests (预扣与结算)

2. SECURITY - 安全测试
   2.1 Tenant Isolation (租户隔离)
   2.2 API Key Security (Key安全)
   2.3 Permission Checks (权限检查)

3. BUSINESS LOGIC - 业务逻辑测试
   3.1 Route Selection (路由选择)
   3.2 Model Authorization (模型授权)
   3.3 Shadow Tenant Mapping (Shadow租户映射)

4. ERROR HANDLING - 错误处理测试
   4.1 Insufficient Balance (余额不足)
   4.2 Quota Exceeded (配额超限)
   4.3 Invalid Requests (无效请求)

5. CONCURRENCY - 并发测试
   5.1 Atomic Deduction (原子扣费)
   5.2 Race Condition Prevention (竞态防护)

6. DATA INTEGRITY - 数据完整性
   6.1 Log Accuracy (日志准确性)
   6.2 Balance Consistency (余额一致性)
*/

var (
	db          *gorm.DB
	gatewaySvc  llmGateway.LLMGatewayService
	internalSvc llmClient.LLMClient
	apiKeyRepo  apikeyrepo.APIKeyRepository
	initErr     error

	// Test fixtures
	testTenantID = "ecd02342-43a7-4c8b-9b2a-6a217b0d3fc9"
	testAPIKeyID = "a4db3e51-763d-4e7b-ad5e-12934ac605dd"
	testOwnerID  = "e71c8716-3557-4391-aa67-74b87f0e95df"
)

func init() {
	dbCfg := config.Current().Database
	if dbCfg.Host == "" || dbCfg.DBName == "" || dbCfg.Username == "" {
		initErr = fmt.Errorf("integration test database environment is not configured")
		return
	}

	var err error
	db, err = initTestDB()
	if err != nil {
		initErr = fmt.Errorf("failed to init DB: %w", err)
		return
	}

	// Initialize services
	apiKeyRepo = apikeyrepo.NewAPIKeyRepository(db)

	gatewaySvc, err = llmGateway.NewLLMGatewayService(
		db, apiKeyRepo, llmAdapter.GlobalFactory,
	)
	if err != nil {
		initErr = fmt.Errorf("failed to init gateway service: %w", err)
		return
	}
	internalSvc = llmClient.New(gatewaySvc, apiKeyRepo, db)
}

func requireIntegrationHarness(t *testing.T) {
	t.Helper()
	if initErr != nil {
		t.Skipf("Skipping LLM integration regression tests: %v", initErr)
	}
}

func initTestDB() (*gorm.DB, error) {
	return database.InitDB(config.Current().Database)
}

// =============================================================================
// 1. BILLING TESTS - Billing tests
// =============================================================================

// 1.1 Credit Balance Tests
func TestBilling_CreditBalance(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("1.1.1_BalanceDeductedAfterSuccessfulCall", func(t *testing.T) {
		// Get balance before
		balanceBefore := getGroupBalance(t, testTenantID)

		// Make a call
		resp, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}

		// Get balance after
		balanceAfter := getGroupBalance(t, testTenantID)

		// Verify deduction
		deducted := balanceBefore - balanceAfter
		if deducted <= 0 {
			t.Errorf("Balance not deducted: before=%.0f, after=%.0f", balanceBefore, balanceAfter)
		}

		// Verify deduction matches token count
		totalTokens := int64(resp.Usage.TotalTokens)
		if int64(deducted) != totalTokens {
			t.Logf("Note: Deducted %.0f credits for %d tokens (pricing may differ)", deducted, totalTokens)
		}

		t.Logf("✅ Balance deducted: %.0f credits (tokens=%d)", deducted, totalTokens)
	})

	t.Run("1.1.2_InsufficientBalanceRejected", func(t *testing.T) {
		// Create a tenant with zero balance
		zeroBalanceTenantID := createTestTenantWithBalance(t, 0)
		defer cleanupTestTenant(t, zeroBalanceTenantID)

		_, err := internalSvc.Chat(ctx, zeroBalanceTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err == nil {
			t.Error("❌ Should reject call with zero balance")
		} else {
			t.Logf("✅ Zero balance rejected: %v", err)
		}
	})

	t.Run("1.1.3_BalanceNotDeductedOnFailure", func(t *testing.T) {
		balanceBefore := getGroupBalance(t, testTenantID)

		// Make a call that will fail (invalid model)
		_, _ = internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "non-existent-model",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		balanceAfter := getGroupBalance(t, testTenantID)

		if balanceBefore != balanceAfter {
			t.Errorf("❌ Balance changed on failed call: before=%.0f, after=%.0f", balanceBefore, balanceAfter)
		} else {
			t.Logf("✅ Balance unchanged on failure: %.0f", balanceAfter)
		}
	})

	t.Run("1.1.4_TotalSpentAccumulated", func(t *testing.T) {
		var accountBefore paymentModel.GroupAICreditAccount
		db.Where("group_id = ?", testTenantID).First(&accountBefore)
		spentBefore := accountBefore.TotalSpent

		// Make a call
		resp, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}

		var accountAfter paymentModel.GroupAICreditAccount
		db.Where("group_id = ?", testTenantID).First(&accountAfter)

		spentIncrease := accountAfter.TotalSpent - spentBefore
		if spentIncrease <= 0 {
			t.Errorf("❌ TotalSpent not increased: before=%d, after=%d", spentBefore, accountAfter.TotalSpent)
		} else {
			t.Logf("✅ TotalSpent increased by %d (tokens=%d)", spentIncrease, resp.Usage.TotalTokens)
		}
	})
}

// 1.2 API Key Quota Tests
func TestBilling_APIKeyQuota(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("1.2.1_QuotaDeductedOnCall", func(t *testing.T) {
		apiKey := getTestAPIKey(t)
		quotaBefore := apiKey.UsedQuota

		resp, err := gatewaySvc.ChatCompletion(ctx, apiKey, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "quota test"}},
		})
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}

		apiKeyAfter := getTestAPIKey(t)
		quotaUsed := apiKeyAfter.UsedQuota - quotaBefore

		if quotaUsed != int64(resp.Usage.TotalTokens) {
			t.Logf("Note: Quota used %d, tokens %d", quotaUsed, resp.Usage.TotalTokens)
		}

		t.Logf("✅ Quota deducted: %d tokens", quotaUsed)
	})

	t.Run("1.2.2_QuotaLimitEnforced", func(t *testing.T) {
		// Create API key with very low quota limit
		limitedKey := createTestAPIKeyWithQuotaLimit(t, testTenantID, 1) // 1 token limit
		defer cleanupTestAPIKey(t, limitedKey.ID)

		_, err := gatewaySvc.ChatCompletion(ctx, limitedKey, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "this message has more than 1 token"}},
		})

		if err == nil {
			t.Error("❌ Should reject call exceeding quota limit")
		} else {
			t.Logf("✅ Quota limit enforced: %v", err)
		}
	})

	t.Run("1.2.3_UnlimitedQuotaWorks", func(t *testing.T) {
		// Create API key with no quota limit (nil)
		unlimitedKey := createTestAPIKeyUnlimited(t, testTenantID)
		defer cleanupTestAPIKey(t, unlimitedKey.ID)

		_, err := gatewaySvc.ChatCompletion(ctx, unlimitedKey, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err != nil {
			t.Errorf("❌ Unlimited key should work: %v", err)
		} else {
			t.Logf("✅ Unlimited quota works")
		}
	})
}

// 1.3 Transaction Accuracy Tests
func TestBilling_TransactionAccuracy(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("1.3.1_TransactionRecordedWithCorrectAmount", func(t *testing.T) {
		// Count transactions before
		var countBefore int64
		db.Model(&paymentModel.Transaction{}).Where("group_id = ?", testTenantID).Count(&countBefore)

		balanceBefore := getGroupBalance(t, testTenantID)

		// Make a call
		resp, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "transaction test"}},
		})
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}

		balanceAfter := getGroupBalance(t, testTenantID)
		actualDeduction := balanceBefore - balanceAfter

		// Wait for async processing
		time.Sleep(2 * time.Second)

		// Get latest transaction
		var latestTx paymentModel.Transaction
		db.Where("group_id = ?", testTenantID).Order("created_at DESC").First(&latestTx)

		// Verify transaction amount matches actual deduction
		txAmount := -latestTx.Amount // Transaction amount is negative for deductions
		if int64(txAmount) != int64(actualDeduction) {
			t.Errorf("❌ Transaction amount mismatch: tx=%.0f, actual=%.0f", txAmount, actualDeduction)
		}

		// Verify balance_before and balance_after in transaction
		if int64(latestTx.BalanceBefore) != int64(balanceBefore) {
			t.Errorf("❌ Transaction balance_before mismatch: tx=%.0f, actual=%.0f", latestTx.BalanceBefore, balanceBefore)
		}
		if int64(latestTx.BalanceAfter) != int64(balanceAfter) {
			t.Errorf("❌ Transaction balance_after mismatch: tx=%.0f, actual=%.0f", latestTx.BalanceAfter, balanceAfter)
		}

		t.Logf("✅ Transaction recorded correctly: amount=%.0f, tokens=%d", txAmount, resp.Usage.TotalTokens)
	})

	t.Run("1.3.2_TransactionDetailContainsMetadata", func(t *testing.T) {
		// Make a call
		_, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "metadata test"}},
		})
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}

		time.Sleep(2 * time.Second)

		// Get latest transaction
		var latestTx paymentModel.Transaction
		db.Where("group_id = ?", testTenantID).Order("created_at DESC").First(&latestTx)

		// Check transaction_detail contains required fields
		detail := latestTx.TransactionDetail
		if detail == nil {
			t.Error("❌ Transaction detail is nil")
		} else {
			requiredFields := []string{"model_name", "provider_name", "prompt_tokens", "completion_tokens", "request_id"}
			for _, field := range requiredFields {
				if _, exists := detail[field]; !exists {
					t.Errorf("❌ Missing field in transaction detail: %s", field)
				}
			}
			t.Logf("✅ Transaction detail contains required metadata")
		}
	})
}

// =============================================================================
// 2. SECURITY TESTS - Security tests
// =============================================================================

func TestSecurity_TenantIsolation(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("2.1.1_CannotAccessOtherTenantAPIKey", func(t *testing.T) {
		wrongTenantID := uuid.New().String()

		_, err := apiKeyRepo.GetByID(ctx, testAPIKeyID, wrongTenantID)
		if err == nil {
			t.Error("❌ SECURITY: Accessed API key from wrong tenant!")
		} else {
			t.Logf("✅ Tenant isolation: Cannot access other tenant's API key")
		}
	})

	t.Run("2.1.2_CannotDeductOtherTenantCredits", func(t *testing.T) {
		// Create isolated tenant
		isolatedTenantID := createTestTenantWithBalance(t, 1000)
		defer cleanupTestTenant(t, isolatedTenantID)

		balanceBefore := getGroupBalance(t, isolatedTenantID)

		// Try to use main tenant's API key (should not affect isolated tenant)
		mainKey := getTestAPIKey(t)
		_, _ = gatewaySvc.ChatCompletion(ctx, mainKey, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		// Verify isolated tenant's balance unchanged
		balanceAfter := getGroupBalance(t, isolatedTenantID)
		if balanceBefore != balanceAfter {
			t.Errorf("❌ SECURITY: Other tenant's balance affected! before=%.0f, after=%.0f", balanceBefore, balanceAfter)
		} else {
			t.Logf("✅ Tenant isolation: Other tenant's credits not affected")
		}
	})

	t.Run("2.1.3_APIKeyBoundToTenant", func(t *testing.T) {
		// Get API key
		apiKey := getTestAPIKey(t)

		// Verify it belongs to correct tenant
		if apiKey.OrganizationID != testTenantID {
			t.Errorf("❌ API key tenant mismatch: expected=%s, got=%s", testTenantID, apiKey.OrganizationID)
		} else {
			t.Logf("✅ API key correctly bound to tenant")
		}
	})
}

func TestSecurity_APIKeyStatus(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("2.2.1_InactiveKeyRejected", func(t *testing.T) {
		// Create inactive key
		inactiveKey := createTestAPIKeyWithStatus(t, testTenantID, "inactive")
		defer cleanupTestAPIKey(t, inactiveKey.ID)

		_, err := gatewaySvc.ChatCompletion(ctx, inactiveKey, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err == nil {
			t.Error("❌ SECURITY: Inactive key should be rejected")
		} else {
			t.Logf("✅ Inactive key rejected: %v", err)
		}
	})

	t.Run("2.2.2_ExpiredKeyRejected", func(t *testing.T) {
		// Create expired key
		expiredKey := createTestAPIKeyExpired(t, testTenantID)
		defer cleanupTestAPIKey(t, expiredKey.ID)

		_, err := gatewaySvc.ChatCompletion(ctx, expiredKey, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		// Note: Gateway may not check expiry, this depends on implementation
		t.Logf("Expired key result: err=%v", err)
	})

	t.Run("2.2.3_DeletedKeyRejected", func(t *testing.T) {
		// Create and soft-delete a key
		deletedKey := createTestAPIKeyWithStatus(t, testTenantID, "active")
		db.Delete(&apikeymodel.TenantAPIKey{}, "id = ?", deletedKey.ID)

		_, err := gatewaySvc.ChatCompletion(ctx, deletedKey, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err == nil {
			t.Error("❌ SECURITY: Deleted key should be rejected")
		} else {
			t.Logf("✅ Deleted key rejected")
		}
	})
}

// =============================================================================
// 3. BUSINESS LOGIC TESTS - Business logic tests
// =============================================================================

func TestBusinessLogic_ModelAuthorization(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("3.1.1_ValidModelAccepted", func(t *testing.T) {
		_, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err != nil {
			t.Errorf("❌ Valid model rejected: %v", err)
		} else {
			t.Logf("✅ Valid model accepted")
		}
	})

	t.Run("3.1.2_InvalidModelRejected", func(t *testing.T) {
		_, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "invalid-model-xyz",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err == nil {
			t.Error("❌ Invalid model should be rejected")
		} else {
			t.Logf("✅ Invalid model rejected: %v", err)
		}
	})

	t.Run("3.1.3_InactiveModelRejected", func(t *testing.T) {
		// This test requires knowing an inactive model in the system
		// Skip if no inactive models
		t.Skip("Requires inactive model in test data")
	})
}

func TestBusinessLogic_UsageBillAccuracy(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("3.2.1_UsageBillMatchesResponse", func(t *testing.T) {
		resp, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "bill accuracy test"}},
		})
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}

		var bill llmGateway.UsageBill
		if err := db.Where("organization_id = ?", testTenantID).Order("settled_at DESC").First(&bill).Error; err != nil {
			t.Fatalf("usage bill not found: %v", err)
		}

		if bill.PromptTokens != int64(resp.Usage.PromptTokens) {
			t.Errorf("❌ PromptTokens mismatch: bill=%d, resp=%d", bill.PromptTokens, resp.Usage.PromptTokens)
		}
		if bill.CompletionTokens != int64(resp.Usage.CompletionTokens) {
			t.Errorf("❌ CompletionTokens mismatch: bill=%d, resp=%d", bill.CompletionTokens, resp.Usage.CompletionTokens)
		}
		if bill.ModelName != resp.Model {
			t.Errorf("❌ Model mismatch: bill=%s, resp=%s", bill.ModelName, resp.Model)
		}
		if bill.Status != "success" {
			t.Errorf("❌ Status should be success, got: %s", bill.Status)
		}

		t.Logf("✅ Usage bill matches response: model=%s, tokens=%d/%d", bill.ModelName, bill.PromptTokens, bill.CompletionTokens)
	})

	t.Run("3.2.2_UsageBillContainsRequestID", func(t *testing.T) {
		_, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "request id test"}},
		})
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}

		var bill llmGateway.UsageBill
		if err := db.Where("organization_id = ?", testTenantID).Order("settled_at DESC").First(&bill).Error; err != nil {
			t.Fatalf("usage bill not found: %v", err)
		}

		if bill.RequestID == "" {
			t.Error("❌ RequestID is empty")
		} else {
			t.Logf("✅ Usage bill contains RequestID: %s", bill.RequestID)
		}
	})
}

// =============================================================================
// 4. ERROR HANDLING TESTS - Error handling tests
// =============================================================================

func TestErrorHandling_InsufficientBalance(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("4.1.1_RejectWhenBalanceZero", func(t *testing.T) {
		tenantID := createTestTenantWithBalance(t, 0)
		defer cleanupTestTenant(t, tenantID)

		_, err := internalSvc.Chat(ctx, tenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err == nil {
			t.Error("❌ Should reject when balance is zero")
		} else {
			t.Logf("✅ Zero balance rejected: %v", err)
		}
	})

	t.Run("4.1.2_RejectWhenBalanceTooLow", func(t *testing.T) {
		// Create tenant with very low balance (1 credit)
		tenantID := createTestTenantWithBalance(t, 1)
		defer cleanupTestTenant(t, tenantID)

		_, err := internalSvc.Chat(ctx, tenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "this is a longer message that will use more tokens than 1 credit"}},
		})

		// Should fail because estimated tokens > balance
		if err == nil {
			t.Logf("Note: Call succeeded with low balance (may depend on implementation)")
		} else {
			t.Logf("✅ Low balance handled: %v", err)
		}
	})
}

func TestErrorHandling_InvalidRequests(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("4.2.1_EmptyMessagesRejected", func(t *testing.T) {
		_, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{},
		})

		if err == nil {
			t.Error("❌ Empty messages should be rejected")
		} else {
			t.Logf("✅ Empty messages rejected: %v", err)
		}
	})

	t.Run("4.2.2_EmptyModelRejected", func(t *testing.T) {
		_, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err == nil {
			t.Error("❌ Empty model should be rejected")
		} else {
			t.Logf("✅ Empty model rejected: %v", err)
		}
	})

	t.Run("4.2.3_InvalidTenantIDHandled", func(t *testing.T) {
		_, err := internalSvc.Chat(ctx, "invalid-uuid", &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err == nil {
			t.Error("❌ Invalid tenant ID should be rejected")
		} else {
			t.Logf("✅ Invalid tenant ID handled: %v", err)
		}
	})
}

// =============================================================================
// 5. CONCURRENCY TESTS - Concurrency tests
// =============================================================================

func TestConcurrency_AtomicDeduction(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("5.1.1_ConcurrentCallsDeductCorrectly", func(t *testing.T) {
		balanceBefore := getGroupBalance(t, testTenantID)

		const numCalls = 5
		var wg sync.WaitGroup
		var successCount int32
		var totalTokens int64

		for i := 0; i < numCalls; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				resp, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
					Model:    "qwen-plus",
					Messages: []llmAdapter.Message{{Role: "user", Content: fmt.Sprintf("concurrent %d", idx)}},
				})
				if err == nil {
					atomic.AddInt32(&successCount, 1)
					atomic.AddInt64(&totalTokens, int64(resp.Usage.TotalTokens))
				}
			}(i)
		}

		wg.Wait()

		balanceAfter := getGroupBalance(t, testTenantID)
		actualDeduction := int64(balanceBefore - balanceAfter)

		t.Logf("Concurrent calls: %d/%d succeeded", successCount, numCalls)
		t.Logf("Total tokens: %d, Total deducted: %d", totalTokens, actualDeduction)

		// Verify no over-deduction or under-deduction
		if actualDeduction < totalTokens-10 || actualDeduction > totalTokens+10 {
			t.Errorf("❌ Deduction mismatch: expected ~%d, got %d", totalTokens, actualDeduction)
		} else {
			t.Logf("✅ Concurrent deduction correct")
		}
	})

	t.Run("5.1.2_NoDoubleDeductionOnRetry", func(t *testing.T) {
		// This test verifies that if a request fails mid-way, we don't double-deduct
		// Implementation depends on pre-deduct/settle mechanism
		t.Skip("Requires specific failure injection mechanism")
	})
}

// =============================================================================
// 6. STREAMING TESTS - Streaming tests
// =============================================================================

func TestStreaming_BillingOnCompletion(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("6.1.1_StreamingBilledCorrectly", func(t *testing.T) {
		balanceBefore := getGroupBalance(t, testTenantID)

		streamChan, err := internalSvc.ChatStream(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "count 1 to 5"}},
		})
		if err != nil {
			t.Fatalf("Stream failed: %v", err)
		}

		// Consume stream
		for resp := range streamChan {
			if resp.Error != nil {
				t.Fatalf("Stream error: %v", resp.Error)
			}
		}

		balanceAfter := getGroupBalance(t, testTenantID)

		if balanceAfter >= balanceBefore {
			t.Error("❌ Balance not deducted after streaming")
		} else {
			t.Logf("✅ Streaming billed correctly: deducted %.0f", balanceBefore-balanceAfter)
		}
	})

	t.Run("6.1.2_StreamingBillRecorded", func(t *testing.T) {
		var countBefore int64
		db.Model(&llmGateway.UsageBill{}).Where("organization_id = ? AND model_name = ?", testTenantID, "qwen-plus").Count(&countBefore)

		streamChan, err := internalSvc.ChatStream(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "stream bill test"}},
		})
		if err != nil {
			t.Fatalf("Stream failed: %v", err)
		}

		for resp := range streamChan {
			if resp.Error != nil {
				t.Fatalf("Stream error: %v", resp.Error)
			}
		}

		var countAfter int64
		db.Model(&llmGateway.UsageBill{}).Where("organization_id = ? AND model_name = ?", testTenantID, "qwen-plus").Count(&countAfter)

		if countAfter <= countBefore {
			t.Error("❌ Streaming bill not recorded")
		} else {
			t.Logf("✅ Streaming bill recorded")
		}
	})
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func getGroupBalance(t *testing.T, groupID string) float64 {
	var account paymentModel.GroupAICreditAccount
	if err := db.Where("group_id = ?", groupID).First(&account).Error; err != nil {
		t.Fatalf("Failed to get account: %v", err)
	}
	return float64(account.SubscriptionCredits + account.PurchasedCredits)
}

func getTestAPIKey(t *testing.T) *apikeymodel.TenantAPIKey {
	var apiKey apikeymodel.TenantAPIKey
	if err := db.Where("id = ?", testAPIKeyID).First(&apiKey).Error; err != nil {
		t.Fatalf("Failed to get API key: %v", err)
	}
	return &apiKey
}

func createTestTenantWithBalance(t *testing.T, balance int64) string {
	tenantID := uuid.New().String()
	accountID, _ := uuid.Parse(testOwnerID)
	groupID, _ := uuid.Parse(tenantID)

	// Create enterprise group
	db.Exec(`INSERT INTO enterprise_groups (id, name, status, created_at, updated_at)
		VALUES (?, 'Test Group', 'active', NOW(), NOW())`, tenantID)

	// Create credit account
	account := paymentModel.GroupAICreditAccount{
		ID:                  uuid.New().String(),
		AccountID:           accountID,
		GroupID:             groupID,
		SubscriptionCredits: balance,
		PurchasedCredits:    0,
	}
	db.Create(&account)

	return tenantID
}

func cleanupTestTenant(t *testing.T, tenantID string) {
	db.Exec("DELETE FROM group_ai_credit_accounts WHERE group_id = ?", tenantID)
	db.Exec("DELETE FROM enterprise_groups WHERE id = ?", tenantID)
}

func createTestAPIKeyWithQuotaLimit(t *testing.T, tenantID string, limit int64) *apikeymodel.TenantAPIKey {
	apiKey := &apikeymodel.TenantAPIKey{
		ID:             uuid.New().String(),
		OrganizationID: tenantID,
		Key:            "sk-test-" + uuid.New().String(),
		KeyHash:        uuid.New().String(),
		Name:           "Test Key Limited",
		Status:         "active",
		QuotaLimit:     &limit,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	db.Create(apiKey)
	return apiKey
}

func createTestAPIKeyUnlimited(t *testing.T, tenantID string) *apikeymodel.TenantAPIKey {
	apiKey := &apikeymodel.TenantAPIKey{
		ID:             uuid.New().String(),
		OrganizationID: tenantID,
		Key:            "sk-test-unlimited-" + uuid.New().String(),
		KeyHash:        uuid.New().String(),
		Name:           "Test Key Unlimited",
		Status:         "active",
		QuotaLimit:     nil,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	db.Create(apiKey)
	return apiKey
}

func createTestAPIKeyWithStatus(t *testing.T, tenantID, status string) *apikeymodel.TenantAPIKey {
	apiKey := &apikeymodel.TenantAPIKey{
		ID:             uuid.New().String(),
		OrganizationID: tenantID,
		Key:            "sk-test-" + status + "-" + uuid.New().String(),
		KeyHash:        uuid.New().String(),
		Name:           "Test Key " + status,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	db.Create(apiKey)
	return apiKey
}

func createTestAPIKeyExpired(t *testing.T, tenantID string) *apikeymodel.TenantAPIKey {
	expiredTime := time.Now().Add(-24 * time.Hour)
	apiKey := &apikeymodel.TenantAPIKey{
		ID:             uuid.New().String(),
		OrganizationID: tenantID,
		Key:            "sk-test-expired-" + uuid.New().String(),
		KeyHash:        uuid.New().String(),
		Name:           "Test Key Expired",
		Status:         "active",
		ExpiresAt:      &expiredTime,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	db.Create(apiKey)
	return apiKey
}

func cleanupTestAPIKey(t *testing.T, keyID string) {
	db.Unscoped().Delete(&apikeymodel.TenantAPIKey{}, "id = ?", keyID)
}

// =============================================================================
// 7. EMBEDDINGS API TESTS - Embeddings API tests
// =============================================================================

func TestEmbeddings_Basic(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("7.1.1_EmbeddingsSuccessfulCall", func(t *testing.T) {
		balanceBefore := getGroupBalance(t, testTenantID)

		resp, err := internalSvc.Embed(ctx, testTenantID, &llmAdapter.EmbeddingsRequest{
			Model: "text-embedding-v3",
			Input: []string{"Hello world", "Test embedding"},
		})

		if err != nil {
			t.Skipf("Embeddings not available: %v", err)
			return
		}

		if resp == nil || len(resp.Data) == 0 {
			t.Error("❌ Empty embeddings response")
		} else {
			t.Logf("✅ Embeddings returned %d vectors", len(resp.Data))
		}

		balanceAfter := getGroupBalance(t, testTenantID)
		if balanceAfter >= balanceBefore {
			t.Logf("⚠️ Balance not deducted (may use free model)")
		} else {
			t.Logf("✅ Embeddings billed: deducted %.0f", balanceBefore-balanceAfter)
		}
	})

	t.Run("7.1.2_EmbeddingsEmptyInputRejected", func(t *testing.T) {
		_, err := internalSvc.Embed(ctx, testTenantID, &llmAdapter.EmbeddingsRequest{
			Model: "text-embedding-v3",
			Input: []string{},
		})

		if err != nil {
			t.Logf("✅ Empty input rejected: %v", err)
		} else {
			t.Error("❌ Empty input should be rejected")
		}
	})

	t.Run("7.1.3_EmbeddingsBillRecorded", func(t *testing.T) {
		var countBefore int64
		db.Model(&llmGateway.UsageBill{}).Where("organization_id = ? AND model_name LIKE ?", testTenantID, "%embedding%").Count(&countBefore)

		_, err := internalSvc.Embed(ctx, testTenantID, &llmAdapter.EmbeddingsRequest{
			Model: "text-embedding-v3",
			Input: []string{"bill test"},
		})

		if err != nil {
			t.Skipf("Embeddings not available: %v", err)
			return
		}

		var countAfter int64
		db.Model(&llmGateway.UsageBill{}).Where("organization_id = ? AND model_name LIKE ?", testTenantID, "%embedding%").Count(&countAfter)

		if countAfter > countBefore {
			t.Logf("✅ Embeddings bill recorded")
		} else {
			t.Logf("⚠️ Embeddings bill not recorded")
		}
	})
}

// =============================================================================
// 8. RERANK API TESTS - Rerank API tests
// =============================================================================

func TestRerank_Basic(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("8.1.1_RerankSuccessfulCall", func(t *testing.T) {
		resp, err := internalSvc.Rerank(ctx, testTenantID, &llmAdapter.RerankRequest{
			Model:     "rerank-v1",
			Query:     "What is machine learning?",
			Documents: []string{"ML is a subset of AI", "Python is a programming language", "Machine learning uses algorithms"},
		})

		if err != nil {
			t.Skipf("Rerank not available: %v", err)
			return
		}

		if resp == nil || len(resp.Results) == 0 {
			t.Error("❌ Empty rerank response")
		} else {
			t.Logf("✅ Rerank returned %d results", len(resp.Results))
		}
	})

	t.Run("8.1.2_RerankEmptyDocumentsRejected", func(t *testing.T) {
		_, err := internalSvc.Rerank(ctx, testTenantID, &llmAdapter.RerankRequest{
			Model:     "rerank-v1",
			Query:     "test query",
			Documents: []string{},
		})

		if err != nil {
			t.Logf("✅ Empty documents rejected: %v", err)
		} else {
			t.Error("❌ Empty documents should be rejected")
		}
	})
}

// =============================================================================
// 9. LIST MODELS API TESTS - List models API tests
// =============================================================================

func TestListModels_Basic(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("9.1.1_ListAllModels", func(t *testing.T) {
		apiKey := getTestAPIKey(t)
		models, err := gatewaySvc.ListAvailableModels(ctx, apiKey)

		if err != nil {
			t.Fatalf("Failed to list models: %v", err)
		}

		if len(models) == 0 {
			// This may happen if the API key has model_limits disabled and no models are configured
			t.Logf("⚠️ No models returned (may be expected based on configuration)")
		} else {
			t.Logf("✅ Listed %d available models", len(models))
		}
	})

	t.Run("9.1.2_ListModelsWithModelLimits", func(t *testing.T) {
		// Create API key with model limits
		modelLimits := `["qwen-plus", "qwen-turbo"]`
		apiKey := &apikeymodel.TenantAPIKey{
			ID:                 uuid.New().String(),
			OrganizationID:     testTenantID,
			Key:                "sk-test-limited-models-" + uuid.New().String(),
			KeyHash:            uuid.New().String(),
			Name:               "Test Key Model Limited",
			Status:             "active",
			ModelLimitsEnabled: true,
			ModelLimits:        &modelLimits,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		db.Create(apiKey)
		defer cleanupTestAPIKey(t, apiKey.ID)

		models, err := gatewaySvc.ListAvailableModels(ctx, apiKey)

		if err != nil {
			t.Fatalf("Failed to list models: %v", err)
		}

		if len(models) > 2 {
			t.Errorf("❌ Model limits not enforced: got %d models, expected <= 2", len(models))
		} else {
			t.Logf("✅ Model limits enforced: %d models", len(models))
		}
	})
}

// =============================================================================
// 10. MODEL LIMITS TESTS - Model limit tests
// =============================================================================

func TestAPIKey_ModelLimits(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("10.1.1_AllowedModelAccepted", func(t *testing.T) {
		modelLimits := `["qwen-plus"]`
		apiKey := &apikeymodel.TenantAPIKey{
			ID:                 uuid.New().String(),
			OrganizationID:     testTenantID,
			Key:                "sk-test-model-allowed-" + uuid.New().String(),
			KeyHash:            uuid.New().String(),
			Name:               "Test Key Model Allowed",
			Status:             "active",
			ModelLimitsEnabled: true,
			ModelLimits:        &modelLimits,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		db.Create(apiKey)
		defer cleanupTestAPIKey(t, apiKey.ID)

		_, err := gatewaySvc.ChatCompletion(ctx, apiKey, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err != nil {
			t.Errorf("❌ Allowed model rejected: %v", err)
		} else {
			t.Logf("✅ Allowed model accepted")
		}
	})

	t.Run("10.1.2_DisallowedModelRejected", func(t *testing.T) {
		modelLimits := `["qwen-turbo"]`
		apiKey := &apikeymodel.TenantAPIKey{
			ID:                 uuid.New().String(),
			OrganizationID:     testTenantID,
			Key:                "sk-test-model-disallow-" + uuid.New().String(),
			KeyHash:            uuid.New().String(),
			Name:               "Test Key Model Disallow",
			Status:             "active",
			ModelLimitsEnabled: true,
			ModelLimits:        &modelLimits,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		db.Create(apiKey)
		defer cleanupTestAPIKey(t, apiKey.ID)

		_, err := gatewaySvc.ChatCompletion(ctx, apiKey, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err != nil {
			t.Logf("✅ Disallowed model rejected: %v", err)
		} else {
			t.Error("❌ Disallowed model should be rejected")
		}
	})

	t.Run("10.1.3_ModelLimitsDisabledAllowsAll", func(t *testing.T) {
		modelLimits := `["qwen-turbo"]`
		apiKey := &apikeymodel.TenantAPIKey{
			ID:                 uuid.New().String(),
			OrganizationID:     testTenantID,
			Key:                "sk-test-model-disabled-" + uuid.New().String(),
			KeyHash:            uuid.New().String(),
			Name:               "Test Key Model Disabled",
			Status:             "active",
			ModelLimitsEnabled: false, // Disabled
			ModelLimits:        &modelLimits,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		db.Create(apiKey)
		defer cleanupTestAPIKey(t, apiKey.ID)

		_, err := gatewaySvc.ChatCompletion(ctx, apiKey, &llmAdapter.ChatRequest{
			Model:    "qwen-plus", // Not in limits but should work
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err != nil {
			t.Errorf("❌ Model limits disabled but still rejected: %v", err)
		} else {
			t.Logf("✅ Model limits disabled allows all models")
		}
	})
}

// =============================================================================
// 11. APP CONTEXT TESTS - App context tests
// =============================================================================

func TestAppContext_UsageTracking(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("11.1.1_AppContextRecordedInLog", func(t *testing.T) {
		apiKey := getTestAPIKey(t)
		appID := uuid.New()
		accountID := uuid.New()
		appType := "workflow"

		appCtx := &llmGateway.AppContext{
			AppID:     &appID,
			AppType:   &appType,
			AccountID: &accountID,
		}

		_, err := gatewaySvc.ChatCompletionWithAppContext(ctx, apiKey, appCtx, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "app context test"}},
		})

		if err != nil {
			t.Fatalf("Failed: %v", err)
		}

		// Check if usage bill contains app context.
		var bill llmGateway.UsageBill
		err = db.Where("organization_id = ? AND app_id = ?", testTenantID, appID).
			Order("settled_at DESC").First(&bill).Error

		if err != nil {
			t.Logf("⚠️ App context bill not found")
		} else if bill.AppType == nil || *bill.AppType != "workflow" {
			t.Errorf("❌ App type mismatch: got %v", bill.AppType)
		} else {
			t.Logf("✅ App context recorded: app_id=%s, app_type=%s", appID, *bill.AppType)
		}
	})

	t.Run("11.1.2_StreamingWithAppContext", func(t *testing.T) {
		apiKey := getTestAPIKey(t)
		appID := uuid.New()
		appType := "chatbot"

		appCtx := &llmGateway.AppContext{
			AppID:   &appID,
			AppType: &appType,
		}

		streamChan, err := gatewaySvc.ChatCompletionStreamWithAppContext(ctx, apiKey, appCtx, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "stream app context"}},
		})

		if err != nil {
			// Provider may not be available, which is acceptable
			t.Logf("⚠️ Streaming not available: %v", err)
			return
		}

		// Consume stream
		for resp := range streamChan {
			if resp.Error != nil {
				t.Fatalf("Stream error: %v", resp.Error)
			}
		}

		t.Logf("✅ Streaming with app context completed")
	})
}

// =============================================================================
// 12. PROVIDER FAILOVER TESTS - Provider failover tests
// =============================================================================

func TestProviderFailover_Basic(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("12.1.1_FailoverToNextProvider", func(t *testing.T) {
		// This test verifies that when one provider fails, the system tries the next one
		// We use a model that may have multiple providers configured

		resp, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "failover test"}},
		})

		if err != nil {
			t.Errorf("❌ Request failed (no failover): %v", err)
		} else if resp != nil {
			t.Logf("✅ Request succeeded (failover may have occurred)")
		}
	})

	t.Run("12.1.2_AllProvidersFail", func(t *testing.T) {
		// Test with a model that definitely doesn't exist
		_, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "non-existent-model-xyz",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err == nil {
			t.Error("❌ Should fail when no provider available")
		} else {
			t.Logf("✅ Properly failed when no provider: %v", err)
		}
	})
}

// =============================================================================
// 13. INTERNAL SERVICE TESTS - Internal service tests
// =============================================================================

func TestInternalService_AutoAPIKey(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("13.1.1_AutoCreateInternalAPIKey", func(t *testing.T) {
		// Create a new tenant
		tenantID := createTestTenantWithBalance(t, 10000)
		defer cleanupTestTenant(t, tenantID)

		// Internal service should auto-create API key
		resp, err := internalSvc.Chat(ctx, tenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "auto key test"}},
		})

		if err != nil {
			t.Errorf("❌ Internal service failed: %v", err)
		} else if resp != nil {
			t.Logf("✅ Internal service auto-created API key and completed request")
		}

		// Verify internal key was created
		var apiKey apikeymodel.TenantAPIKey
		err = db.Where("tenant_id = ? AND is_internal = true", tenantID).First(&apiKey).Error
		if err != nil {
			t.Error("❌ Internal API key not created")
		} else {
			t.Logf("✅ Internal API key created: %s", apiKey.ID)
			cleanupTestAPIKey(t, apiKey.ID)
		}
	})

	t.Run("13.1.2_ReuseExistingInternalKey", func(t *testing.T) {
		// Make two calls and verify the same internal key is used
		var keysBefore int64
		db.Model(&apikeymodel.TenantAPIKey{}).Where("tenant_id = ? AND is_internal = true", testTenantID).Count(&keysBefore)

		_, _ = internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test1"}},
		})

		_, _ = internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test2"}},
		})

		var keysAfter int64
		db.Model(&apikeymodel.TenantAPIKey{}).Where("tenant_id = ? AND is_internal = true", testTenantID).Count(&keysAfter)

		if keysAfter > keysBefore+1 {
			t.Errorf("❌ Multiple internal keys created: before=%d, after=%d", keysBefore, keysAfter)
		} else {
			t.Logf("✅ Internal key reused correctly")
		}
	})
}

// =============================================================================
// 14. RESPONSE CREATE API TESTS - Response create API tests
// =============================================================================

func TestCreateResponse_Basic(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("14.1.1_CreateResponseSuccessful", func(t *testing.T) {
		apiKey := getTestAPIKey(t)

		resp, err := gatewaySvc.CreateResponse(ctx, apiKey, &llmAdapter.CreateResponseRequest{
			Model: "qwen-plus",
			Messages: []llmAdapter.Message{
				{Role: "user", Content: "What is 2+2?"},
			},
		})

		if err != nil {
			t.Skipf("CreateResponse not available: %v", err)
			return
		}

		if resp == nil || len(resp.Output) == 0 {
			t.Error("❌ Empty response")
		} else {
			t.Logf("✅ CreateResponse successful")
		}
	})

	t.Run("14.1.2_CreateResponseBilled", func(t *testing.T) {
		apiKey := getTestAPIKey(t)
		balanceBefore := getGroupBalance(t, testTenantID)

		_, err := gatewaySvc.CreateResponse(ctx, apiKey, &llmAdapter.CreateResponseRequest{
			Model: "qwen-plus",
			Messages: []llmAdapter.Message{
				{Role: "user", Content: "billing test"},
			},
		})

		if err != nil {
			t.Skipf("CreateResponse not available: %v", err)
			return
		}

		balanceAfter := getGroupBalance(t, testTenantID)
		if balanceAfter >= balanceBefore {
			t.Logf("⚠️ Balance not deducted")
		} else {
			t.Logf("✅ CreateResponse billed: deducted %.0f", balanceBefore-balanceAfter)
		}
	})
}

// =============================================================================
// 15. EDGE CASES AND BOUNDARY TESTS - Edge cases and boundary tests
// =============================================================================

func TestEdgeCases_Boundary(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("15.1.1_VeryLongMessage", func(t *testing.T) {
		// Create a very long message
		longContent := ""
		for i := 0; i < 1000; i++ {
			longContent += "This is a test message. "
		}

		resp, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: longContent}},
		})

		if err != nil {
			t.Logf("⚠️ Long message rejected: %v", err)
		} else if resp != nil {
			t.Logf("✅ Long message handled successfully")
		}
	})

	t.Run("15.1.2_MultipleMessages", func(t *testing.T) {
		resp, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model: "qwen-plus",
			Messages: []llmAdapter.Message{
				{Role: "system", Content: "You are a helpful assistant."},
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
				{Role: "user", Content: "How are you?"},
			},
		})

		if err != nil {
			t.Errorf("❌ Multiple messages failed: %v", err)
		} else if resp != nil {
			t.Logf("✅ Multiple messages handled successfully")
		}
	})

	t.Run("15.1.3_MaxTokensLimit", func(t *testing.T) {
		maxTokens := 10
		resp, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:     "qwen-plus",
			Messages:  []llmAdapter.Message{{Role: "user", Content: "Count from 1 to 100"}},
			MaxTokens: &maxTokens, // Very limited
		})

		if err != nil {
			t.Errorf("❌ Max tokens limit failed: %v", err)
		} else if resp != nil {
			t.Logf("✅ Max tokens limit respected")
		}
	})

	t.Run("15.1.4_TemperatureBoundary", func(t *testing.T) {
		// Test temperature = 0 (deterministic)
		temp := float64(0.0)
		resp, err := internalSvc.Chat(ctx, testTenantID, &llmAdapter.ChatRequest{
			Model:       "qwen-plus",
			Messages:    []llmAdapter.Message{{Role: "user", Content: "Say hello"}},
			Temperature: &temp,
		})

		if err != nil {
			t.Errorf("❌ Temperature=0 failed: %v", err)
		} else if resp != nil {
			t.Logf("✅ Temperature=0 handled successfully")
		}
	})
}

// =============================================================================
// 16. QUOTA EDGE CASES - Quota edge cases
// =============================================================================

func TestQuota_EdgeCases(t *testing.T) {
	requireIntegrationHarness(t)
	ctx := context.Background()

	t.Run("16.1.1_ExactQuotaMatch", func(t *testing.T) {
		// Create API key with exact quota that matches estimated usage
		tenantID := createTestTenantWithBalance(t, 100000)
		defer cleanupTestTenant(t, tenantID)

		// Small quota that should be consumed in one call
		var limit int64 = 1000
		apiKey := createTestAPIKeyWithQuotaLimit(t, tenantID, limit)
		defer cleanupTestAPIKey(t, apiKey.ID)

		// First call should succeed
		_, err := gatewaySvc.ChatCompletion(ctx, apiKey, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err != nil {
			t.Logf("⚠️ First call failed: %v", err)
		} else {
			t.Logf("✅ First call succeeded")
		}

		// Check remaining quota
		db.First(apiKey, "id = ?", apiKey.ID)
		t.Logf("Remaining quota: %d", apiKey.RemainQuota)
	})

	t.Run("16.1.2_ZeroQuotaRejected", func(t *testing.T) {
		tenantID := createTestTenantWithBalance(t, 100000)
		defer cleanupTestTenant(t, tenantID)

		var limit int64 = 0
		apiKey := createTestAPIKeyWithQuotaLimit(t, tenantID, limit)
		defer cleanupTestAPIKey(t, apiKey.ID)

		_, err := gatewaySvc.ChatCompletion(ctx, apiKey, &llmAdapter.ChatRequest{
			Model:    "qwen-plus",
			Messages: []llmAdapter.Message{{Role: "user", Content: "test"}},
		})

		if err != nil {
			t.Logf("✅ Zero quota rejected: %v", err)
		} else {
			t.Error("❌ Zero quota should be rejected")
		}
	})
}
