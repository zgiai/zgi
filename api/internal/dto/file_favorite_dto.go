package dto

import "time"

// FileFavoriteRequest represents request for file favorite operations
type FileFavoriteRequest struct {
	FileID string `json:"file_id" binding:"required"`
}

// BatchFileFavoriteRequest represents request for batch file favorite operations
type BatchFileFavoriteRequest struct {
	FileIDs []string `json:"file_ids" binding:"required"`
}

// FileFavoriteResponse represents response for file favorite operations
type FileFavoriteResponse struct {
	ID        string    `json:"id"`
	FileID    string    `json:"file_id"`
	AccountID string    `json:"account_id"`
	CreatedAt time.Time `json:"created_at"`
}

// FileFavoriteListRequest represents request for listing file favorites
type FileFavoriteListRequest struct {
	Page  int `form:"page" binding:"omitempty,min=1"`
	Limit int `form:"limit" binding:"omitempty,min=1,max=100"`
}

// FileFavoriteListResponse represents response for file favorite list
type FileFavoriteListResponse struct {
	Data    []FileFavoriteResponse `json:"data"`
	HasMore bool                   `json:"has_more"`
	Limit   int                    `json:"limit"`
	Total   int64                  `json:"total"`
	Page    int                    `json:"page"`
}