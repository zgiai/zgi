package workflowevents

import "context"

type contextKey struct{}

// Event is a normalized workflow event emitted while an Agent workflow tool is running.
type Event struct {
	Type    string
	Payload map[string]interface{}
}

// Emitter receives workflow runtime events.
type Emitter func(Event)

// WithEmitter stores a workflow event emitter in ctx.
func WithEmitter(ctx context.Context, emitter Emitter) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if emitter == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, emitter)
}

// FromContext returns the workflow event emitter stored in ctx.
func FromContext(ctx context.Context) Emitter {
	if ctx == nil {
		return nil
	}
	emitter, _ := ctx.Value(contextKey{}).(Emitter)
	return emitter
}
