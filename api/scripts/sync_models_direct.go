package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Model struct {
	ID                       string          `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	Provider                 string          `gorm:"type:varchar(100);not null"`
	Name                     string          `gorm:"type:varchar(100);not null"`
	DisplayName              string          `gorm:"type:varchar(200);not null"`
	Tagline                  string          `gorm:"type:text"`
	Description              string          `gorm:"type:text"`
	Family                   string          `gorm:"type:varchar(100)"`
	FamilyName               string          `gorm:"type:varchar(200)"`
	Status                   string          `gorm:"type:varchar(20);default:'active'"`
	AccessType               string          `gorm:"type:varchar(20);default:'closed'"`
	Type                     string          `gorm:"type:varchar(50);not null;default:'llm'"`
	ContextWindow            int             `gorm:"column:context_window"`
	MaxInputTokens           int             `gorm:"column:max_input_tokens"`
	MaxOutputTokens          int             `gorm:"column:max_output_tokens"`
	InputPrice               decimal.Decimal `gorm:"column:input_price;type:decimal(10,4)"`
	OutputPrice              decimal.Decimal `gorm:"column:output_price;type:decimal(10,4)"`
	CachedInputPrice         decimal.Decimal `gorm:"column:cached_input_price;type:decimal(10,4)"`
	IsFlagship               bool            `gorm:"default:false"`
	IsFeatured               bool            `gorm:"default:false"`
	IsNew                    bool            `gorm:"default:false"`
	IsRecommended            bool            `gorm:"default:false"`
	SupportsVision           bool            `gorm:"column:vision;default:false"`
	SupportsAudio            bool            `gorm:"column:audio;default:false"`
	SupportsReasoning        bool            `gorm:"column:reasoning;default:false"`
	SupportsToolCall         bool            `gorm:"column:function_calling;default:false"`
	SupportsStreaming        bool            `gorm:"column:streaming;default:true"`
	SupportsJsonMode         bool            `gorm:"column:json_mode;default:false"`
	SupportsStructuredOutput bool            `gorm:"column:structured_output;default:false"`
	IsActive                 bool            `gorm:"default:true"`
	Currency                 string          `gorm:"type:varchar(10);default:'USD'"`
	CreatedAt                time.Time       `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt                time.Time       `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

func (Model) TableName() string {
	return "llm_models"
}

type ModelMetaModel struct {
	Provider                 string  `json:"provider"`
	Model                    string  `json:"model"`
	ModelName                string  `json:"model_name"`
	Tagline                  string  `json:"tagline"`
	Description              string  `json:"description"`
	Family                   string  `json:"family"`
	FamilyName               string  `json:"family_name"`
	Status                   string  `json:"status"`
	AccessType               string  `json:"access_type"`
	ContextWindow            int     `json:"context_window"`
	MaxInputTokens           int     `json:"max_input_tokens"`
	MaxOutputTokens          int     `json:"max_output_tokens"`
	InputPricePerMillion     float64 `json:"input_price_per_million"`
	OutputPricePerMillion    float64 `json:"output_price_per_million"`
	CachedInputPrice         float64 `json:"cached_input_price"`
	IsFlagship               bool    `json:"is_flagship"`
	IsRecommended            bool    `json:"is_recommended"`
	IsFeatured               bool    `json:"is_featured"`
	IsNew                    bool    `json:"is_new"`
	HasVision                bool    `json:"has_vision"`
	HasAudio                 bool    `json:"has_audio"`
	HasReasoning             bool    `json:"has_reasoning"`
	SupportsFunctionCall     bool    `json:"supports_function_call"`
	SupportsStreaming        bool    `json:"supports_streaming"`
	SupportsJsonMode         bool    `json:"supports_json_mode"`
	SupportsStructuredOutput bool    `json:"supports_structured_output"`
}

type ModelMetaResponse struct {
	Data       []ModelMetaModel `json:"data"`
	Total      int              `json:"total"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	dbCfg := cfg.Database
	dsn := dbCfg.URL
	if dsn == "" {
		dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
			dbCfg.Host, dbCfg.Port, dbCfg.Username, dbCfg.Password, dbCfg.DBName, dbCfg.SSLMode, dbCfg.Timezone)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	log.Println("🚀 Starting model sync from ModelMeta API...")

	modelMetaModelsURL := modelMetaEndpoint(cfg.ModelMeta.APIURL, "/models")
	created, updated := 0, 0
	page := 1

	for {
		url := fmt.Sprintf("%s?page=%d&page_size=100", modelMetaModelsURL, page)
		log.Printf("📡 Fetching page %d...", page)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		resp, err := http.DefaultClient.Do(req)

		if err != nil {
			cancel()
			log.Fatalf("Failed to fetch: %v", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()

		if err != nil {
			log.Fatalf("Failed to read body: %v", err)
		}

		if len(body) == 0 {
			log.Fatalf("Empty response body")
		}

		var result ModelMetaResponse
		if err := json.Unmarshal(body, &result); err != nil {
			log.Printf("Response body (first 500 chars): %s", string(body[:min(500, len(body))]))
			log.Fatalf("Failed to parse JSON: %v", err)
		}

		log.Printf("📦 Processing %d models...", len(result.Data))

		for _, m := range result.Data {
			if m.Provider == "" || m.Model == "" {
				continue
			}

			var existing Model
			err := db.Where("provider = ? AND name = ?", m.Provider, m.Model).First(&existing).Error

			model := Model{
				Provider:                 m.Provider,
				Name:                     m.Model,
				DisplayName:              m.ModelName,
				Tagline:                  m.Tagline,
				Description:              m.Description,
				Family:                   m.Family,
				FamilyName:               m.FamilyName,
				Status:                   m.Status,
				AccessType:               m.AccessType,
				Type:                     "llm",
				ContextWindow:            m.ContextWindow,
				MaxInputTokens:           m.MaxInputTokens,
				MaxOutputTokens:          m.MaxOutputTokens,
				InputPrice:               decimal.NewFromFloat(m.InputPricePerMillion),
				OutputPrice:              decimal.NewFromFloat(m.OutputPricePerMillion),
				CachedInputPrice:         decimal.NewFromFloat(m.CachedInputPrice),
				IsFlagship:               m.IsFlagship,
				IsFeatured:               m.IsFeatured,
				IsNew:                    m.IsNew,
				IsRecommended:            m.IsRecommended,
				SupportsVision:           m.HasVision,
				SupportsAudio:            m.HasAudio,
				SupportsReasoning:        m.HasReasoning,
				SupportsToolCall:         m.SupportsFunctionCall,
				SupportsStreaming:        m.SupportsStreaming,
				SupportsJsonMode:         m.SupportsJsonMode,
				SupportsStructuredOutput: m.SupportsStructuredOutput,
				IsActive:                 true,
				Currency:                 "USD",
			}

			if err == gorm.ErrRecordNotFound {
				if err := db.Create(&model).Error; err != nil {
					log.Printf("  ⚠️  Failed to create %s: %v", m.Model, err)
				} else {
					created++
					if created%10 == 0 {
						log.Printf("  ✅ Created %d models so far...", created)
					}
				}
			} else {
				if err := db.Model(&existing).Updates(map[string]interface{}{
					"display_name":       model.DisplayName,
					"tagline":            model.Tagline,
					"description":        model.Description,
					"family":             model.Family,
					"family_name":        model.FamilyName,
					"status":             model.Status,
					"context_window":     model.ContextWindow,
					"max_input_tokens":   model.MaxInputTokens,
					"max_output_tokens":  model.MaxOutputTokens,
					"input_price":        model.InputPrice,
					"output_price":       model.OutputPrice,
					"cached_input_price": model.CachedInputPrice,
					"is_flagship":        model.IsFlagship,
					"is_featured":        model.IsFeatured,
					"is_new":             model.IsNew,
					"is_recommended":     model.IsRecommended,
					"vision":             model.SupportsVision,
					"audio":              model.SupportsAudio,
					"reasoning":          model.SupportsReasoning,
					"function_calling":   model.SupportsToolCall,
					"streaming":          model.SupportsStreaming,
					"json_mode":          model.SupportsJsonMode,
					"structured_output":  model.SupportsStructuredOutput,
					"updated_at":         time.Now(),
				}).Error; err != nil {
					log.Printf("  ⚠️  Failed to update %s: %v", m.Model, err)
				} else {
					updated++
				}
			}
		}

		if page >= result.TotalPages {
			break
		}
		page++
		time.Sleep(500 * time.Millisecond)
	}

	log.Printf("\n✅ Sync completed!")
	log.Printf("   Created: %d", created)
	log.Printf("   Updated: %d", updated)
	log.Printf("   Total: %d", created+updated)
}

func modelMetaEndpoint(apiURL, path string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if baseURL == "" {
		baseURL = "https://models.zgi.ai"
	}
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}
	return baseURL + path
}
