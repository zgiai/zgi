package providers

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// OptionalCode distinguishes an absent documented zero default from null or a
// value of another type. Missing fields retain the zero-value, Present=false.
type OptionalCode struct {
	Value   int
	Present bool
}

func (c *OptionalCode) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("null response code")
	}
	if err := json.Unmarshal(raw, &c.Value); err != nil {
		return err
	}
	c.Present = true
	return nil
}
func ResponseFailure(phase string, code core.ErrorCode, unknown bool, providerCode int) error {
	category := "response_unrecognized"
	if providerCode != 0 {
		category = "provider"
	}
	return &Failure{Code: code, Unknown: unknown, Diagnostic: &Diagnostic{TraceID: uuid.NewString(), Phase: phase, Category: category, HTTPStatus: 200, ProviderCode: providerCode, RequestWritten: true, ResponseReceived: true}}
}
