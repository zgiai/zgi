package model

import "time"

// SetupResponse represents the response for setup status
type SetupResponse struct {
	Step    string     `json:"step"`
	SetupAt *time.Time `json:"setup_at,omitempty"`
}

// SetupRequest represents the request for system setup
type SetupRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Name     string `json:"name" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// SetupResult represents the result of system setup
type SetupResult struct {
	Result string `json:"result"`
}
