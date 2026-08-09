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
		errors.New("request budget exhausted"),
		catalog.CodeRateLimitExceeded,
		apperror.WithOperation("api.request"),
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
	// rate_limit.exceeded 429 Too many requests were sent. Wait a moment and try again.
}
