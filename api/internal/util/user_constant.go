package util

type ErrorResponse struct {
	ErrorCode     string `json:"error_code"`
	Description   string `json:"description"`
	EnDescription string `json:"en_description"`
	Code          int    `json:"code"`
}

// Error constants
var (
	AccountBannedError = ErrorResponse{
		ErrorCode:     "account_banned",
		Description:   "Account has been disabled.",
		EnDescription: "Account is banned.",
		Code:          400,
	}

	AccountInFreezeError = ErrorResponse{
		ErrorCode:     "account_in_freeze",
		Description:   "Account has been frozen.",
		EnDescription: "Account is frozen.",
		Code:          400,
	}

	AccountNotFoundError = ErrorResponse{
		ErrorCode:     "account_not_found",
		Description:   "Account not found.",
		EnDescription: "Account not found.",
		Code:          400,
	}

	EmailOrPasswordMismatchError = ErrorResponse{
		ErrorCode:     "email_or_password_mismatch",
		Description:   "Email or password does not match.",
		EnDescription: "The email or password is mismatched.",
		Code:          400,
	}

	EmailPasswordLoginLimitError = ErrorResponse{
		ErrorCode:     "email_code_login_limit",
		Description:   "Too many password error attempts. Please try again later.",
		EnDescription: "Too many incorrect password attempts. Please try again later.",
		Code:          429,
	}

	InvalidEmailError = ErrorResponse{
		ErrorCode:     "invalid_email",
		Description:   "Invalid email address.",
		EnDescription: "The email address is not valid.",
		Code:          400,
	}

	InvalidTokenError = ErrorResponse{
		ErrorCode:     "invalid_or_expired_token",
		Description:   "Token is invalid or has expired.",
		EnDescription: "The token is invalid or has expired.",
		Code:          400,
	}

	EmailCodeError = ErrorResponse{
		ErrorCode:     "email_code_error",
		Description:   "Email verification code is invalid or has expired.",
		EnDescription: "Email code is invalid or expired.",
		Code:          400,
	}

	NotAllowedCreateWorkspaceError = ErrorResponse{
		ErrorCode:     "not_allowed_create_workspace",
		Description:   "Workspace not found, please contact system administrator to invite you to join workspace.",
		EnDescription: "Workspace not found, please contact system admin to invite you to join in a workspace.",
		Code:          400,
	}

	EmailSendIpLimitError = ErrorResponse{
		ErrorCode:     "email_send_ip_limit",
		Description:   "Sending emails too frequently, please try again later.",
		EnDescription: "Too many emails sent from this IP. Please try again later.",
		Code:          429,
	}

	EmailSendFailedError = ErrorResponse{
		ErrorCode:     "email_send_failed",
		Description:   "Failed to send email",
		EnDescription: "Failed to send email",
		Code:          500,
	}

	UnknownError = ErrorResponse{
		ErrorCode:     "unknown_error",
		Description:   "Unknown error.",
		EnDescription: "Aunknown_error.",
		Code:          430,
	}

	LoginErrorRateLimitKeyPrefix = "login_error_rate_limit:"
	LoginMaxErrorLimits          = 5
)
