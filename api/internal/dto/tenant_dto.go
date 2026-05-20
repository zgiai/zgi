package dto

import "time"

// TenantInfo represents comprehensive tenant information
type TenantInfo struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Plan        string                 `json:"plan"`
	Status      string                 `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CustomAttrs map[string]interface{} `json:"custom_attrs,omitempty"`
}

// SimpleWorkspaceInfo represents simplified tenant information for DTOs
type SimpleWorkspaceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Plan string `json:"plan"`
}
