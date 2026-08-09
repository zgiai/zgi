package catalog_test

import (
	"errors"
	"fmt"

	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func ExampleCatalog_Present() {
	productCatalog, _ := catalog.NewDefault() // Build once during bootstrap and inject it.

	err := apperror.Wrap(
		errors.New("upstream deadline exceeded"),
		catalog.CodeLLMProviderTimeout,
		apperror.WithOperation("gateway.chat_completion"),
	)
	appErr, _ := apperror.As(err)
	presentation, _ := productCatalog.Present(
		appErr.Code(),
		catalog.LocaleEnglishUS,
		appErr.Params(),
	)

	// Only the catalog message crosses the public boundary. err.Error() remains
	// diagnostic because it contains the internal operation and cause.
	fmt.Println(presentation.Code, presentation.HTTPStatus, presentation.Message)
	// Output:
	// llm.provider.timeout 504 The model service took too long to respond. Try again or choose another model.
}
