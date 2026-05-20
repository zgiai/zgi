package dto

// GenerateSuggestedQuestionsRequest represents a request to generate editable
// suggested questions for a workflow draft or web app entry point.
type GenerateSuggestedQuestionsRequest struct {
	Locale            string                 `json:"locale,omitempty"`
	Count             int                    `json:"count,omitempty" validate:"omitempty,min=1,max=6"`
	Provider          string                 `json:"provider,omitempty"`
	Model             string                 `json:"model,omitempty"`
	Graph             map[string]interface{} `json:"graph,omitempty"`
	Features          map[string]interface{} `json:"features,omitempty"`
	ExistingQuestions []string               `json:"existing_questions,omitempty"`
}

// SuggestedQuestionCandidate is one model-generated question candidate.
type SuggestedQuestionCandidate struct {
	Text   string `json:"text"`
	Reason string `json:"reason,omitempty"`
}

// GenerateSuggestedQuestionsResponse contains generated suggested questions
// plus non-blocking warnings for the editor UI.
type GenerateSuggestedQuestionsResponse struct {
	Questions []SuggestedQuestionCandidate `json:"questions"`
	Warnings  []string                     `json:"warnings,omitempty"`
	Provider  string                       `json:"provider,omitempty"`
	Model     string                       `json:"model,omitempty"`
}
