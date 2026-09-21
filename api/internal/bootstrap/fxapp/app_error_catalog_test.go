package fxapp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	connectorpreferences "github.com/zgiai/zgi/api/internal/modules/connectors/preferences"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	musicmodule "github.com/zgiai/zgi/api/internal/modules/music"
	authservice "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestProvideApplicationErrorCatalogComposesDomainDefinitions(t *testing.T) {
	t.Parallel()

	productCatalog, err := provideApplicationErrorCatalog()
	if err != nil {
		t.Fatalf("provideApplicationErrorCatalog() error = %v", err)
	}
	presentation, err := productCatalog.Present(
		llmerrors.AppCodeProviderTimeout,
		appcatalog.LocaleChineseSimplified,
		nil,
	)
	if err != nil {
		t.Fatalf("Present(LLM code) error = %v", err)
	}
	if presentation.HTTPStatus != 504 || presentation.Message == "" {
		t.Fatalf("LLM presentation = %#v", presentation)
	}
	for _, definition := range connectorpreferences.CatalogDefinitions() {
		localized, err := productCatalog.Present(definition.Code, appcatalog.LocaleChineseSimplified, nil)
		if err != nil || localized.Message != definition.Messages[appcatalog.LocaleChineseSimplified] {
			t.Fatalf("Connector preference definition missing: %s %v", definition.Code, err)
		}
	}
	musicPresentation, err := productCatalog.Present(
		musicmodule.AppCodeTaskNotDeletable,
		appcatalog.LocaleChineseSimplified,
		nil,
	)
	if err != nil {
		t.Fatalf("Present(music code) error = %v", err)
	}
	if musicPresentation.HTTPStatus != 409 || musicPresentation.Message != "音乐生成完成或失败后才能删除该任务。" {
		t.Fatalf("music presentation = %#v", musicPresentation)
	}
	registrationPresentation, err := productCatalog.Present(
		authservice.AppCodeRegistrationInvitationUnavailable,
		appcatalog.LocaleChineseSimplified,
		nil,
	)
	if err != nil {
		t.Fatalf("Present(registration code) error = %v", err)
	}
	if registrationPresentation.HTTPStatus != 401 || registrationPresentation.Message != "该邀请已失效或无法使用，请联系管理员获取新的邀请链接。" {
		t.Fatalf("registration presentation = %#v", registrationPresentation)
	}

	sharedCatalog, err := appcatalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sharedCatalog.Definition(llmerrors.AppCodeProviderTimeout); ok {
		t.Fatal("shared catalog unexpectedly owns an LLM definition")
	}
}

const contractCatalogPath = "testdata/error_catalog/error-catalog-v1.json"

type contractCatalog struct {
	Codes []struct {
		Code      string `json:"code"`
		Category  string `json:"category"`
		HTTP      int    `json:"http"`
		Retryable bool   `json:"retryable"`
		Domain    string `json:"domain"`
		GRPC      string `json:"grpc"`
		EN        string `json:"en"`
		ZH        string `json:"zh"`
	} `json:"codes"`
}

var contractCategories = map[string]appcatalog.Category{
	"validation":     appcatalog.CategoryValidation,
	"authentication": appcatalog.CategoryAuthentication,
	"authorization":  appcatalog.CategoryAuthorization,
	"not_found":      appcatalog.CategoryNotFound,
	"conflict":       appcatalog.CategoryConflict,
	"quota":          appcatalog.CategoryQuota,
	"rate_limit":     appcatalog.CategoryRateLimit,
	"upstream":       appcatalog.CategoryUpstream,
	"internal":       appcatalog.CategoryInternal,
	"configuration":  appcatalog.CategoryConfiguration,
}

// TestApplicationErrorCatalogCoversContract is the T2 completeness gate: every
// code frozen in error-catalog-v1.json must be present in the composed process
// catalog with the same category, HTTP status, retryable flag and bilingual
// message. Extra Pre-only codes are allowed.
func TestApplicationErrorCatalogCoversContract(t *testing.T) {
	productCatalog, err := provideApplicationErrorCatalog()
	if err != nil {
		t.Fatalf("compose application error catalog: %v", err)
	}
	definitions := make(map[string]appcatalog.Definition)
	for _, definition := range productCatalog.Definitions() {
		definitions[definition.Code.String()] = definition
	}

	raw, err := os.ReadFile(contractCatalogPath)
	if err != nil {
		t.Fatalf("read contract catalog: %v", err)
	}
	var contract contractCatalog
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("decode contract catalog: %v", err)
	}
	if len(contract.Codes) == 0 {
		t.Fatal("contract catalog contains no codes")
	}

	for _, expected := range contract.Codes {
		definition, ok := definitions[expected.Code]
		if !ok {
			t.Fatalf("contract code %s is missing from the composed Pre catalog", expected.Code)
		}
		wantCategory, ok := contractCategories[expected.Category]
		if !ok {
			t.Fatalf("contract code %s uses unknown category %q", expected.Code, expected.Category)
		}
		if definition.Category != wantCategory {
			t.Fatalf("code %s category = %s, want %s", expected.Code, definition.Category, wantCategory)
		}
		if definition.HTTPStatus != expected.HTTP {
			t.Fatalf("code %s http = %d, want %d", expected.Code, definition.HTTPStatus, expected.HTTP)
		}
		if definition.Retryable != expected.Retryable {
			t.Fatalf("code %s retryable = %v, want %v", expected.Code, definition.Retryable, expected.Retryable)
		}
		if definition.Messages[appcatalog.LocaleEnglishUS] != expected.EN {
			t.Fatalf("code %s en-US = %q, want %q", expected.Code, definition.Messages[appcatalog.LocaleEnglishUS], expected.EN)
		}
		if definition.Messages[appcatalog.LocaleChineseSimplified] != expected.ZH {
			t.Fatalf("code %s zh-Hans = %q, want %q", expected.Code, definition.Messages[appcatalog.LocaleChineseSimplified], expected.ZH)
		}
	}
}

func TestApplicationErrorCatalogMirrorHash(t *testing.T) {
	sumsRaw, err := os.ReadFile(filepath.Join(filepath.Dir(contractCatalogPath), "SHA256SUMS"))
	if err != nil {
		t.Fatalf("read SHA256SUMS: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(sumsRaw)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("malformed SHA256SUMS line %q", line)
		}
		content, err := os.ReadFile(filepath.Join(filepath.Dir(contractCatalogPath), fields[1]))
		if err != nil {
			t.Fatalf("read mirror: %v", err)
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != fields[0] {
			t.Fatalf("error catalog mirror %s drifted from the lock file", fields[1])
		}
	}
}
