package dto

type InvocationContentSettings struct {
	// Available is retained as true for compatibility with older web clients
	// during rolling upgrades. Organization Enabled is the only feature switch.
	Available     bool `json:"available"`
	Enabled       bool `json:"enabled"`
	MaxBytes      int  `json:"max_bytes"`
	RetentionDays int  `json:"retention_days"`
}

type UpdateInvocationContentSettingsRequest struct {
	Enabled bool `json:"enabled"`
}

type InvocationContentDetail struct {
	InvocationID     string `json:"invocation_id"`
	InputText        string `json:"input_text"`
	OutputText       string `json:"output_text"`
	InputJSON        string `json:"input_json"`
	OutputJSON       string `json:"output_json"`
	ContentStatus    string `json:"content_status"`
	InputTruncated   bool   `json:"input_truncated"`
	OutputTruncated  bool   `json:"output_truncated"`
	RedactionVersion string `json:"redaction_version"`
	ExpiresAt        int64  `json:"expires_at"`
}
