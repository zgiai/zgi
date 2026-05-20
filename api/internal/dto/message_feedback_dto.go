package dto

import "time"

// MessageFeedbackRequest represents request for message feedback
type MessageFeedbackRequest struct {
	MessageID string  `json:"message_id" binding:"required" validate:"uuid4"`
	Rating    *string `json:"rating" validate:"omitempty,oneof=like dislike"`
}

// MessageFeedbackResultResponse represents result of message feedback operation
type MessageFeedbackResultResponse struct {
	Result string `json:"result"`
}

// MessageAnnotationRequest represents request for message annotation
type MessageAnnotationRequest struct {
	MessageID       *string                `json:"message_id" validate:"omitempty,uuid4"`
	Question        string                 `json:"question" binding:"required"`
	Answer          string                 `json:"answer" binding:"required"`
	AnnotationReply map[string]interface{} `json:"annotation_reply"`
}

// MessageAnnotationDetailResponse represents detailed message annotation information
type MessageAnnotationDetailResponse struct {
	ID        string    `json:"id"`
	AppID     string    `json:"app_id"`
	Question  *string   `json:"question"`
	Content   string    `json:"content"`
	AccountID string    `json:"account_id"`
	HitCount  int       `json:"hit_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
