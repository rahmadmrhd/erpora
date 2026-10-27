package lifecycle

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

const telemetryScope = "erpora/internal/platform/modules/lifecycle"

// telemetry membungkus tracer dan instrumen metrik operasi lifecycle (FR-17.4).
type telemetry struct {
	tracer trace.Tracer
	ops    metric.Int64Counter
	dur    metric.Float64Histogram
}

func newTelemetry(tp trace.TracerProvider, mp metric.MeterProvider) telemetry {
	if tp == nil {
		tp = tracenoop.NewTracerProvider()
	}
	if mp == nil {
		mp = metricnoop.NewMeterProvider()
	}
	m := mp.Meter(telemetryScope)
	// Error pembuatan instrumen hanya terjadi untuk nama tidak valid; nama di sini konstan.
	ops, _ := m.Int64Counter("erp.module.operations",
		metric.WithDescription("Jumlah operasi lifecycle modul menurut operasi dan hasil"))
	dur, _ := m.Float64Histogram("erp.module.operation.duration",
		metric.WithDescription("Durasi operasi lifecycle modul"), metric.WithUnit("s"))
	return telemetry{tracer: tp.Tracer(telemetryScope), ops: ops, dur: dur}
}

// record mencatat satu operasi selesai; result "succeeded" atau "failed".
func (t telemetry) record(ctx context.Context, op Op, result string, d time.Duration) {
	attrs := metric.WithAttributes(attribute.String("operation", string(op)), attribute.String("result", result))
	t.ops.Add(ctx, 1, attrs)
	t.dur.Record(ctx, d.Seconds(), attrs)
}
