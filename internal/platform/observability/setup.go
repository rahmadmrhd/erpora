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
	tracenoop "go.opentelemetry.io/otel/trace/noop"
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

// Setup menyiapkan OTel. Tanpa OTEL_EXPORTER_OTLP_ENDPOINT: provider noop, tanpa exporter dan
// tanpa koneksi keluar. Propagator TraceContext+Baggage selalu dipasang global.
func Setup(ctx context.Context, o Options) (Providers, func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return Providers{Tracer: tracenoop.NewTracerProvider(), Meter: metricnoop.NewMeterProvider()},
			func(context.Context) error { return nil }, nil
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

	var (
		spanExp sdktrace.SpanExporter
		metExp  sdkmetric.Exporter
	)
	if os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL") == "grpc" {
		if spanExp, err = otlptracegrpc.New(ctx); err == nil {
			metExp, err = otlpmetricgrpc.New(ctx)
		}
	} else {
		if spanExp, err = otlptracehttp.New(ctx); err == nil {
			metExp, err = otlpmetrichttp.New(ctx)
		}
	}
	if err != nil {
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
