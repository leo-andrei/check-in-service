package config

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// InitTracerProvider sets up OpenTelemetry tracing with stdout or OTLP exporter
func InitTracerProvider(ctx context.Context, serviceName string) (*trace.TracerProvider, error) {
	exporterType := ""
	if Cfg != nil && Cfg.OpenTelemetry.Exporter != "" {
		exporterType = Cfg.OpenTelemetry.Exporter
	}
	var (
		exporter trace.SpanExporter
		err      error
	)
	if exporterType == "otlp" {
		endpoint := ""
		if Cfg != nil && Cfg.OpenTelemetry.OtlpEndpoint != "" {
			endpoint = Cfg.OpenTelemetry.OtlpEndpoint
		}
		exporter, err = otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithInsecure())
		if err != nil {
			return nil, err
		}
	} else {
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, err
		}
	}

	rsrc, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(rsrc),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}
