package gateway

import (
	"context"
	"testing"
)

func TestResolveInvocationSource(t *testing.T) {
	appCtx := &AppContext{}
	tests := []struct {
		name       string
		ctx        context.Context
		appCtx     *AppContext
		wantSource InvocationSource
	}{
		{name: "direct gateway call", ctx: context.Background(), wantSource: InvocationSourceAPI},
		{name: "internal app call", ctx: context.Background(), appCtx: appCtx, wantSource: InvocationSourceProduct},
		{name: "published app api", ctx: context.WithValue(context.Background(), "invoke_from", "external-api"), appCtx: appCtx, wantSource: InvocationSourceAPI},
		{name: "web app", ctx: context.WithValue(context.Background(), "invoke_from", "web-app"), appCtx: appCtx, wantSource: InvocationSourceProduct},
		{name: "automation", ctx: context.WithValue(context.Background(), "invoke_from", "automation"), appCtx: appCtx, wantSource: InvocationSourceProduct},
		{name: "internal client without app context", ctx: WithInvocationSource(context.Background(), InvocationSourceProduct), wantSource: InvocationSourceProduct},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveInvocationSource(tt.ctx, tt.appCtx); got != tt.wantSource {
				t.Fatalf("resolveInvocationSource() = %q, want %q", got, tt.wantSource)
			}
		})
	}
}
