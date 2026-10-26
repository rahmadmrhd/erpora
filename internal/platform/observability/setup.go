// Package observability menyiapkan OpenTelemetry (traces + metrics) dan logging slog yang sadar trace.
package observability

import (
	"context"
	"errors"
	"os"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Options adalah identitas layanan untuk resource OTel.
type Options struct{ ServiceVersion, Environment string }

// Providers adalah provider yang dipakai aplikasi. Resource nil bila tidak Enabled.
type Providers struct {
	Tracer   trace.TracerProvider
	Meter    metric.MeterProvider
	Enabled  bool
	Resource *resource.Resource
}

// exporterFactory membuat exporter span dan metrik; dapat diganti pada test (newExporters).
type exporterFactory struct {
	span   func(context.Context) (sdktrace.SpanExporter, error)
	metric func(context.Context) (sdkmetric.Exporter, error)
}

// newExporters memilih exporter OTLP gRPC atau HTTP. Variabel agar test dapat menyuntikkan kegagalan.
var newExporters = func(grpc bool) exporterFactory {
	if grpc {
		return exporterFactory{
			span:   func(ctx context.Context) (sdktrace.SpanExporter, error) { return otlptracegrpc.New(ctx) },
			metric: func(ctx context.Context) (sdkmetric.Exporter, error) { return otlpmetricgrpc.New(ctx) },
		}
	}
	return exporterFactory{
		span:   func(ctx context.Context) (sdktrace.SpanExporter, error) { return otlptracehttp.New(ctx) },
		metric: func(ctx context.Context) (sdkmetric.Exporter, error) { return otlpmetrichttp.New(ctx) },
	}
}

// Setup menyiapkan OTel. Tanpa OTEL_EXPORTER_OTLP_ENDPOINT: TracerProvider SDK tanpa exporter (span
// tetap punya trace_id/span_id valid untuk log dan envelope error, FR-17.1) dan MeterProvider noop;
// tidak ada koneksi keluar. Propagator TraceContext+Baggage selalu dipasang global.
func Setup(ctx context.Context, o Options) (Providers, func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		tp := sdktrace.NewTracerProvider() // tanpa exporter/processor: ID valid, tidak ada ekspor
		return Providers{Tracer: tp, Meter: metricnoop.NewMeterProvider()}, tp.Shutdown, nil
	}

	name := os.Getenv("OTEL_SERVICE_NAME")
	if name == "" {
		name = "erpora"
	}
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		attribute.String("service.name", name),
		attribute.String("service.version", o.ServiceVersion),
		attribute.String("deployment.environment", o.Environment),
	))
	if err != nil {
		return Providers{}, nil, err
	}

	f := newExporters(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL") == "grpc")
	spanExp, err := f.span(ctx)
	if err != nil {
		return Providers{}, nil, err
	}
	metExp, err := f.metric(ctx)
	if err != nil {
		_ = spanExp.Shutdown(ctx) // exporter metrik gagal: jangan biarkan exporter span bocor
		return Providers{}, nil, err
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(spanExp))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metExp)))
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)

	shutdown := func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx))
	}
	if err := runtime.Start(runtime.WithMeterProvider(mp)); err != nil {
		_ = shutdown(ctx)
		return Providers{}, nil, err
	}
	return Providers{Tracer: tp, Meter: mp, Enabled: true, Resource: res}, shutdown, nil
}
