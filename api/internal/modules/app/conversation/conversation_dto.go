package conversation

import (
	"time"
)

// type PaginationResult struct {
//     Page    int         `json:"page"`
//     Limit   int         `json:"limit"`
//     Total   int64       `json:"total"`
//     HasMore bool        `json:"has_more"`
//     Data    interface{} `json:"data"`
// }

// ConversationDTO basic conversation DTO
type ConversationDTO struct {
	ID                   string                 `json:"id"`
	Status               string                 `json:"status"`
	FromSource           string                 `json:"from_source"`
	FromEndUserID        *string                `json:"from_end_user_id"`
	FromEndUserSessionID *string                `json:"from_end_user_session_id"`
	FromAccountID        *string                `json:"from_account_id"`
	FromAccountName      *string                `json:"from_account_name"`
	ReadAt               *time.Time             `json:"read_at"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
	ModelConfig          map[string]interface{} `json:"model_config"`
	FirstMessage         *SimpleMessageDTO      `json:"message,omitempty"`
}

// ConversationWithSummaryDTO conversation DTO with summary
type ConversationWithSummaryDTO struct {
	ID                   string                 `json:"id"`
	Status               string                 `json:"status"`
	FromSource           string                 `json:"from_source"`
	FromEndUserID        *string                `json:"from_end_user_id"`
	FromEndUserSessionID *string                `json:"from_end_user_session_id"`
	FromAccountID        *string                `json:"from_account_id"`
	FromAccountName      *string                `json:"from_account_name"`
	Name                 string                 `json:"name"`
	Summary              string                 `json:"summary"`
	ReadAt               *time.Time             `json:"read_at"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
	ModelProvider        *string                `json:"model_provider"`
	ModelID              *string                `json:"model_id"`
	ModelConfig          map[string]interface{} `json:"model_config"`
	FirstMessage         *FirstMessageDTO       `json:"first_message,omitempty"`
}

// ConversationDetailDTO conversation detail DTO
type ConversationDetailDTO struct {
	ID            string                 `json:"id"`
	Status        string                 `json:"status"`
	FromSource    string                 `json:"from_source"`
	FromEndUserID *string                `json:"from_end_user_id"`
	FromAccountID *string                `json:"from_account_id"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Introduction  *string                `json:"introduction"`
	ModelConfig   map[string]interface{} `json:"model_config"`
	FirstMessage  *MessageDetailDTO      `json:"message,omitempty"`
}

// SimpleMessageDTO simple message DTO
type SimpleMessageDTO struct {
	Inputs map[string]interface{} `json:"inputs"`
	Query  string                 `json:"query"`
	Answer string                 `json:"answer"`
}

// FirstMessageDTO first message DTO
type FirstMessageDTO struct {
	Inputs  map[string]interface{} `json:"inputs"`
	Query   string                 `json:"query"`
	Message *string                `json:"message"`
	Answer  string                 `json:"answer"`
}

// MessageDetailDTO message detail DTO
type MessageDetailDTO struct {
	ID     string                 `json:"id"`
	Inputs map[string]interface{} `json:"inputs"`
	Query  string                 `json:"query"`
	Answer string                 `json:"answer"`
}

// ConversationGroupDTO conversation group DTO
type ConversationGroupDTO struct {
	GroupID        string                       `json:"group_id"`
	GroupName      string                       `json:"group_name"`
	GroupCreatedAt string                       `json:"group_created_at"`
	GroupUpdatedAt string                       `json:"group_updated_at"`
	Conversations  []ConversationWithSummaryDTO `json:"conversations"`
}

type ConversationListRequest struct {
	Keyword          string    `form:"keyword" json:"keyword"`
	Start            time.Time `form:"start" json:"start" time_format:"2006-01-02 15:04"`
	End              time.Time `form:"end" json:"end" time_format:"2006-01-02 15:04"`
	AnnotationStatus string    `form:"annotation_status" json:"annotation_status"`
	MessageCountGte  int       `form:"message_count_gte" json:"message_count_gte"`
	Page             int       `form:"page" json:"page" validate:"min=1" default:"1"`
	Limit            int       `form:"limit" json:"limit" validate:"min=1,max=100" default:"20"`
	SortBy           string    `form:"sort_by" json:"sort_by" default:"-updated_at"`
}

type ConversationResponse struct {
	ID                 string                 `json:"id"`
	Name               string                 `json:"name"`
	Inputs             map[string]interface{} `json:"inputs"`
	Status             string                 `json:"status"`
	Introduction       string                 `json:"introduction"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	UserID             string                 `json:"user_id,omitempty"`
	FromEndUserID      *string                `json:"from_end_user_id,omitempty"`
	FromAccountID      *string                `json:"from_account_id,omitempty"`
	FromSource         string                 `json:"from_source"`
	ReadAt             *time.Time             `json:"read_at,omitempty"`
	ReadAccountID      *string                `json:"read_account_id,omitempty"`
	Summary            *string                `json:"summary,omitempty"`
	MessageCount       int64                  `json:"message_count,omitempty"`
	UserFeedbackStats  *UserFeedbackStats     `json:"user_feedback_stats,omitempty"`
	AdminFeedbackStats *AdminFeedbackStats    `json:"admin_feedback_stats,omitempty"`
}

