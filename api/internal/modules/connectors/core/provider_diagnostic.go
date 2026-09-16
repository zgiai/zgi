package core

import (
	"errors"

	"github.com/google/uuid"
)

// ProviderDiagnostic contains only bounded metadata. URLs, headers, response
// bodies and provider error messages must never be retained here.
type ProviderDiagnostic struct {
	TraceID          string
	Phase            string
	Category         string
	HTTPStatus       int
	ProviderCode     int
	ElapsedMS        int64
	RequestWritten   bool
	ResponseReceived bool
}

func (diagnostic *ProviderDiagnostic) SafeCopy() *ProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	trace, err := uuid.Parse(diagnostic.TraceID)
	if err != nil || trace == uuid.Nil {
		return nil
	}
	switch diagnostic.Phase {
	case "token", "identity", "operation", "poll":
	default:
		return nil
	}
	switch diagnostic.Category {
	case "dns", "connect", "tls", "deadline", "cancelled", "http", "response_unrecognized", "provider":
	default:
		return nil
	}
	if diagnostic.HTTPStatus != 0 && (diagnostic.HTTPStatus < 100 || diagnostic.HTTPStatus > 599) {
		return nil
	}
	if diagnostic.ProviderCode < -2147483648 || diagnostic.ProviderCode > 2147483647 || diagnostic.ElapsedMS < 0 || diagnostic.ElapsedMS > 86400000 {
		return nil
	}
	copy := *diagnostic
	copy.TraceID = trace.String()
	return &copy
}

// DiagnosticOf copies only the typed safe metadata; it never unwraps or stores
// the original provider cause as part of a Connector error.
func DiagnosticOf(err error) *ProviderDiagnostic {
	var carrier interface{ SafeDiagnostic() *ProviderDiagnostic }
	if errors.As(err, &carrier) {
		return carrier.SafeDiagnostic().SafeCopy()
	}
	return nil
}

func (err *Error) SafeDiagnostic() *ProviderDiagnostic {
	if err == nil {
		return nil
	}
	return err.diagnostic.SafeCopy()
}
