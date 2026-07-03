package observer

import "context"

type requestIDKey struct{}

func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func MetadataWithContext(ctx context.Context, metadata map[string]any) map[string]any {
	requestID := RequestIDFromContext(ctx)
	if requestID == "" {
		return metadata
	}
	if metadata == nil {
		metadata = make(map[string]any, 1)
	} else {
		copy := make(map[string]any, len(metadata)+1)
		for key, value := range metadata {
			copy[key] = value
		}
		metadata = copy
	}
	metadata["request_id"] = requestID
	return metadata
}