type ConversationDetailResponse struct {
	ConversationResponse
	ModelConfig       map[string]interface{}        `json:"model_config,omitempty"`
	Messages          []ConversationMessageResponse `json:"messages,omitempty"`
	AppModelConfig    map[string]interface{}        `json:"app_model_config,omitempty"`
	SystemInstruction *string                       `json:"system_instruction,omitempty"`
}

type ConversationPaginationResponse struct {
	Data    []ConversationResponse `json:"data"`
	HasMore bool                   `json:"has_more"`
	Limit   int                    `json:"limit"`
	Total   int64                  `json:"total"`
	Page    int                    `json:"page"`
}

type ConversationWithSummaryPaginationResponse struct {
	Data    []ConversationWithSummaryResponse `json:"data"`
	HasMore bool                              `json:"has_more"`
	Limit   int                               `json:"limit"`
	Total   int64                             `json:"total"`
	Page    int                               `json:"page"`
}

type ConversationWithSummaryResponse struct {
	ConversationResponse
	Summary           *string            `json:"summary,omitempty"`
	MessageCount      int64              `json:"message_count"`
	UserFeedbackStats *UserFeedbackStats `json:"user_feedback_stats,omitempty"`
	EndUser           *EndUserInfo       `json:"end_user,omitempty"`
}

