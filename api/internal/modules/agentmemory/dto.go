package agentmemory

type SlotResponse struct {
	ID               string `json:"id"`
	Key              string `json:"key"`
	Description      string `json:"description"`
	MaxChars         int    `json:"max_chars"`
	Enabled          bool   `json:"enabled"`
	SortOrder        int    `json:"sort_order"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
	CreatedAtUnix    int64  `json:"created_at_unix"`
	UpdatedAtUnix    int64  `json:"updated_at_unix"`
	CreatedAtISO     string `json:"created_at_iso"`
	UpdatedAtISO     string `json:"updated_at_iso"`
	CreatedAtDisplay string `json:"created_at_display"`
	UpdatedAtDisplay string `json:"updated_at_display"`
}

type SlotValueResponse struct {
	SlotResponse
	Content string `json:"content"`
}

type ReplaceSlotsRequest struct {
	Slots []SlotUpsertRequest `json:"slots" binding:"required"`
}

type SlotUpsertRequest struct {
	ID          string `json:"id,omitempty"`
	Key         string `json:"key" binding:"required"`
	Description string `json:"description,omitempty"`
	MaxChars    int    `json:"max_chars,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
	SortOrder   int    `json:"sort_order,omitempty"`
}

type UpdateValueRequest struct {
	Key     string `json:"key" binding:"required"`
	Content string `json:"content" binding:"required"`
}
