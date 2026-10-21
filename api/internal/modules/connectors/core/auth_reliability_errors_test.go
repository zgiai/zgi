package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestAuthReliabilityErrorCatalogAndLegacyMapping(t *testing.T) {
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, AppCodeFlowExpired, CatalogDefinitions()...)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range authErrorDefinitions {
		err := NewError(definition.legacy, false, errors.New("private credential"))
		projected, ok := ApplicationError(err)
		if !ok || !apperror.IsCode(projected, definition.code) {
			t.Fatalf("unmapped code %s", definition.legacy)
		}
		for _, locale := range []appcatalog.Locale{appcatalog.LocaleEnglishUS, appcatalog.LocaleChineseSimplified} {
			presentation, err := catalog.Present(definition.code, locale, nil)
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(presentation)
			if strings.Contains(string(encoded), "private credential") || len(encoded) < 20 {
				t.Fatalf("unsafe presentation %s", encoded)
			}
		}
	}
}

type diagnosticTestCause struct{ diagnostic *ProviderDiagnostic }

func (cause diagnosticTestCause) Error() string                       { return "raw-secret-provider-cause" }
func (cause diagnosticTestCause) SafeDiagnostic() *ProviderDiagnostic { return cause.diagnostic }

func TestErrorRetainsOnlyWhitelistedDiagnosticCopy(t *testing.T) {
	diagnostic := &ProviderDiagnostic{TraceID: uuid.NewString(), Phase: "poll", Category: "deadline", ElapsedMS: 40000, RequestWritten: true}
	err := NewError(ErrorCodeVerificationUncertain, false, diagnosticTestCause{diagnostic})
	diagnostic.Phase = "raw-secret-provider-cause"
	copy := DiagnosticOf(err)
	if copy == nil || copy.Phase != "poll" || !copy.RequestWritten {
		t.Fatalf("lost safe diagnostic: %#v", copy)
	}
	copy.Phase = "changed"
	if DiagnosticOf(err).Phase != "poll" {
		t.Fatal("diagnostic returned shared mutable state")
	}
	encoded, _ := json.Marshal(err)
	for _, output := range []string{fmt.Sprintf("%#v", err), string(encoded), err.Error()} {
		if strings.Contains(output, "raw-secret") {
			t.Fatal("raw provider information retained")
		}
	}
	if errors.Unwrap(err) != nil {
		t.Fatal("raw cause retained")
	}
	if DiagnosticOf(NewError(ErrorCodeProviderUnavailable, true, diagnosticTestCause{diagnostic})) != nil {
		t.Fatal("unbounded phase accepted")
	}
	if DiagnosticOf(errors.New("raw-secret-provider-cause")) != nil {
		t.Fatal("unstructured diagnostic accepted")
	}
}