// ConversationMessageResponse represents comprehensive message response for conversations
type ConversationMessageResponse struct {
	ID             string                 `json:"id"`
	ConversationID string                 `json:"conversation_id"`
	Inputs         map[string]interface{} `json:"inputs"`
	Query          string                 `json:"query"`
	Answer         string                 `json:"answer"`
	MessageTokens  int                    `json:"message_tokens"`
	AnswerTokens   int                    `json:"answer_tokens"`
	TotalPrice     *float64               `json:"total_price,omitempty"`
	Currency       string                 `json:"currency"`
	FromSource     string                 `json:"from_source"`
	FromAccountID  *string                `json:"from_account_id,omitempty"`
	FromEndUserID  *string                `json:"from_end_user_id,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	AgentBased     bool                   `json:"agent_based"`
	WorkflowRunID  *string                `json:"workflow_run_id,omitempty"`
	Status         string                 `json:"status"`
	Error          *string                `json:"error,omitempty"`
	Feedbacks      []MessageFeedback      `json:"feedbacks,omitempty"`
	Annotation     *MessageAnnotation     `json:"annotation,omitempty"`
}

type UserFeedbackStats struct {
	Like    int64 `json:"like"`
	Dislike int64 `json:"dislike"`
}

type AdminFeedbackStats struct {
	Total int64 `json:"total"`
}

type EndUserInfo struct {
	ID          string  `json:"id"`
	SessionID   string  `json:"session_id"`
	IsAnonymous bool    `json:"is_anonymous"`
	Name        *string `json:"name,omitempty"`
}

type MessageFeedback struct {
	ID            string    `json:"id"`
	Rating        string    `json:"rating"`
	Content       *string   `json:"content,omitempty"`
	FromSource    string    `json:"from_source"`
	FromEndUserID *string   `json:"from_end_user_id,omitempty"`
	FromAccountID *string   `json:"from_account_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type MessageAnnotation struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Question  *string   `json:"question,omitempty"`
	AccountID string    `json:"account_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	HitCount  int       `json:"hit_count"`
}

type DeleteConversationResponse struct {
	Result string `json:"result"`
}

type ConversationFilters struct {
	AppID            string
	Keyword          string
	Start            *time.Time
	End              *time.Time
	AnnotationStatus string
	MessageCountGte  int
	FromSource       string
	FromEndUserID    *string
	FromAccountID    *string
	SortBy           string
	Page             int
	Limit            int
}

type ConversationPaginationResult struct {
	Conversations []ConversationWithRelations
	Total         int64
	Page          int
	Limit         int
	HasMore       bool
}

type ConversationWithRelations struct {
	ID                 string
	AppID              string
	Name               string
	Inputs             string // JSON string
	Status             string
	Introduction       *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	FromEndUserID      *string
	FromAccountID      *string
	FromSource         string
	ReadAt             *time.Time
	ReadAccountID      *string
	Summary            *string
	MessageCount       int64
	UserFeedbackStats  *UserFeedbackStats
	AdminFeedbackStats *AdminFeedbackStats
	EndUserSessionID   *string
	EndUserName        *string
	EndUserIsAnonymous *bool
}

// ConversationCreatedFrom constants
const (
	ConversationCreatedFromAPI     = "api"
	ConversationCreatedFromConsole = "console"
)

// ConversationStatus constants
const (
	ConversationStatusNormal = "normal"
	ConversationStatusPinned = "pinned"
)

// AnnotationStatus constants
const (
	AnnotationStatusAll          = "all"
	AnnotationStatusAnnotated    = "not_annotated"
	AnnotationStatusNotAnnotated = "annotated"
)

// SortBy constants
const (
	SortByCreatedAtAsc  = "created_at"
	SortByCreatedAtDesc = "-created_at"
	SortByUpdatedAtAsc  = "updated_at"
	SortByUpdatedAtDesc = "-updated_at"
)

// ConversationGroupQuery conversation group query parameters
type ConversationGroupQuery struct {
	Page  int `form:"page" binding:"omitempty,min=1"`
	Limit int `form:"limit" binding:"omitempty,min=1,max=100"`
}

// CreateConversationGroupRequest create conversation group request
type CreateConversationGroupRequest struct {
	GroupID         *string  `json:"group_id"`
	GroupName       *string  `json:"group_name"`
	ConversationIDs []string `json:"conversation_ids" binding:"required"`
}

// UpdateConversationGroupRequest update conversation group request
type UpdateConversationGroupRequest struct {
	GroupID        string  `json:"group_id" binding:"required"`
	GroupName      *string `json:"group_name"`
	ConversationID string  `json:"conversation_id" binding:"required"`
}

// DeleteConversationFromGroupRequest delete conversation from group request
type DeleteConversationFromGroupRequest struct {
	GroupID        string `json:"group_id" binding:"required"`
	ConversationID string `json:"conversation_id" binding:"required"`
}

// UpdateConversationRequest update conversation request
type UpdateConversationRequest struct {
	ConversationID string `json:"conversation_id" binding:"required"`
	ModelID        string `json:"model_id" binding:"required"`
	ModelProvider  string `json:"model_provider" binding:"required"`
}

// DeleteConversationRequest delete conversation request
type DeleteConversationRequest struct {
	ConversationID string `json:"conversation_id" binding:"required"`
}

// SuccessResponse success response
type SuccessResponse struct {
	Result string `json:"result"`
}

// ConversationGroupSuccessResponse conversation group success response
type ConversationGroupSuccessResponse struct {
	Message string `json:"message"`
	GroupID string `json:"group_id"`
}
