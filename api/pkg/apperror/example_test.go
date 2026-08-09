package apperror_test

import (
	"errors"
	"fmt"

	"github.com/zgiai/zgi/api/pkg/apperror"
)

// Codes are declared once at package scope. They are identities, not messages.
var exampleCodeProviderTimeout = apperror.MustCode("llm.provider.timeout")

func Example() {
	providerErr := errors.New("upstream deadline exceeded")
	err := apperror.Wrap(
		providerErr,
		exampleCodeProviderTimeout,
		apperror.WithOperation("gateway.chat_completion"),
		apperror.WithParams(
			apperror.StringParam("provider", "example"),
			apperror.IntParam("timeout_seconds", 30),
		),
	)

	appErr, ok := apperror.As(err)
	fmt.Println(ok)
	fmt.Println(appErr.Code())
	fmt.Println(appErr.Operation())
	fmt.Println(errors.Is(err, providerErr))
	fmt.Println(apperror.IsCode(err, exampleCodeProviderTimeout))

	// Output:
	// true
	// llm.provider.timeout
	// gateway.chat_completion
	// true
	// true
}
