package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpora/internal/platform/audit"
	"erpora/internal/platform/authorization"
	"erpora/internal/platform/database"
	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/authz"
	"erpora/sdk/authz/authztest"
	"erpora/sdk/export"
	"erpora/sdk/httpx"
	"erpora/sdk/module"
)

// limitsRouter memasang satu modul uji dengan route yang didaftarkan register, status modul status.
func limitsRouter(t *testing.T, status Status, register func(r module.Registrar), opts ...MountOption) *httpserver.Router {
	t.Helper()
	reg, err := registry.Build(registry.Options{
		PlatformVersion: "0.1.0", SDKVersion: "0.1.0",
	}, fakeModule{m: notesManifest(), register: func(r module.Registrar) error {
		register(r)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	router := httpserver.NewRouter(httpserver.Deps{
		Logger:       observability.NewLogger(io.Discard, "text", slog.LevelDebug),
		Resolver:     resolverFunc(func(*http.Request) (tenancy.TenantID, error) { return tenantA, nil }),
		Connector:    database.SingleTenantConnector{Default: tenantA},
		Authenticate: func(next http.Handler) http.Handler { return next },
	})
	snap := NewSnapshot()
	snap.Apply(tenantA, map[string]Status{"acme.notes": status}, 1)
	Mount(router, reg, snap, allowAll, nil, opts...)
	return router
}

var okH = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

func TestMount_NoStoreByClassification(t *testing.T) {
	cases := []struct {
		name  string
		opts  []module.RouteOption
		store bool
	}{
		{"none", nil, false},
		{"public", []module.RouteOption{module.Classify(module.ClassPublic)}, false},
		{"internal", []module.RouteOption{module.Classify(module.ClassInternal)}, false},
		{"confidential", []module.RouteOption{module.Classify(module.ClassConfidential)}, true},
		{"highly_confidential", []module.RouteOption{module.Classify(module.ClassHighlyConfidential)}, true},
		{"repeated_takes_highest", []module.RouteOption{module.Classify(module.ClassConfidential), module.Classify(module.ClassPublic)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := limitsRouter(t, StatusEnabled, func(r module.Registrar) {
				r.Handle("GET", "/items", "notes.items.read", okH, tc.opts...)
				r.Public("GET", "/status", okH, tc.opts...)
			})
			for _, target := range []string{"/api/notes/items", "/api/notes/status"} {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
				got := rec.Header().Get("Cache-Control")
				if tc.store && got != "no-store" || !tc.store && got != "" {
					t.Errorf("%s Cache-Control = %q, store=%v", target, got, tc.store)
				}
			}
		})
	}
}

func TestMount_NoStoreAlsoOnHandlerError(t *testing.T) {
	router := limitsRouter(t, StatusEnabled, func(r module.Registrar) {
		r.Handle("GET", "/items", "notes.items.read", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			httpx.WriteError(w, req, 422, "validation_failed", "x", nil)
		}), module.Classify(module.ClassConfidential))
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/notes/items", nil))
	if rec.Code != 422 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("status=%d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestMount_BodyLimitEnforcedWithoutDecode(t *testing.T) {
	var readErr error
	router := limitsRouter(t, StatusEnabled, func(r module.Registrar) {
		r.Handle("POST", "/items", "notes.items.read", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			_, readErr = io.ReadAll(req.Body)
		}), module.MaxBodyBytes(16))
	})
	req := httptest.NewRequest("POST", "/api/notes/items", strings.NewReader(strings.Repeat("x", 17)))
	router.ServeHTTP(httptest.NewRecorder(), req)
	var mbe *http.MaxBytesError
	if !errors.As(readErr, &mbe) {
		t.Errorf("readErr = %v, want *http.MaxBytesError", readErr)
	}
}

type decodeDTO struct {
	Title string `json:"title"`
}

func TestMount_DecodeUsesRouteLimit(t *testing.T) {
	decodeH := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := httpx.DecodeJSON[decodeDTO](w, r); err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	router := limitsRouter(t, StatusEnabled, func(r module.Registrar) {
		r.Handle("POST", "/big", "notes.items.read", decodeH, module.MaxBodyBytes(2<<20))
		r.Handle("POST", "/small", "notes.items.read", decodeH)
	})
	post := func(path string, size int) *httptest.ResponseRecorder {
		body := `{"title":"` + strings.Repeat("a", size) + `"}`
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := post("/api/notes/big", 3<<19); rec.Code != 200 { // 1.5 MiB
		t.Errorf("big = %d %s", rec.Code, rec.Body.String())
	}
	rec := post("/api/notes/small", 1<<20)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"code":"body_too_large"`) {
		t.Errorf("small = %d %s", rec.Code, rec.Body.String())
	}
}

func TestMount_GuardBeforeLimits(t *testing.T) {
	called := false
	router := limitsRouter(t, StatusDisabled, func(r module.Registrar) {
		r.Handle("GET", "/items", "notes.items.read", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			called = true
		}), module.Classify(module.ClassConfidential))
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/notes/items", nil))
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "module_disabled") || called {
		t.Errorf("status=%d called=%v body=%s", rec.Code, called, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "" {
		t.Errorf("guard menjawab sebelum batas request; Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

type recAuditor struct{ events []export.Event }

func (a *recAuditor) RecordExport(_ context.Context, e export.Event) error {
	a.events = append(a.events, e)
	return nil
}

func TestMount_InstallsExportServices(t *testing.T) {
	var gotMasker, gotAuditor bool
	probe := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMasker, gotAuditor = authz.HasFieldMasker(r.Context()), export.HasAuditor(r.Context())
	})
	register := func(r module.Registrar) { r.Handle("GET", "/items", "notes.items.read", probe) }
	get := func(opts ...MountOption) {
		gotMasker, gotAuditor = false, false
		router := limitsRouter(t, StatusEnabled, register, opts...)
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/notes/items", nil))
	}
	get()
	if gotMasker || gotAuditor {
		t.Errorf("tanpa opsi: masker=%v auditor=%v", gotMasker, gotAuditor)
	}
	get(WithServices(Services{Masker: authztest.AllowAllFields, Auditor: &recAuditor{}}))
	if !gotMasker || !gotAuditor {
		t.Errorf("dengan opsi: masker=%v auditor=%v", gotMasker, gotAuditor)
	}
}

func TestExport_EndToEndThroughMount(t *testing.T) {
	var logs bytes.Buffer
	log := observability.NewLogger(&logs, "json", slog.LevelDebug)
	svc := Services{Masker: authorization.NewStubFieldMasker("allow", log), Auditor: audit.NewExportLog(log)}
	router := limitsRouter(t, StatusEnabled, func(r module.Registrar) {
		r.Handle("GET", "/export", "notes.items.read", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			rows := func(yield func(export.Row, error) bool) { yield(export.Row{"id": "1"}, nil) }
			spec := export.Spec{Resource: "notes.item", Format: export.CSV, Filename: "items", Columns: []export.Column{
				{Key: "id", Header: "ID", Classification: module.ClassConfidential},
			}}
			if err := export.Serve(w, req, spec, rows); err != nil {
				httpx.RespondError(w, req, err)
			}
		}), module.Classify(module.ClassConfidential))
	}, WithServices(svc))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/notes/export", nil))
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Body.String(), "ID\r\n1\r\n") {
		t.Fatalf("%d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}
	if n := strings.Count(logs.String(), `"msg":"ekspor data"`); n != 1 {
		t.Errorf("log ekspor = %d: %s", n, logs.String())
	}
}

// Codex PR#7 C5: handler tidak dapat menimpa atau menghapus Cache-Control: no-store pada route confidential ke atas.
func TestMount_NoStoreCannotBeOverriddenByHandler(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"overwrite": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "public, max-age=3600")
			w.WriteHeader(http.StatusOK)
		},
		"delete": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Del("Cache-Control")
			_, _ = w.Write([]byte("x"))
		},
		"overwrite_then_write": func(w http.ResponseWriter, _ *http.Request) {
			w.Header()["Cache-Control"] = []string{"max-age=60"}
			_, _ = w.Write([]byte("x"))
		},
		"delete_without_write": func(w http.ResponseWriter, _ *http.Request) { w.Header().Del("Cache-Control") },
		"flush": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "public")
			w.(http.Flusher).Flush()
		},
		"controller_flush": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "public")
			_ = http.NewResponseController(w).Flush()
		},
		"error_helper": func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Cache-Control", "public")
			httpx.WriteError(w, req, 422, "validation_failed", "x", nil)
		},
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			router := limitsRouter(t, StatusEnabled, func(r module.Registrar) {
				r.Handle("GET", "/items", "notes.items.read", h, module.Classify(module.ClassConfidential))
			})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/notes/items", nil))
			if got := rec.Header().Values("Cache-Control"); len(got) != 1 || got[0] != "no-store" {
				t.Errorf("Cache-Control = %q, want [no-store]", got)
			}
		})
	}
}
