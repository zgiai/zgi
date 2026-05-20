package fxapp

import (
	"context"
	"errors"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	otelHTTPProtobufProtocol = "http/protobuf"
	defaultOTELShutdown      = 5 * time.Second
)

var errMissingOTELEndpoint = errors.New("missing OpenTelemetry OTLP endpoint")

type OpenTelemetryResource struct {
	Enabled        bool
	TracerProvider *sdktrace.TracerProvider
}

func provideOpenTelemetryResource(cfg *config.Config, log *zap.Logger) (*OpenTelemetryResource, error) {
	if cfg == nil || !cfg.OpenTelemetry.Enabled {
		log.Info("OpenTelemetry tracing disabled")
		return &OpenTelemetryResource{}, nil
	}

	if strings.TrimSpace(cfg.OpenTelemetry.Protocol) != otelHTTPProtobufProtocol {
		log.Warn(
			"OpenTelemetry tracing disabled because protocol is unsupported",
			zap.String("protocol", cfg.OpenTelemetry.Protocol),
			zap.String("supported_protocol", otelHTTPProtobufProtocol),
		)
		return &OpenTelemetryResource{}, nil
	}

	opts, endpoint, insecure, err := buildOTLPHTTPOptions(cfg.OpenTelemetry.Endpoint, cfg.OpenTelemetry.TracesEndpoint, cfg.OpenTelemetry.Headers)
	if err != nil {
		log.Warn("OpenTelemetry tracing disabled because endpoint is invalid", zap.Error(err))
		return &OpenTelemetryResource{}, nil
	}

	exp, err := otlptracehttp.New(context.Background(), opts...)
	if err != nil {
		log.Warn("OpenTelemetry tracing disabled because exporter initialization failed", zap.Error(err))
		return &OpenTelemetryResource{}, nil
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			"",
			attribute.String("service.name", observability.SanitizeString(cfg.OpenTelemetry.ServiceName)),
			attribute.String("deployment.environment", observability.SanitizeString(cfg.Server.Environment)),
			attribute.String("service.version", observability.SanitizeString(cfg.Sentry.Release)),
		),
	)
	if err != nil {
		log.Warn("OpenTelemetry resource merge failed", zap.Error(err))
		res = resource.Default()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(buildOTELSampler(cfg.OpenTelemetry.TraceSampleRate)),
		sdktrace.WithSpanProcessor(observability.NewLangfuseTraceAttributeSpanProcessor()),
		sdktrace.WithBatcher(exp),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Info(
		"OpenTelemetry tracing enabled",
		zap.String("endpoint", endpoint),
		zap.Bool("insecure", insecure),
		zap.Float64("sample_rate", normalizedOTELSampleRate(cfg.OpenTelemetry.TraceSampleRate)),
	)
	return &OpenTelemetryResource{Enabled: true, TracerProvider: tp}, nil
}

func buildOTELSampler(sampleRate float64) sdktrace.Sampler {
	sampleRate = normalizedOTELSampleRate(sampleRate)
	if sampleRate <= 0 {
		return sdktrace.NeverSample()
	}
	if sampleRate >= 1 {
		return sdktrace.AlwaysSample()
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRate))
}

func normalizedOTELSampleRate(sampleRate float64) float64 {
	if sampleRate < 0 {
		return 0
	}
	if sampleRate > 1 {
		return 1
	}
	return sampleRate
}

func buildOTLPHTTPOptions(rawEndpoint string, rawTracesEndpoint string, headers map[string]string) ([]otlptracehttp.Option, string, bool, error) {
	if tracesEndpoint := strings.TrimSpace(rawTracesEndpoint); tracesEndpoint != "" {
		opts, endpoint, insecure, err := buildOTLPHTTPTraceEndpointOptions(tracesEndpoint)
		if err != nil {
			return nil, "", false, err
		}
		return appendOTLPHeaders(opts, headers), endpoint, insecure, nil
	}

	endpoint := strings.TrimSpace(rawEndpoint)
	if endpoint == "" {
		return nil, "", false, errMissingOTELEndpoint
	}

	opts := []otlptracehttp.Option{}
	insecure := false

	if parsed, err := url.Parse(endpoint); err == nil && parsed.Scheme != "" {
		switch parsed.Scheme {
		case "http":
			insecure = true
			opts = append(opts, otlptracehttp.WithInsecure())
		case "https":
		default:
			return nil, "", false, errUnsupportedOTELEndpointScheme(parsed.Scheme)
		}

		opts = append(opts, otlptracehttp.WithEndpoint(parsed.Host))
		if parsed.Path != "" && parsed.Path != "/" {
			opts = append(opts, otlptracehttp.WithURLPath(path.Join(parsed.Path, "v1", "traces")))
		}
		return appendOTLPHeaders(opts, headers), endpoint, insecure, nil
	}

	opts = append(opts, otlptracehttp.WithEndpoint(endpoint))
	return appendOTLPHeaders(opts, headers), endpoint, insecure, nil
}

func buildOTLPHTTPTraceEndpointOptions(rawEndpoint string) ([]otlptracehttp.Option, string, bool, error) {
	parsed, err := url.Parse(rawEndpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, "", false, errInvalidOTELTraceEndpoint(rawEndpoint)
	}

	insecure := false
	switch parsed.Scheme {
	case "http":
		insecure = true
	case "https":
	default:
		return nil, "", false, errUnsupportedOTELEndpointScheme(parsed.Scheme)
	}

	return []otlptracehttp.Option{otlptracehttp.WithEndpointURL(rawEndpoint)}, rawEndpoint, insecure, nil
}

func appendOTLPHeaders(opts []otlptracehttp.Option, headers map[string]string) []otlptracehttp.Option {
	if len(headers) == 0 {
		return opts
	}
	copied := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		copied[key] = value
	}
	if len(copied) == 0 {
		return opts
	}
	return append(opts, otlptracehttp.WithHeaders(copied))
}

type unsupportedOTELEndpointSchemeError string

func (e unsupportedOTELEndpointSchemeError) Error() string {
	return "unsupported OpenTelemetry endpoint scheme: " + string(e)
}

func errUnsupportedOTELEndpointScheme(scheme string) error {
	return unsupportedOTELEndpointSchemeError(scheme)
}

type invalidOTELTraceEndpointError string

func (e invalidOTELTraceEndpointError) Error() string {
	return "invalid OpenTelemetry trace endpoint: " + string(e)
}

func errInvalidOTELTraceEndpoint(endpoint string) error {
	return invalidOTELTraceEndpointError(endpoint)
}

func registerOpenTelemetryLifecycle(lc fx.Lifecycle, resource *OpenTelemetryResource, log *zap.Logger) {
	if resource == nil || !resource.Enabled || resource.TracerProvider == nil {
		return
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			shutdownCtx, cancel := context.WithTimeout(ctx, defaultOTELShutdown)
			defer cancel()

			log.Info("Stopping OpenTelemetry tracer provider")
			return resource.TracerProvider.Shutdown(shutdownCtx)
		},
	})
}
