package response

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/pkg/logger"
)

const (
	CodeSuccess = 0

	CodeParamErrorStart = 100000
	CodeParamErrorEnd   = 200000

	CodeBusinessErrorStart = 200000
	CodeBusinessErrorEnd   = 300000

	CodeSystemErrorStart = 300000
	CodeSystemErrorEnd   = 400000

	CodeAuthErrorStart = 400000
	CodeAuthErrorEnd   = 500000

	CodeForbiddenStart = 403000
	CodeForbiddenEnd   = 404000

	CodeNotFoundStart = 404000
	CodeNotFoundEnd   = 405000

	CodeThirdPartyErrorStart = 500000
	CodeThirdPartyErrorEnd   = 600000
)

var specialStatusMapping = map[int]int{
	201002: 404,
	201007: 404,
	201012: 404,
	202001: 404,
	203001: 404,
	203010: 404,
	204001: 404,
	204002: 404,
	204008: 403,
	205001: 404,
	205004: 404,
	205008: 404,
	205016: 404,
	206001: 404,
	206008: 404,
	206009: 404,
	206010: 404,
	207004: 404,
	210001: 404,
	211001: 404,
	404001: 404,

	202009: 403,
	209007: 403,
	209008: 403,
	211008: 403,

	201008: 429,
	201018: 429,

	201004: 402,
	202004: 402,
	207001: 402,
	207002: 402,
	207011: 402,
	207012: 402,
	207013: 402,
	501002: 402,

	201001: 409,
	202002: 409,
	205002: 409,
	205003: 409,
	205005: 409,
	205009: 409,
	205013: 409,
	205014: 409,

	202003: 202,
	203002: 202,
	212004: 202,

	205020: 200,
	205021: 200,
}

type Response struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ErrorResponse represents an error response for API documentation
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorCodeConflict struct {
	Code         int      `json:"code"`
	ConflictWith []string `json:"conflict_with"`
	Suggestion   string   `json:"suggestion"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(200, Response{
		Code:    "0",
		Message: "success",
		Data:    data,
	})
}

func translateErrorMsg(err ErrorCode) Response {
	// Always return detailed error message for better debugging
	msg := err.Message
	if msg == "" {
		msg = "系统繁忙，请稍后重试"
	}

	return Response{
		Code:    strconv.Itoa(err.Code),
		Message: msg,
	}
}

// Fail send fail response to client with error code validation
func Fail(c *gin.Context, err ErrorCode) {
	if !ValidateErrorCode(err.Code) {
		logger.WarnContext(c.Request.Context(), "invalid response error code", "code", err.Code, "range", GetErrorCodeRange(err.Code))
	}

	resp := translateErrorMsg(err)
	httpStatus := getHTTPStatusFromErrorCode(err.Code)
	c.JSON(httpStatus, resp)
}

func getHTTPStatusFromErrorCode(code int) int {
	// Check special status mapping first
	if status, exists := specialStatusMapping[code]; exists {
		return status
	}

	// Handle 5-digit module-based error codes (new standard)
	if code >= 10000 && code < 100000 {
		return getHTTPStatusFor5DigitCode(code)
	}

	// Handle 6-digit legacy error codes
	switch {
	case code == CodeSuccess:
		return 200 // Success

	case code >= CodeParamErrorStart && code < CodeParamErrorEnd:
		return 400

	case code >= CodeBusinessErrorStart && code < CodeBusinessErrorEnd:
		return 400

	case code >= CodeSystemErrorStart && code < CodeSystemErrorEnd:
		return 500

	case code >= CodeNotFoundStart && code < CodeNotFoundEnd:
		return 404

	case code >= CodeForbiddenStart && code < CodeForbiddenEnd:
		return 403

	case code >= CodeAuthErrorStart && code < CodeAuthErrorEnd:
		return 401

	case code >= CodeThirdPartyErrorStart && code < CodeThirdPartyErrorEnd:
		return 500

	default:
		return 500
	}
}

// getHTTPStatusFor5DigitCode maps 5-digit module-based error codes to HTTP status
// Format: MMSSS where MM is module code, SSS is specific error
func getHTTPStatusFor5DigitCode(code int) int {
	// Extract error category from last 3 digits
	category := code % 1000

	switch {
	// Authentication errors (x01xx) -> 401
	case category >= 100 && category < 200:
		return 401

	// Authorization/Permission errors (x03xx) -> 403
	case category >= 300 && category < 400:
		return 403

	// Not Found errors (x04xx) -> 404
	case category >= 400 && category < 500:
		return 404

	// Upstream/Provider errors (x05xx) -> 502/503/504
	case category >= 500 && category < 600:
		// Special handling for specific upstream errors
		switch category {
		case 502: // Rate limit
			return 429
		case 503: // Timeout
			return 504
		case 504: // Unavailable
			return 503
		case 506: // No provider available
			return 503
		default:
			return 502 // Bad Gateway
		}

	// System errors (x06xx) -> 500
	case category >= 600 && category < 700:
		return 500

	// Rate limit (x09xx) -> 429
	case category >= 900 && category < 1000:
		return 429

	// Parameter/validation errors (x00xx) -> 400
	default:
		return 400
	}
}

func ValidateErrorCode(code int) bool {
	return code == CodeSuccess ||
		(code >= 10000 && code < 100000) || // 5-digit module-based codes
		(code >= CodeParamErrorStart && code < CodeThirdPartyErrorEnd) // 6-digit legacy codes
}

func GetErrorCodeRange(code int) string {
	switch {
	case code == CodeSuccess:
		return "SUCCESS"
	case code >= CodeParamErrorStart && code < CodeParamErrorEnd:
		return "PARAM_ERROR"
	case code >= CodeBusinessErrorStart && code < CodeBusinessErrorEnd:
		return "BUSINESS_ERROR"
	case code >= CodeSystemErrorStart && code < CodeSystemErrorEnd:
		return "SYSTEM_ERROR"
	case code >= CodeAuthErrorStart && code < CodeAuthErrorEnd:
		return "AUTH_ERROR"
	case code >= CodeThirdPartyErrorStart && code < CodeThirdPartyErrorEnd:
		return "THIRD_PARTY_ERROR"
	default:
		return "UNKNOWN"
	}
}

func SpecialFail(c *gin.Context, data interface{}) {
	c.JSON(200, data)
}

// FailWithMessage sends fail response to client with custom error message
func FailWithMessage(c *gin.Context, err ErrorCode, message string) {
	if !ValidateErrorCode(err.Code) {
		logger.WarnContext(c.Request.Context(), "invalid response error code", "code", err.Code, "range", GetErrorCodeRange(err.Code))
	}

	resp := translateErrorMsg(err)
	// Override message with custom message
	resp.Message = message
	httpStatus := getHTTPStatusFromErrorCode(err.Code)
	c.JSON(httpStatus, resp)
}
