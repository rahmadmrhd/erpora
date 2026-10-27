package runtime

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"erpora/internal/platform/database"
	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/module"
)

const deprecatedNotesContract = `openapi: 3.1.0
info: { title: notes, version: '1' }
paths:
  /api/notes/old-items:
    get:
      operationId: listOldItems
      deprecated: true
      x-erp-permission: notes.items.read
      responses: { '200': { description: ok } }
  /api/notes/items:
    get:
      operationId: listItems
      x-erp-permission: notes.items.read
      parameters:
        - { name: legacy, in: query, deprecated: true, schema: { type: string } }
      responses: { '200': { description: ok } }
  /api/notes/fresh-items:
    get:
      operationId: listFreshItems
      x-erp-permission: notes.items.read
      responses: { '200': { description: ok } }
`

type contractMod struct {
	fakeModule
	doc []byte
}

func (c contractMod) OpenAPI() []byte { return c.doc }

type deprFixture struct {
	router *httpserver.Router
	reader *sdkmetric.ManualReader
}

func newDeprFixture(t *testing.T, authz Authorizer, withMeter bool) deprFixture {
	t.Helper()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	reg, err := registry.Build(registry.Options{PlatformVersion: "0.1.0", SDKVersion: "0.1.0"},
		contractMod{doc: []byte(deprecatedNotesContract), fakeModule: fakeModule{m: notesManifest(), register: func(r module.Registrar) error {
			r.Handle("GET", "/old-items", "notes.items.read", ok)
			r.Handle("GET", "/items", "notes.items.read", ok)
			r.Handle("GET", "/fresh-items", "notes.items.read", ok)
			return nil
		}}})
	if err != nil {
		t.Fatal(err)
	}
	log := observability.NewLogger(&discard{}, "text", slog.LevelDebug)
	router := httpserver.NewRouter(httpserver.Deps{
		Logger:       log,
		Tracer:       sdktrace.NewTracerProvider(),
		Resolver:     resolverFunc(func(*http.Request) (tenancy.TenantID, error) { return tenantA, nil }),
		Connector:    database.SingleTenantConnector{Default: tenantA},
		Authenticate: func(next http.Handler) http.Handler { return next },
	})
	snap := NewSnapshot()
	snap.Apply(tenantA, map[string]Status{"acme.notes": StatusEnabled}, 1)
	reader := sdkmetric.NewManualReader()
	var opts []MountOption
	if withMeter {
		opts = append(opts, WithMeter(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test")))
	}
	Mount(router, reg, snap, authz, log, opts...)
	return deprFixture{router: router, reader: reader}
}

type discard struct{}

func (*discard) Write(p []byte) (int, error) { return len(p), nil }

func (f deprFixture) get(target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// points mengembalikan titik data counter erp.api.deprecated_usage (atribut -> nilai).
func (f deprFixture) points(t *testing.T) []metricdata.DataPoint[int64] {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := f.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "erp.api.deprecated_usage" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("data = %T, want Sum[int64]", m.Data)
			}
			if m.Unit != "{request}" {
				t.Errorf("unit = %q, want {request}", m.Unit)
			}
			return sum.DataPoints
		}
	}
	return nil
}

func attrOf(dp metricdata.DataPoint[int64], key string) string {
	v, _ := dp.Attributes.Value(attribute.Key(key))
	return v.AsString()
}

func TestMount_DeprecatedOperationCountsAndHeader(t *testing.T) {
	f := newDeprFixture(t, allowAll, true)
	rec := f.get("/api/notes/old-items")
	if rec.Code != 200 || rec.Header().Get("Deprecation") != "true" {
		t.Fatalf("status %d Deprecation %q, want 200 dan true", rec.Code, rec.Header().Get("Deprecation"))
	}
	pts := f.points(t)
	if len(pts) != 1 || pts[0].Value != 1 {
		t.Fatalf("titik data = %+v, want satu titik bernilai 1", pts)
	}
	dp := pts[0]
	if attrOf(dp, "erp.api.deprecated.kind") != "operation" || attrOf(dp, "erp.module.key") != "acme.notes" ||
		attrOf(dp, "http.route") != "/api/notes/old-items" {
		t.Errorf("atribut = %v", dp.Attributes)
	}
	f.get("/api/notes/old-items")
	if pts := f.points(t); len(pts) != 1 || pts[0].Value != 2 {
		t.Errorf("setelah dua request: %+v, want nilai 2", pts)
	}
}

func TestMount_DeprecatedQueryParamCounted(t *testing.T) {
	f := newDeprFixture(t, allowAll, true)
	if rec := f.get("/api/notes/items"); rec.Header().Get("Deprecation") != "" {
		t.Error("operasi tidak deprecated tidak boleh memberi header Deprecation")
	}
	if pts := f.points(t); len(pts) != 0 {
		t.Fatalf("tanpa parameter deprecated tidak boleh ada titik data: %+v", pts)
	}
	f.get("/api/notes/items?legacy=1")
	pts := f.points(t)
	if len(pts) != 1 || attrOf(pts[0], "erp.api.deprecated.kind") != "parameter" || attrOf(pts[0], "erp.api.deprecated.name") != "legacy" {
		t.Fatalf("titik data = %+v, want kind=parameter name=legacy", pts)
	}
}

func TestMount_NonDeprecatedNoMetric(t *testing.T) {
	f := newDeprFixture(t, allowAll, true)
	rec := f.get("/api/notes/fresh-items")
	if rec.Code != 200 || rec.Header().Get("Deprecation") != "" {
		t.Errorf("status %d Deprecation %q", rec.Code, rec.Header().Get("Deprecation"))
	}
	if pts := f.points(t); len(pts) != 0 {
		t.Errorf("titik data = %+v, want kosong", pts)
	}
}

// Dipasang setelah permission: request yang ditolak tidak dihitung dan tidak membawa header.
func TestMount_DeprecatedAfterPermission(t *testing.T) {
	f := newDeprFixture(t, denyAll, true)
	rec := f.get("/api/notes/old-items")
	if rec.Code != 403 || rec.Header().Get("Deprecation") != "" {
		t.Fatalf("status %d Deprecation %q, want 403 tanpa header", rec.Code, rec.Header().Get("Deprecation"))
	}
	if pts := f.points(t); len(pts) != 0 {
		t.Errorf("request ditolak tidak boleh dihitung: %+v", pts)
	}
}

func TestMount_DeprecatedWithoutMeterOnlyHeader(t *testing.T) {
	f := newDeprFixture(t, allowAll, false)
	rec := f.get("/api/notes/old-items")
	if rec.Code != 200 || rec.Header().Get("Deprecation") != "true" {
		t.Errorf("status %d Deprecation %q, want 200 dan true", rec.Code, rec.Header().Get("Deprecation"))
	}
}
