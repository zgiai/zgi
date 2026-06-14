package service

import "errors"

var (
	ErrInvalidInput         = errors.New("invalid action runtime input")
	ErrNotFound             = errors.New("action run not found")
	ErrPermissionDenied     = errors.New("action runtime permission denied")
	ErrConfirmationRequired = errors.New("action confirmation required")
)
