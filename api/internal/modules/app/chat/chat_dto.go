package chat

import (
	"time"
)

// SendMessageRequest represents the request for sending a message
type SendMessageRequest struct {
	Role     string `json:"role" binding:"required" validate:"required"`
	Content  string `json:"content" binding:"required" validate:"required"`
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// GetMessagesRequest represents the request for getting messages
type GetMessagesRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	Limit    int    `form:"limit" binding:"omitempty,min=1,max=100"`
	Role     string `form:"role" binding:"omitempty"`
	Model    string `form:"model" binding:"omitempty"`
	Provider string `form:"provider" binding:"omitempty"`
}

// UpdateMessageRequest represents the request for updating a message
type UpdateMessageRequest struct {
	Content  string `json:"content,omitempty"`
	Role     string `json:"role,omitempty"`
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// AddFeedbackRequest represents the request for adding feedback
type AddFeedbackRequest struct {
	Rating  int    `json:"rating" binding:"required,min=1,max=5" validate:"required,min=1,max=5"`
	Content string `json:"content,omitempty"`
}

// MessageResponse represents the response for a message
type ChatMessageResponse struct {
	ID        string    `json:"id"`
	AppID     string    `json:"app_id"`
	AccountID string    `json:"account_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Model     string    `json:"model"`
	Provider  string    `json:"provider"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MessagePagination represents paginated message response
type MessagePagination struct {
	Page    int                   `json:"page"`
	Limit   int                   `json:"limit"`
	Total   int64                 `json:"total"`
	HasMore bool                  `json:"has_more"`
	Data    []ChatMessageResponse `json:"data"`
}

// MessageFeedbackResponse represents the response for message feedback
type MessageFeedbackResponse struct {
	ID        string    `json:"id"`
	MessageID string    `json:"message_id"`
	Rating    int       `json:"rating"`
	Content   string    `json:"content,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageFilter defines filter options for message queries
type MessageFilter struct {
	AppID     string
	AccountID string
	Role      string
	Model     string
	Provider  string
}
