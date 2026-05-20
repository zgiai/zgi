package service

import (
	"github.com/zgiai/zgi/api/internal/dto"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	helper "github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

// ResponseType Response type enumeration
type ResponseType int

const (
	ResponseTypeSuccess ResponseType = iota
	ResponseTypeBusinessError
	ResponseTypeSpecialFail
)

// LoginResult Unified login result
type LoginResult struct {
	Type        ResponseType                `json:"type"`
	Success     bool                        `json:"success"`
	TokenPair   *auth_model.TokenPair       `json:"token_pair,omitempty"`
	Account     *dto.AccountProfileResponse `json:"account,omitempty"`
	Error       *LoginError                 `json:"error,omitempty"`
	SpecialData interface{}                 `json:"special_data,omitempty"`
	SpecialCode string                      `json:"special_code,omitempty"`
}

// LoginError Login error information
type LoginError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	EnMessage  string `json:"en_message"`
	HTTPStatus int    `json:"http_status"`
}

// NewSuccessResult Create success result
func NewSuccessResult(tokenPair *auth_model.TokenPair, account *dto.AccountProfileResponse) *LoginResult {
	return &LoginResult{
		Type:      ResponseTypeSuccess,
		Success:   true,
		TokenPair: tokenPair,
		Account:   account,
	}
}

// NewBusinessErrorResult Create business error result
func NewBusinessErrorResult(errorResp helper.ErrorResponse) *LoginResult {
	return &LoginResult{
		Type:    ResponseTypeBusinessError,
		Success: false,
		Error: &LoginError{
			Code:       errorResp.ErrorCode,
			Message:    errorResp.Description,
			EnMessage:  errorResp.EnDescription,
			HTTPStatus: errorResp.Code,
		},
	}
}

// NewSpecialFailResult Create special fail result
func NewSpecialFailResult(data interface{}, code string) *LoginResult {
	return &LoginResult{
		Type:        ResponseTypeSpecialFail,
		Success:     false,
		SpecialData: data,
		SpecialCode: code,
	}
}

// IsSuccess Determine if successful
func (r *LoginResult) IsSuccess() bool {
	return r.Success
}

// GetTokenPair Get token pair
func (r *LoginResult) GetTokenPair() *auth_model.TokenPair {
	if r.Success {
		return r.TokenPair
	}
	return nil
}

// GetError Get error information
func (r *LoginResult) GetError() *LoginError {
	if !r.Success && r.Type == ResponseTypeBusinessError {
		return r.Error
	}
	return nil
}

// GetResponseType Get response type
func (r *LoginResult) GetResponseType() ResponseType {
	return r.Type
}

// GetSpecialData Get special data
func (r *LoginResult) GetSpecialData() interface{} {
	if r.Type == ResponseTypeSpecialFail {
		return r.SpecialData
	}
	return nil
}

// GetSpecialCode Get special code
func (r *LoginResult) GetSpecialCode() string {
	if r.Type == ResponseTypeSpecialFail {
		return r.SpecialCode
	}
	return ""
}

// GetStandardErrorCode maps helper.ErrorResponse to standard ErrorCode
func GetStandardErrorCode(errorResp helper.ErrorResponse) response.ErrorCode {
	switch errorResp.ErrorCode {
	case "account_banned":
		return response.ErrAccountBanned
	case "account_in_freeze":
		return response.ErrAccountFrozen
	case "email_or_password_mismatch":
		return response.ErrEmailPasswordMismatch
	case "email_code_login_limit":
		return response.ErrLoginErrorRateLimit
	case "invalid_email":
		return response.ErrEmailInvalid
	case "account_not_found":
		return response.ErrAccountNotFound
	case "unknown_error":
		return response.ErrSystemError
	case "invalid_or_expired_token":
		return response.ErrTokenInvalid
	case "email_code_error":
		return response.ErrInvalidCode
	case "not_allowed_create_workspace":
		return response.ErrWorkspaceJoinedNotFound
	case "email_send_ip_limit":
		return response.ErrRateLimitExceeded
	case "email_send_failed":
		return response.ErrEmailSendFailed
	default:
		return response.ErrSystemError
	}
}
