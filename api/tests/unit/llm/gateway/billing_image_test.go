package gateway_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/gateway"
	llmmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openImageTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:memdb" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("sqlite driver requires cgo in this environment")
		}
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	return db
}

func intPtr(v int) *int { return &v }

func TestBillingService_CalculateImageCredits(t *testing.T) {
	db := openImageTestDB(t)
	// Migrate LLMModel
	if err := db.AutoMigrate(&llmmodel.LLMModel{}); err != nil {
		t.Fatalf("failed to migrate LLMModel: %v", err)
	}

	svc := gateway.NewBillingService(db, nil, nil, nil)

	// Prepare pricing rules
	rules := []llmmodel.PricingRule{
		{
			ID:       "rule-hd-1024",
			Priority: 10,
			Conditions: map[string]interface{}{
				"size":    "1024x1024",
				"quality": "hd",
			},
			Price: llmmodel.PricingDetail{
				Credits: 100,
			},
		},
		{
			ID:       "rule-1024",
			Priority: 5,
			Conditions: map[string]interface{}{
				"size": "1024x1024",
			},
			Price: llmmodel.PricingDetail{
				Credits: 50,
			},
		},
		{
			ID:         "default",
			Priority:   0,
			Conditions: map[string]interface{}{},
			Price: llmmodel.PricingDetail{
				Credits: 10,
			},
		},
	}
	rulesJSON, _ := json.Marshal(rules)

	// Create model with rules
	modelID := uuid.New()
	model := &llmmodel.LLMModel{
		ID:              modelID,
		Model:           "dall-e-3",
		ImagePrices:     datatypes.JSON(rulesJSON),
		Provider:        "openai",
		UseCases:        llmmodel.StringArray{"image-gen"},
		ImageGeneration: true,
	}
	if err := db.Create(model).Error; err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	// Create model with no rules
	emptyModelID := uuid.New()
	emptyModel := &llmmodel.LLMModel{
		ID:              emptyModelID,
		Model:           "empty-model",
		ImagePrices:     nil,
		Provider:        "openai",
		UseCases:        llmmodel.StringArray{"image-gen"},
		ImageGeneration: true,
	}
	if err := db.Create(emptyModel).Error; err != nil {
		t.Fatalf("failed to create empty model: %v", err)
	}

	// Create model with invalid JSON
	invalidModelID := uuid.New()
	invalidModel := &llmmodel.LLMModel{
		ID:              invalidModelID,
		Model:           "invalid-json",
		ImagePrices:     datatypes.JSON([]byte(`{invalid-json`)),
		Provider:        "openai",
		UseCases:        llmmodel.StringArray{"image-gen"},
		ImageGeneration: true,
	}
	if err := db.Create(invalidModel).Error; err != nil {
		t.Fatalf("failed to create invalid model: %v", err)
	}

	tests := []struct {
		name    string
		modelID uuid.UUID
		req     *adapter.ImageRequest
		want    int64
		wantErr bool
	}{
		{
			name:    "Match exact rule (high priority)",
			modelID: modelID,
			req: &adapter.ImageRequest{
				Size:    "1024x1024",
				Quality: "hd",
				N:       intPtr(1),
			},
			want: 100,
		},
		{
			name:    "Match lower priority rule",
			modelID: modelID,
			req: &adapter.ImageRequest{
				Size:    "1024x1024",
				Quality: "standard",
				N:       intPtr(1),
			},
			want: 50, // Matches rule-1024 (priority 5) because quality mismatch for rule-hd-1024
		},
		{
			name:    "Match fallback rule",
			modelID: modelID,
			req: &adapter.ImageRequest{
				Size: "512x512",
				N:    intPtr(1),
			},
			want: 10, // Matches default
		},
		{
			name:    "Multiple N",
			modelID: modelID,
			req: &adapter.ImageRequest{
				Size:    "1024x1024",
				Quality: "hd",
				N:       intPtr(4),
			},
			want: 400, // 100 * 4
		},
		{
			name:    "No match (empty rules)",
			modelID: emptyModelID,
			req: &adapter.ImageRequest{
				Size: "1024x1024",
			},
			want: 0,
		},
		{
			name:    "JSON parsing error",
			modelID: invalidModelID,
			req: &adapter.ImageRequest{
				Size: "1024x1024",
			},
			want:    0,
			wantErr: true,
		},
		{
			name:    "Default N if 0",
			modelID: modelID,
			req: &adapter.ImageRequest{
				Size: "512x512",
				N:    intPtr(0),
			},
			want: 10, // Default rule 10 * 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.CalculateImageCredits(tt.req, tt.modelID)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateImageCredits() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CalculateImageCredits() = %v, want %v", got, tt.want)
			}
		})
	}
}
