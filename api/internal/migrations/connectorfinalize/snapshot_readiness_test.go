package connectorfinalize

import (
	"strings"
	"testing"
)

func TestSchema1ReadinessModes(t *testing.T) {
	for _, test := range []struct {
		name, state string
		version     int
		discard     bool
		wantError   bool
	}{
		{"plan ready", "ready", 1, false, false},
		{"plan conversion required", "conversion_required", 1, false, true},
		{"discard ready", "ready", 1, true, false},
		{"discard conversion required", "conversion_required", 1, true, false},
		{"discard schema2", "ready", 2, true, true},
		{"discard invalid state", "unknown", 1, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateSchema1Readiness(test.version, test.state, test.discard)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError = %v", err, test.wantError)
			}
			if err != nil && (!strings.Contains(err.Error(), test.state) || !strings.Contains(err.Error(), "schema_version=")) {
				t.Fatalf("error omits observed readiness: %v", err)
			}
		})
	}
}
