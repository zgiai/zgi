package model

// AppInfo represents basic app information
type AppInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Mode           string `json:"mode"`
	Icon           string `json:"icon"`
	IconType       string `json:"icon_type,omitempty"`
	IconURL        string `json:"icon_url,omitempty"`
	IconBackground string `json:"icon_background"`
}

// RecommendedApp represents a recommended app with its metadata
type RecommendedApp struct {
	App             AppInfo `json:"app"`
	AppID           string  `json:"app_id"`
	Description     *string `json:"description"`
	Copyright       *string `json:"copyright"`
	PrivacyPolicy   *string `json:"privacy_policy"`
	CustomDisclaimer *string `json:"custom_disclaimer,omitempty"`
	Category        string  `json:"category"`
	Position        int     `json:"position"`
	IsListed        bool    `json:"is_listed"`
}

// RecommendedAppListResponse represents the response for listing recommended apps
type RecommendedAppListResponse struct {
	RecommendedApps []RecommendedApp `json:"recommended_apps"`
	Categories      []string         `json:"categories"`
}

// RecommendedAppsData represents the structure of recommended_apps.json
type RecommendedAppsData struct {
	RecommendedApps map[string]RecommendedAppListResponse `json:"recommended_apps"`
	AppDetails      map[string]interface{}                `json:"app_details,omitempty"`
}
