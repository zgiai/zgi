package channelprovider

import "errors"

const providerAPIKeyInvalidMessage = "Private channel API key is invalid or expired. Update the API key and try again."

var ErrProviderAPIKeyInvalid = errors.New("provider API key is invalid or expired")

type providerAPIKeyInvalidError struct {
	cause error
}

func (e *providerAPIKeyInvalidError) Error() string {
	return providerAPIKeyInvalidMessage
}

func (e *providerAPIKeyInvalidError) Unwrap() error {
	return e.cause
}

func (e *providerAPIKeyInvalidError) Is(target error) bool {
	return target == ErrProviderAPIKeyInvalid
}

func newProviderAPIKeyInvalidError(cause error) error {
	return &providerAPIKeyInvalidError{cause: cause}
}

func UserVisibleValidationMessage(err error) (string, bool) {
	if errors.Is(err, ErrProviderAPIKeyInvalid) {
		return providerAPIKeyInvalidMessage, true
	}
	return "", false
}
