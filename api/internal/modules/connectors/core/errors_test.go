package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestConnectorErrorNeverFormatsOrSerializesCause(t *testing.T) {
	const sentinel = "connector-key-sentinel-error-73ce51"
	err := NewError(ErrorCodeInvalidKey, false, errors.New(sentinel))

	for _, rendered := range []string{
		err.Error(),
		fmt.Sprint(err),
		fmt.Sprintf("%v", err),
		fmt.Sprintf("%+v", err),
		fmt.Sprintf("%#v", err),
	} {
		if strings.Contains(rendered, sentinel) {
			t.Fatalf("formatted Connector error exposed submitted Key sentinel: %q", rendered)
		}
	}
	encoded, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("json.Marshal(error) error = %v", marshalErr)
	}
	if strings.Contains(string(encoded), sentinel) {
		t.Fatalf("serialized Connector error exposed submitted Key sentinel: %s", encoded)
	}
	if got := ErrorCodeOf(err); got != ErrorCodeInvalidKey {
		t.Fatalf("ErrorCodeOf(error) = %q, want %q", got, ErrorCodeInvalidKey)
	}
	if errors.Unwrap(err) != nil {
		t.Fatal("Connector error retained a potentially sensitive cause")
	}
}

func TestConnectorStableErrorCodes(t *testing.T) {
	for _, code := range []ErrorCode{
		ErrorCodeInvalidInput,
		ErrorCodeNotFound,
		ErrorCodeAccessDenied,
		ErrorCodeConflict,
		ErrorCodeInvalidKey,
		ErrorCodeInsufficientKey,
		ErrorCodeProviderUnavailable,
		ErrorCodeConnectorUnavailable,
		ErrorCodeConnectionNotReady,
		ErrorCodeTargetNotReady,
		ErrorCodeOutcomeUnknown,
		ErrorCodeInternal,
	} {
		if !code.Valid() {
			t.Fatalf("stable error code %q was rejected", code)
		}
		if got := NewError(code, true, nil).Error(); got != string(code) {
			t.Fatalf("Error() = %q, want stable code %q", got, code)
		}
	}
	if ErrorCode("provider message").Valid() {
		t.Fatal("arbitrary provider message accepted as a stable error code")
	}
}
