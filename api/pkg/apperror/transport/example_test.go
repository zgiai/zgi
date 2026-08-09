package transport_test

import (
	"fmt"

	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"github.com/zgiai/zgi/api/pkg/apperror/transport"
)

func ExampleProjector_ProjectLegacyMessage() {
	legacyKey := catalog.MustLegacyKey("example.gateway:40503")
	code := apperror.MustCode("example.provider.timeout")
	definitions := append(catalog.DefaultDefinitions(), catalog.Definition{
		Code:       code,
		Category:   catalog.CategoryUpstream,
		HTTPStatus: 504,
		Retryable:  true,
		Messages: map[catalog.Locale]string{
			catalog.LocaleEnglishUS:         "The model service timed out. Try again.",
			catalog.LocaleChineseSimplified: "大模型服务响应超时，请重试。",
		},
		LegacyCodes: []catalog.LegacyKey{legacyKey},
	})
	productCatalog, _ := catalog.New(catalog.LocaleEnglishUS, catalog.CodeInternal, definitions...)
	projector, _ := transport.NewProjector(productCatalog)

	public := projector.ProjectLegacyMessage(
		apperror.New(code),
		transport.LocaleFromAcceptLanguage("en-US,en;q=0.9"),
		legacyKey,
	)

	// These remain owned by the existing protocol adapter.
	legacyHTTPStatus := 504
	legacyWireCode := 40503
	fmt.Println(legacyHTTPStatus, legacyWireCode, public.Message)

	// Output:
	// 504 40503 The model service timed out. Try again.
}
