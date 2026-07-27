package skills

import (
	"errors"
	"strings"
)

type publicErrorCoder interface {
	PublicErrorCode() string
}

// Keep provider details out of traces while preserving stable integration codes.
func skillTraceError(err error) (message string, code string) {
	if err == nil {
		return "", ""
	}
	var coded publicErrorCoder
	if errors.As(err, &coded) {
		code = strings.TrimSpace(coded.PublicErrorCode())
		if validIntegrationErrorCode(code) {
			return code, code
		}
	}
	return err.Error(), ""
}

func validIntegrationErrorCode(code string) bool {
	if !strings.HasPrefix(code, "integration_") || len(code) > 80 {
		return false
	}
	for _, character := range code {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' {
			continue
		}
		return false
	}
	return true
}
