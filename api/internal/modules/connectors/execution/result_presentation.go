package execution

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

type presentationLocaleKey struct{}

// WithPresentationLocale affects labels only, never execution or its identity.
func WithPresentationLocale(ctx context.Context, value string) context.Context {
	locale, _ := appcatalog.ParseLocale(value)
	return context.WithValue(ctx, presentationLocaleKey{}, locale)
}

func (service *Service) presentResult(ctx context.Context, result ExecutionResult) ExecutionResult {
	var locale appcatalog.Locale
	if ctx != nil {
		locale, _ = ctx.Value(presentationLocaleKey{}).(appcatalog.Locale)
	}
	if locale == "" {
		locale = appcatalog.LocaleEnglishUS
	}
	if result.Display == nil {
		result.Display = &OperationDisplay{}
	}
	result.Display.Locale = string(locale)
	if result.Evidence.ValidFor(result.State) && service.resultProjector != nil {
		err := core.ExecutionPresentationError(string(result.Evidence.Disposition), result.Evidence.DiagnosticCode)
		result.Display.Summary = service.resultProjector.Project(err, locale).Presentation.Message
	}
	return result
}
