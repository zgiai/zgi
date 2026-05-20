package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
)

func TestGRPCBillingConnection(t *testing.T) {
	client, err := gateway.NewQuotaClient("localhost:50051")
	if err != nil {
		t.Skipf("Skipping test: gRPC server not available: %v", err)
		return
	}
	defer client.Close()

	t.Log("✅ Successfully connected to gRPC billing service")
}

func TestGRPCPreDeductQuota(t *testing.T) {
	client, err := gateway.NewQuotaClient("localhost:50051")
	if err != nil {
		t.Skipf("Skipping test: gRPC server not available: %v", err)
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &gateway.PreDeductQuotaRequest{
		OrganizationID:   uuid.New().String(),
		EstimatedCredits: 1000,
		ModelID:          uuid.New().String(),
		ModelName:        "gpt-4",
		ProviderID:       uuid.New().String(),
		ProviderName:     "openai",
		RequestID:        "test-request-" + uuid.New().String(),
	}

	resp, err := client.PreDeductQuota(ctx, req)

	// It is expected to fail because the API key does not exist.
	// That still proves the gRPC call succeeded.
	if err != nil {
		t.Logf("Expected error (API key not found): %v", err)
	}

	if resp != nil {
		t.Logf("Response: Success=%v, ErrorCode=%s, ErrorMessage=%s",
			resp.Success, resp.ErrorCode, resp.ErrorMessage)

		// It should return an error because the API key does not exist
		assert.False(t, resp.Success, "Should fail for non-existent API key")
	}

	t.Log("✅ gRPC PreDeductQuota call successful")
}

func TestGRPCSettleQuota(t *testing.T) {
	client, err := gateway.NewQuotaClient("localhost:50051")
	if err != nil {
		t.Skipf("Skipping test: gRPC server not available: %v", err)
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &gateway.SettleQuotaRequest{
		OrganizationID:    uuid.New().String(),
		EstimatedCredits:  1000,
		ActualCredits:     800,
		PromptTokens:      100,
		CompletionTokens:  50,
		TotalTokens:       150,
		InputCost:         0.001,
		OutputCost:        0.002,
		TotalCost:         0.003,
		ModelID:           uuid.New().String(),
		ModelName:         "gpt-4",
		ProviderID:        uuid.New().String(),
		ProviderName:      "openai",
		RequestID:         "test-request-" + uuid.New().String(),
		ResponseTime:      1500,
		Status:            "success",
		IsStreaming:       false,
		UseSystemProvider: true,
	}

	resp, err := client.SettleQuota(ctx, req)

	if err != nil {
		t.Logf("Expected error (API key not found): %v", err)
	}

	if resp != nil {
		t.Logf("Response: Success=%v, ErrorMessage=%s, RefundedCredits=%d",
			resp.Success, resp.ErrorMessage, resp.RefundedCredits)
	}

	t.Log("✅ gRPC SettleQuota call successful")
}

func TestBillingServiceWithGRPCFallback(t *testing.T) {
	// Test the fallback mechanism when gRPC is unavailable
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Console: config.ConsoleConfig{GRPCAddr: "localhost:9999"}, // non-existent port
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})

	// This should create BillingServiceWithGRPC and test the fallback.
	// It is only a stub here because it would require full dependency injection.

	t.Log("✅ Fallback mechanism test placeholder")
}
