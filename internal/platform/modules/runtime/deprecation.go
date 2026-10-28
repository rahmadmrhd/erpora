package runtime

import (
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"erpora/internal/platform/modules/registry"
)

// deprecatedUsageMetric adalah counter pemakaian operasi/parameter deprecated (PRD-000 FR-23.3, api-guidelines §14.3).
const deprecatedUsageMetric = "erp.api.deprecated_usage"

// WithMeter memasang meter untuk metrik erp.api.deprecated_usage. Tanpa meter, middleware deprecation hanya memasang
// header Deprecation.
func WithMeter(m metric.Meter) MountOption {
	return func(c *mountConfig) { c.meter = m }
}

// newDeprecatedCounter membuat counter; nil bila meter nil atau pembuatan gagal (metrik tidak boleh menjatuhkan request).
func newDeprecatedCounter(m metric.Meter) metric.Int64Counter {
	if m == nil {
		return nil
	}
	c, err := m.Int64Counter(deprecatedUsageMetric,
		metric.WithUnit("{request}"),
		metric.WithDescription("Request ke operasi atau parameter query yang ditandai deprecated di kontrak OpenAPI"))
	if err != nil {
		return nil
	}
	return c
}

// deprecationMW: untuk route deprecated memasang header "Deprecation: true" dan menambah counter (erp.module.key,
// http.route, erp.api.deprecated.kind="operation"); untuk setiap parameter query deprecated yang dikirim klien menambah
// counter dengan kind="parameter" dan erp.api.deprecated.name. Dipasang setelah permission, sebelum batas request,
// sehingga request yang ditolak tidak dihitung. c boleh nil.
func deprecationMW(rt registry.RouteInfo, c metric.Int64Counter, next http.Handler) http.Handler {
	if !rt.Deprecated && len(rt.DeprecatedParams) == 0 {
		return next
	}
	base := []attribute.KeyValue{attribute.String("erp.module.key", rt.Module), attribute.String("http.route", rt.Pattern)}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rt.Deprecated {
			w.Header().Set("Deprecation", "true")
			if c != nil {
				c.Add(r.Context(), 1, metric.WithAttributes(append(base[:2:2], attribute.String("erp.api.deprecated.kind", "operation"))...))
			}
		}
		if c != nil && len(rt.DeprecatedParams) > 0 {
			q := r.URL.Query()
			for _, p := range rt.DeprecatedParams {
				if _, sent := q[p]; sent {
					c.Add(r.Context(), 1, metric.WithAttributes(append(base[:2:2],
						attribute.String("erp.api.deprecated.kind", "parameter"),
						attribute.String("erp.api.deprecated.name", p))...))
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
