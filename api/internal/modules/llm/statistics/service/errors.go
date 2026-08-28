package service

import "errors"

var (
	ErrInvalidTimestamp                 = errors.New("invalid timestamp, expected unix seconds")
	ErrInvalidTimestampRange            = errors.New("end_timestamp must be greater than or equal to start_timestamp")
	ErrInvalidCursor                    = errors.New("invalid invocation cursor")
	ErrInvalidInvocationContentSettings = errors.New("invalid invocation content settings")
	ErrInvocationContentNotFound        = errors.New("invocation content is unavailable or has expired")
)

func IsValidationError(err error) bool {
	return errors.Is(err, ErrInvalidTimestamp) || errors.Is(err, ErrInvalidTimestampRange) ||
		errors.Is(err, ErrInvalidCursor) || errors.Is(err, ErrInvalidInvocationContentSettings)
}
