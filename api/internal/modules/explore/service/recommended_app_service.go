package service

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/zgiai/ginext/internal/modules/explore/model"
)

//go:embed data/recommended_apps.json
var embeddedData embed.FS

// RecommendedAppService handles recommended app business logic
type RecommendedAppService interface {
	GetRecommendedAppsAndCategories(language string) (*model.RecommendedAppListResponse, error)
	GetRecommendAppDetail(appID string) (interface{}, error)
}

type recommendedAppServiceImpl struct {
	builtinData     *model.RecommendedAppsData
	builtinDataOnce sync.Once
}

// NewRecommendedAppService creates a new recommended app service instance
func NewRecommendedAppService() RecommendedAppService {
	return &recommendedAppServiceImpl{}
}

// getBuiltinData loads and caches the builtin recommended apps data from embedded file
func (s *recommendedAppServiceImpl) getBuiltinData() (*model.RecommendedAppsData, error) {
	var loadErr error

	s.builtinDataOnce.Do(func() {
		// Read from embedded file system
		fileData, err := embeddedData.ReadFile("data/recommended_apps.json")
		if err != nil {
			loadErr = fmt.Errorf("failed to read embedded recommended_apps.json: %w", err)
			return
		}

		var data model.RecommendedAppsData
		if err := json.Unmarshal(fileData, &data); err != nil {
			loadErr = fmt.Errorf("failed to parse recommended_apps.json: %w", err)
			return
		}

		s.builtinData = &data
	})

	if loadErr != nil {
		return nil, loadErr
	}

	if s.builtinData == nil {
		return nil, fmt.Errorf("builtin data not loaded")
	}

	return s.builtinData, nil
}

// GetRecommendedAppsAndCategories retrieves recommended apps and categories for a given language
func (s *recommendedAppServiceImpl) GetRecommendedAppsAndCategories(language string) (*model.RecommendedAppListResponse, error) {
	data, err := s.getBuiltinData()
	if err != nil {
		return nil, err
	}

	var response *model.RecommendedAppListResponse

	// Try to get data for the requested language
	if resp, exists := data.RecommendedApps[language]; exists {
		response = &resp
	} else if language != "en-US" {
		// Fallback to en-US if requested language not found
		if resp, exists := data.RecommendedApps["en-US"]; exists {
			response = &resp
		}
	}

	// Return empty response if no data found
	if response == nil {
		return &model.RecommendedAppListResponse{
			RecommendedApps: []model.RecommendedApp{},
			Categories:      []string{},
		}, nil
	}

	// TODO: Remove this filter when AGENT apps are ready for production
	// Filter out AGENT-related app modes temporarily
	filteredApps := filterOutAgentApps(response.RecommendedApps)

	return &model.RecommendedAppListResponse{
		RecommendedApps: filteredApps,
		Categories:      response.Categories,
	}, nil
}

// filterOutAgentApps filters out apps with AGENT-related modes
// TODO: Remove this function when AGENT apps are ready for production
func filterOutAgentApps(apps []model.RecommendedApp) []model.RecommendedApp {
	filtered := make([]model.RecommendedApp, 0, len(apps))
	for _, app := range apps {
		mode := strings.ToUpper(app.App.Mode)
		// Skip AGENT and CONVERSATIONAL_AGENT modes
		if mode == "AGENT" || mode == "CONVERSATIONAL_AGENT" {
			continue
		}
		filtered = append(filtered, app)
	}
	return filtered
}

// GetRecommendAppDetail retrieves detailed information for a specific app
func (s *recommendedAppServiceImpl) GetRecommendAppDetail(appID string) (interface{}, error) {
	data, err := s.getBuiltinData()
	if err != nil {
		return nil, err
	}

	if data.AppDetails == nil {
		return nil, fmt.Errorf("app detail not found for app_id: %s", appID)
	}

	if detail, exists := data.AppDetails[appID]; exists {
		return detail, nil
	}

	return nil, fmt.Errorf("app detail not found for app_id: %s", appID)
}
