package observability

import (
	"context"
	"net/http"

	"github.com/zgiai/ginext/config"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

const workflowTracerName = "zgi.workflow"

var noopSpan = trace.SpanFromContext(context.Background())

func otelConfig() config.OpenTelemetryConfig {
	if config.GlobalConfig == nil {
		return config.OpenTelemetryConfig{}
	}
	return config.GlobalConfig.OpenTelemetry
}

func tracingEnabled() bool {
	return otelConfig().Enabled
}

// HTTPClient instruments an HTTP client when outbound HTTP tracing is enabled.
func HTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	if !HTTPClientEnabled() {
		return client
	}
	client.Transport = HTTPTransport(client.Transport)
	return client
}

// HTTPTransport wraps a RoundTripper with OpenTelemetry outbound tracing.
func HTTPTransport(base http.RoundTripper) http.RoundTripper {
	if !HTTPClientEnabled() {
		return base
	}
	if base == nil {
		base = http.DefaultTransport
	}
	if _, ok := base.(*otelhttp.Transport); ok {
		return base
	}
	return otelhttp.NewTransport(
		base,
		otelhttp.WithSpanNameFormatter(outboundHTTPSpanName),
		otelhttp.WithFilter(hasActiveSpanContext),
	)
}

func hasActiveSpanContext(r *http.Request) bool {
	if r == nil {
		return false
	}
	return trace.SpanContextFromContext(r.Context()).IsValid()
}

func outboundHTTPSpanName(_ string, r *http.Request) string {
	if r == nil || r.URL == nil {
		return "HTTP"
	}
	host := r.URL.Host
	if host == "" {
		host = "unknown"
	}
	return SanitizeString(r.Method + " " + host)
}

func HTTPClientEnabled() bool {
	cfg := otelConfig()
	return cfg.Enabled && cfg.InstrumentHTTPClient
}

func WorkflowEnabled() bool {
	cfg := otelConfig()
	return cfg.Enabled && cfg.InstrumentWorkflow
}

func DBEnabled() bool {
	cfg := otelConfig()
	return cfg.Enabled && cfg.InstrumentDB
}

func RedisEnabled() bool {
	cfg := otelConfig()
	return cfg.Enabled && cfg.InstrumentRedis
}

func GRPCEnabled() bool {
	cfg := otelConfig()
	return cfg.Enabled && cfg.InstrumentGRPC
}

func StartWorkflowRunSpan(ctx context.Context, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return startPerformanceSpan(ctx, "workflow.run", WorkflowEnabled(), attrs...)
}

func StartWorkflowNodeSpan(ctx context.Context, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return startPerformanceSpan(ctx, "workflow.node", WorkflowEnabled(), attrs...)
}

func EndSpan(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(SanitizeError(err))
		span.SetStatus(codes.Error, SanitizeString(err.Error()))
	}
	span.End()
}

func GRPCServerOptions() []grpc.ServerOption {
	if !GRPCEnabled() {
		return nil
	}
	return []grpc.ServerOption{grpc.StatsHandler(otelgrpc.NewServerHandler())}
}

func GRPCDialOptions() []grpc.DialOption {
	if !GRPCEnabled() {
		return nil
	}
	return []grpc.DialOption{grpc.WithStatsHandler(otelgrpc.NewClientHandler())}
}

func startPerformanceSpan(ctx context.Context, name string, enabled bool, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !tracingEnabled() || !enabled {
		return ctx, noopSpan
	}
	return otel.Tracer(workflowTracerName).Start(ctx, name, trace.WithAttributes(SanitizeAttributes(attrs)...))
}
