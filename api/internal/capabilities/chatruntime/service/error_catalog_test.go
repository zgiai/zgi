package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestContextCompactionErrorCatalog(t *testing.T) {
	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}
	for _, locale := range []appcatalog.Locale{appcatalog.LocaleEnglishUS, appcatalog.LocaleChineseSimplified} {
		presentation, err := catalog.Present(AppCodeContextCompactionUnavailable, locale, nil)
		if err != nil {
			t.Fatalf("present %s: %v", locale, err)
		}
		if presentation.HTTPStatus != 503 || !presentation.Retryable || presentation.Message == "" {
			t.Fatalf("presentation = %#v", presentation)
		}
	}
}

func TestContextCompactionStreamPayloadIsStableAndSafe(t *testing.T) {
	cause := errors.New("provider secret failure")
	err := newContextCompactionUnavailableError(cause)
	if !apperror.IsCode(err, AppCodeContextCompactionUnavailable) {
		t.Fatalf("error = %v, want stable code", err)
	}
	prepared := streamErrorTestPrepared()
	payload := BuildStreamErrorPayload(prepared, err)
	if payload["code"] != AppCodeContextCompactionUnavailable.String() || payload["retryable"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	message, _ := payload["message"].(string)
	if message == "" || strings.Contains(message, cause.Error()) {
		t.Fatalf("public message = %q", message)
	}
}
