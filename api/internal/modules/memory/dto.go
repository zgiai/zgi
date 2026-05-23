package memory

type MemoryEntryResponse struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	Category   string `json:"category"`
	MemoryType string `json:"memory_type"`
	ExpiresAt  *int64 `json:"expires_at,omitempty"`
	Status     string `json:"status"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

type MemoryMeResponse struct {
	Enabled   bool                  `json:"enabled"`
	Entries   []MemoryEntryResponse `json:"entries"`
	UpdatedAt int64                 `json:"updated_at"`
}

type UpdateSettingRequest struct {
	Enabled bool `json:"enabled"`
}

type CreateEntryRequest struct {
	Content    string `json:"content" binding:"required"`
	Category   string `json:"category,omitempty"`
	MemoryType string `json:"memory_type,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

type UpdateEntryRequest struct {
	Content    *string `json:"content,omitempty"`
	Category   *string `json:"category,omitempty"`
	MemoryType *string `json:"memory_type,omitempty"`
	ExpiresAt  *string `json:"expires_at,omitempty"`
	Enabled    *bool   `json:"enabled,omitempty"`
}
