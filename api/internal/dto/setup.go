package dto

import "time"

type SetupResponse struct {
	Step    string     `json:"step"`
	SetupAt *time.Time `json:"setup_at,omitempty"`
}

type SetupRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Name     string `json:"name" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type SetupResult struct {
	Result string `json:"result"`
}
