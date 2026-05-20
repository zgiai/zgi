package llmerrors

import (
	"errors"
	"strings"
	"testing"
)

func TestConvertError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedCode int
	}{
		{
			name:         "Model Not Found",
			err:          DomainErrModelNotFound,
			expectedCode: ErrCodeModelNotFound, // 40401
		},
		{
			name:         "Provider Not Found",
			err:          DomainErrProviderNotFound,
			expectedCode: ErrCodeProviderNotFound, // 40402
		},
		{
			name:         "No Provider Available",
			err:          DomainErrNoProviderAvailable,
			expectedCode: ErrCodeNoProviderAvailable, // 40506
		},
		{
			name:         "Invalid API Key",
			err:          DomainErrInvalidAPIKey,
			expectedCode: ErrCodeInvalidAPIKey, // 40101
		},
		{
			name:         "Insufficient Balance",
			err:          DomainErrInsufficientBalance,
			expectedCode: ErrCodeInsufficientBalance, // 40301
		},
		{
			name:         "Missing Model",
			err:          DomainErrMissingModel,
			expectedCode: ErrCodeMissingModel, // 40001
		},
		{
			name:         "Internal Error",
			err:          DomainErrInternalError,
			expectedCode: ErrCodeInternalError, // 40601
		},
		{
			name:         "Unknown Error",
			err:          errors.New("some unknown error"),
			expectedCode: ErrCodeInternalError, // Default to internal error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertError(tt.err)
			if result.Code != tt.expectedCode {
				t.Errorf("ConvertError() code = %d, want %d", result.Code, tt.expectedCode)
			}
		})
	}
}

func TestWrapError(t *testing.T) {
	originalErr := errors.New("database connection failed")
	wrappedErr := WrapError(originalErr, DomainErrDatabaseError)

	// Should be able to unwrap to domain error
	if !errors.Is(wrappedErr, DomainErrDatabaseError) {
		t.Error("WrapError() should wrap with domain error")
	}

	// ConvertError should recognize the wrapped error
	result := ConvertError(wrappedErr)
	if result.Code != ErrCodeDatabaseError {
		t.Errorf("ConvertError(wrapped) code = %d, want %d", result.Code, ErrCodeDatabaseError)
	}
}

func TestErrorHelpers(t *testing.T) {
	t.Run("NewModelNotFoundErrorWithName", func(t *testing.T) {
		err := NewModelNotFoundErrorWithName("gpt-4")
		if !errors.Is(err, DomainErrModelNotFound) {
			t.Error("Should wrap DomainErrModelNotFound")
		}
		if err.Error() == "" {
			t.Error("Should have error message")
		}
		if !strings.Contains(err.Error(), "full model name") {
			t.Errorf("error message = %q, want guidance about full model name", err.Error())
		}
	})

	t.Run("NewRouteNotFoundErrorWithDetails", func(t *testing.T) {
		err := NewRouteNotFoundErrorWithDetails("gpt-4", "tenant-123", 5)
		if !errors.Is(err, DomainErrRouteNotFound) {
			t.Error("Should wrap DomainErrRouteNotFound")
		}
	})

	t.Run("NewUpstreamErrorWithProvider", func(t *testing.T) {
		originalErr := errors.New("connection refused")
		err := NewUpstreamErrorWithProvider("openai", originalErr)
		if !errors.Is(err, DomainErrUpstreamFailed) {
			t.Error("Should wrap DomainErrUpstreamFailed")
		}
	})
}

func TestConvertError_PreservesDetailedModelNotFoundMessage(t *testing.T) {
	err := NewModelNotFoundErrorWithName("ByteDance-Seed/Seed-OSS-36B-Instruct")

	result := ConvertError(err)

	if result.Code != ErrCodeModelNotFound {
		t.Fatalf("ConvertError(err).Code = %d, want %d", result.Code, ErrCodeModelNotFound)
	}
	if result.Message == ErrModelNotFound.Message {
		t.Fatalf("ConvertError(err).Message = %q, want detailed message", result.Message)
	}
	if !containsSubstring(result.Message, "ByteDance-Seed/Seed-OSS-36B-Instruct") {
		t.Fatalf("ConvertError(err).Message = %q, want model name included", result.Message)
	}
}

func containsSubstring(text, want string) bool {
	return len(text) >= len(want) && (text == want || len(text) > 0 && errors.New(text).Error() != "" && (func() bool {
		return stringIndex(text, want) >= 0
	})())
}

func stringIndex(text, want string) int {
	for i := 0; i+len(want) <= len(text); i++ {
		if text[i:i+len(want)] == want {
			return i
		}
	}
	return -1
}
