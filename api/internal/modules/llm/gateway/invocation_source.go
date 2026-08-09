package gateway

import (
	"context"
	"strings"
)

// InvocationSource is the stable, business-facing origin of a Gateway request.
// It deliberately has fewer values than app invoke_from so analytics remains
// understandable when new product surfaces are added.
type InvocationSource string

type invocationSourceContextKey struct{}

const (
	InvocationSourceAPI     InvocationSource = "api"
	InvocationSourceProduct InvocationSource = "product"
	InvocationSourceUnknown InvocationSource = "unknown"
)

// WithInvocationSource lets trusted in-process callers declare their stable
// business source without adding parameters throughout every protocol method.
func WithInvocationSource(ctx context.Context, source InvocationSource) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, invocationSourceContextKey{}, normalizeInvocationSource(source))
}

func resolveInvocationSource(ctx context.Context, appCtx *AppContext) InvocationSource {
	if ctx != nil {
		if source, ok := ctx.Value(invocationSourceContextKey{}).(InvocationSource); ok {
			if normalized := normalizeInvocationSource(source); normalized != InvocationSourceUnknown {
				return normalized
			}
		}
		if invokeFrom, ok := ctx.Value("invoke_from").(string); ok {
			switch strings.TrimSpace(invokeFrom) {
			case "external-api", "service-api":
				return InvocationSourceAPI
			case "debugger", "web-app", "workflow", "automation":
				return InvocationSourceProduct
			}
		}
	}

	// Calls made directly through the OpenAI/Anthropic-compatible Gateway do
	// not carry app context. Internal product calls always use AppContext.
	if appCtx == nil {
		return InvocationSourceAPI
	}
	return InvocationSourceProduct
}

func normalizeInvocationSource(source InvocationSource) InvocationSource {
	switch InvocationSource(strings.TrimSpace(string(source))) {
	case InvocationSourceAPI:
		return InvocationSourceAPI
	case InvocationSourceProduct:
		return InvocationSourceProduct
	default:
		return InvocationSourceUnknown
	}
}
