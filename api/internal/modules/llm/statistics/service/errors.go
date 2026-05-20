package service

import "errors"

var (
	ErrInvalidTimestamp      = errors.New("invalid timestamp, expected unix seconds")
	ErrInvalidTimestampRange = errors.New("end_timestamp must be greater than or equal to start_timestamp")
)

func IsValidationError(err error) bool {
	return errors.Is(err, ErrInvalidTimestamp) || errors.Is(err, ErrInvalidTimestampRange)
}
