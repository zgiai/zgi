package workflow

type AgentConversationListRequest struct {
	Page             int    `form:"page"`
	Limit            int    `form:"limit"`
	Keyword          string `form:"keyword"`
	AnnotationStatus string `form:"annotation_status"`
	SortBy           string `form:"sort_by"`
	InvokeFrom       []string
	Start            string `form:"start"`
	End              string `form:"end"`
}

type AgentConversationListItem struct {
	ID                 string                 `json:"id"`
	Status             string                 `json:"status"`
	FromSource         string                 `json:"from_source"`
	InvokeFrom         *string                `json:"invoke_from"`
	FromEndUserID      *string                `json:"from_end_user_id"`
	FromAccountID      *string                `json:"from_account_id"`
	FromAccountName    *string                `json:"from_account_name"`
	Name               string                 `json:"name"`
	Summary            *string                `json:"summary"`
	ReadAt             *int64                 `json:"read_at"`
	CreatedAt          int64                  `json:"created_at"`
	UpdatedAt          int64                  `json:"updated_at"`
	Annotated          bool                   `json:"annotated"`
	ModelConfig        map[string]interface{} `json:"model_config"`
	MessageCount       int                    `json:"message_count"`
	UserFeedbackStats  map[string]int64       `json:"user_feedback_stats"`
	AdminFeedbackStats map[string]int64       `json:"admin_feedback_stats"`
}

type AgentConversationListResponse struct {
	Page    int                         `json:"page"`
	Limit   int                         `json:"limit"`
	Total   int64                       `json:"total"`
	HasMore bool                        `json:"has_more"`
	Data    []AgentConversationListItem `json:"data"`
}

type AgentConversationDetailResponse struct {
	ID                 string                 `json:"id"`
	Status             string                 `json:"status"`
	FromSource         string                 `json:"from_source"`
	InvokeFrom         *string                `json:"invoke_from"`
	FromEndUserID      *string                `json:"from_end_user_id"`
	FromAccountID      *string                `json:"from_account_id"`
	FromAccountName    *string                `json:"from_account_name"`
	Name               string                 `json:"name"`
	Summary            *string                `json:"summary"`
	ReadAt             *int64                 `json:"read_at"`
	CreatedAt          int64                  `json:"created_at"`
	UpdatedAt          int64                  `json:"updated_at"`
	Annotated          bool                   `json:"annotated"`
	Introduction       *string                `json:"introduction"`
	ModelConfig        map[string]interface{} `json:"model_config"`
	MessageCount       int                    `json:"message_count"`
	UserFeedbackStats  map[string]int64       `json:"user_feedback_stats"`
	AdminFeedbackStats map[string]int64       `json:"admin_feedback_stats"`
}
