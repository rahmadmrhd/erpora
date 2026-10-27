package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/authorization"
	"erpora/internal/platform/database"
	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/modules/lifecycle"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/resolver"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/module"
)

var (
	testTenant = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	opID       = uuid.MustParse("0190a1b2-0000-7000-8000-000000000001")
	fixedTime  = time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
)

// fakeBE memenuhi Backend; operasi dan event dibatasi per tenant seperti store nyata.
type fakeBE struct {
	mu       sync.Mutex
	mods     []lifecycle.ModuleState
	rev      int64
	execFn   func(ctx context.Context, req lifecycle.Request) ([]lifecycle.Result, error)
	retryFn  func(ctx context.Context, key, actor string, onStarted func(uuid.UUID)) (lifecycle.Result, error)
	ops      map[uuid.UUID]store.Operation
	events   []store.Event
	execReqs []lifecycle.Request
	retried  []string
}

func newFakeBE() *fakeBE {
	return &fakeBE{
		rev: 7,
		mods: []lifecycle.ModuleState{
			{Key: "acme.example", Slug: "example", Name: "Example", BinaryVersion: "1.0.0", InstalledVersion: "1.0.0", Status: runtime.StatusEnabled, Stored: "enabled", Compatible: true,
				Dependencies: []module.Dependency{{Module: "platform.core", Version: "^0.1"}}, Dependents: []string{"acme.example-addon"},
				LastOperation: &store.Operation{ID: opID}},
			{Key: "acme.example-addon", Slug: "example-addon", Name: "Addon", BinaryVersion: "1.0.0", Status: runtime.StatusAvailable, Stored: "available", Compatible: true},
			{Key: "acme.third", Slug: "third", Name: "Third", BinaryVersion: "0.1.0", Status: runtime.StatusAvailable, Stored: "available", Compatible: false,
				Incompatible: []resolver.Issue{{Code: resolver.CodePlatformIncompatible, Module: "acme.third"}}},
		},
		ops: map[uuid.UUID]store.Operation{
			opID: {ID: opID, TenantID: testTenant, ModuleKey: "acme.example", Operation: "enable", Status: "succeeded", Actor: "api:dev", StartedAt: fixedTime,
				Steps: []store.Step{{Name: "health", Status: "succeeded", StartedAt: fixedTime, DurationMS: 3}}},
		},
	}
}

func (f *fakeBE) Modules(context.Context, tenancy.TenantID) ([]lifecycle.ModuleState, int64, error) {
	return f.mods, f.rev, nil
}

func (f *fakeBE) Module(_ context.Context, _ tenancy.TenantID, key string) (lifecycle.ModuleState, int64, error) {
	for _, m := range f.mods {
		if m.Key == key {
			return m, f.rev, nil
		}
	}
	return lifecycle.ModuleState{}, 0, &lifecycle.Error{Code: lifecycle.CodeModuleNotFound, Module: key}
}

func (f *fakeBE) History(_ context.Context, t tenancy.TenantID, key string, page, pageSize int) ([]store.Event, int64, error) {
	if key == "acme.nope" {
		return nil, 0, &lifecycle.Error{Code: lifecycle.CodeModuleNotFound, Module: key}
	}
	var out []store.Event
	for _, e := range f.events {
		if e.TenantID == t && e.ModuleKey == key {
			out = append(out, e)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeBE) Operation(_ context.Context, t tenancy.TenantID, id uuid.UUID) (store.Operation, error) {
	op, ok := f.ops[id]
	if !ok || op.TenantID != t {
		return store.Operation{}, &lifecycle.Error{Code: lifecycle.CodeOperationNotFound}
	}
	return op, nil
}

func (f *fakeBE) Execute(ctx context.Context, req lifecycle.Request) ([]lifecycle.Result, error) {
	// Lifecycle sungguhan membandingkan If-Match di dalam lock (B-5); fake meniru kontraknya.
	if req.IfMatch != "" && !lifecycle.MatchIfMatch(req.IfMatch, f.rev) {
		return nil, &lifecycle.Error{Code: lifecycle.CodeVersionMismatch, Revision: f.rev}
	}
	f.mu.Lock()
	f.execReqs = append(f.execReqs, req)
	f.mu.Unlock()
	if f.execFn != nil {
		return f.execFn(ctx, req)
	}
	return []lifecycle.Result{{Module: req.Key, Op: req.Op, From: runtime.StatusInstalled, To: runtime.StatusEnabled, Status: "succeeded", OperationID: opID}}, nil
}

func (f *fakeBE) RetryModule(ctx context.Context, _ tenancy.TenantID, key, actor string, onStarted func(uuid.UUID), _ string) (lifecycle.Result, error) {
	f.retried = append(f.retried, key+"|"+actor)
	if f.retryFn != nil {
		return f.retryFn(ctx, key, actor, onStarted)
	}
	return lifecycle.Result{Module: key, Op: lifecycle.OpInstall, Status: "succeeded", OperationID: opID}, nil
}

type resolverFunc func(*http.Request) (tenancy.TenantID, error)

func (f resolverFunc) Resolve(r *http.Request) (tenancy.TenantID, error) { return f(r) }

type fixtureAPI struct {
	api *API
	be  *fakeBE
	h   http.Handler
}

func setup(t *testing.T, authz runtime.Authorizer, opts ...runtime.MountOption) fixtureAPI {
	t.Helper()
	api := New()
	be := newFakeBE()
	api.Bind(be)
	reg, err := registry.Build(registry.Options{
		PlatformVersion: "0.1.0", SDKVersion: "0.1.0", Platform: api.Register, PlatformCapabilities: Capabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpserver.NewRouter(httpserver.Deps{
		Logger:    log,
		Resolver:  resolverFunc(func(*http.Request) (tenancy.TenantID, error) { return testTenant, nil }),
		Connector: database.SingleTenantConnector{Default: testTenant},
	})
	runtime.Mount(router, reg, runtime.NewSnapshot(), authz, log, opts...)
	return fixtureAPI{api: api, be: be, h: router}
}

type allow struct{}

func (allow) Allow(context.Context, string) bool { return true }

func (f fixtureAPI) do(method, target, body string, hdr map[string]string) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

type errBody struct {
	Error struct {
		Code    string
		Message string
		Details json.RawMessage
	}
}

func errOf(t *testing.T, rec *httptest.ResponseRecorder) errBody {
	t.Helper()
	var e errBody
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return e
}

func TestList_EnvelopeAndPagination(t *testing.T) {
	f := setup(t, allow{})
	rec := f.do("GET", "/api/platform/modules?page=2&pageSize=2", "", nil)
	if rec.Code != 200 || rec.Header().Get("ETag") != `"7"` {
		t.Fatalf("%d etag=%q %s", rec.Code, rec.Header().Get("ETag"), rec.Body)
	}
	var body struct {
		Data []map[string]any
		Page map[string]any
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0]["key"] != "acme.third" {
		t.Errorf("data = %+v", body.Data)
	}
	if body.Page["page"] != float64(2) || body.Page["pageSize"] != float64(2) || body.Page["total"] != float64(3) || body.Page["hasMore"] != false {
		t.Errorf("page = %+v", body.Page)
	}
	rec = f.do("GET", "/api/platform/modules", "", nil)
	var all struct{ Data []map[string]any }
	_ = json.Unmarshal(rec.Body.Bytes(), &all)
	if len(all.Data) != 3 {
		t.Fatalf("default page = %d item", len(all.Data))
	}
	first := all.Data[0]
	for _, k := range []string{"key", "slug", "name", "binaryVersion", "installedVersion", "status", "required", "compatible", "incompatibleCodes", "dependencies", "dependents", "lastOperationId"} {
		if _, ok := first[k]; !ok {
			t.Errorf("view tanpa field %s: %v", k, first)
		}
	}
	if all.Data[1]["installedVersion"] != nil || all.Data[2]["incompatibleCodes"].([]any)[0] != "platform_incompatible" {
		t.Errorf("views = %+v", all.Data)
	}
	if rec := f.do("GET", "/api/platform/modules?pageSize=500", "", nil); rec.Code != 400 || errOf(t, rec).Error.Code != "bad_request" {
		t.Errorf("pageSize 500: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("GET", "/api/platform/modules?page=0", "", nil); rec.Code != 400 {
		t.Errorf("page 0: %d", rec.Code)
	}
	if rec := f.do("GET", "/api/platform/modules", "", map[string]string{"If-None-Match": `"7"`}); rec.Code != 304 {
		t.Errorf("If-None-Match: %d", rec.Code)
	}
}

func TestGetModule(t *testing.T) {
	f := setup(t, allow{})
	rec := f.do("GET", "/api/platform/modules/acme.example", "", nil)
	if rec.Code != 200 || rec.Header().Get("ETag") != `"7"` || !strings.Contains(rec.Body.String(), `"lastOperationId":"`+opID.String()+`"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec = f.do("GET", "/api/platform/modules/acme.nope", "", nil)
	if rec.Code != 404 || errOf(t, rec).Error.Code != "not_found" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestHistoryAndOperationShareRoute(t *testing.T) {
	f := setup(t, allow{})
	other := tenancy.TenantID(uuid.New())
	f.be.events = []store.Event{
		{ID: uuid.New(), TenantID: testTenant, ModuleKey: "acme.example", Event: "enabled", Actor: "api:dev", OperationID: opID, OccurredAt: fixedTime},
		{ID: uuid.New(), TenantID: other, ModuleKey: "acme.example", Event: "disabled", Actor: "x", OccurredAt: fixedTime},
	}
	rec := f.do("GET", "/api/platform/modules/acme.example/history", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"event":"enabled"`) || strings.Contains(rec.Body.String(), `"disabled"`) {
		t.Fatalf("history: %d %s", rec.Code, rec.Body)
	}
	rec = f.do("GET", "/api/platform/modules/operations/"+opID.String(), "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"operation":"enable"`) || !strings.Contains(rec.Body.String(), `"durationMs":3`) {
		t.Fatalf("operation: %d %s", rec.Code, rec.Body)
	}
	// {id} bukan UUID -> 400, termasuk path yang bentrok dengan history ("operations/history").
	for _, p := range []string{"operations/not-a-uuid", "operations/history"} {
		if rec := f.do("GET", "/api/platform/modules/"+p, "", nil); rec.Code != 400 || errOf(t, rec).Error.Code != "bad_request" {
			t.Errorf("%s: %d %s", p, rec.Code, rec.Body)
		}
	}
	if rec := f.do("GET", "/api/platform/modules/acme.example/unknown", "", nil); rec.Code != 404 {
		t.Errorf("sub tak dikenal: %d", rec.Code)
	}
	if rec := f.do("GET", "/api/platform/modules/acme.nope/history", "", nil); rec.Code != 404 {
		t.Errorf("history key tak dikenal: %d", rec.Code)
	}
}

// FR-119: operasi milik tenant lain -> 404 not_found (bukan 403).
func TestIDOR_OperationOtherTenant(t *testing.T) {
	f := setup(t, allow{})
	foreign := uuid.New()
	f.be.ops[foreign] = store.Operation{ID: foreign, TenantID: tenancy.TenantID(uuid.New()), ModuleKey: "acme.example", Operation: "enable", Status: "succeeded"}
	rec := f.do("GET", "/api/platform/modules/operations/"+foreign.String(), "", nil)
	if rec.Code != 404 || errOf(t, rec).Error.Code != "not_found" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := f.do("GET", "/api/platform/modules/operations/"+uuid.NewString(), "", nil); rec.Code != 404 {
		t.Errorf("operasi tidak ada: %d", rec.Code)
	}
}

func TestAction_ErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		op         string
		wantStatus int
		wantCode   string
		wantDetail []string // kode detail
		wantMsg    string
	}{
		{"invalid_transition", &lifecycle.Error{Code: lifecycle.CodeInvalidTransition, Message: "x"}, "enable", 409, "invalid_state", []string{"invalid_transition"}, ""},
		{"operation_in_progress", &lifecycle.Error{Code: lifecycle.CodeOperationInProgress, Message: "x"}, "enable", 409, "request_in_progress", []string{"operation_in_progress"}, ""},
		{"dependents_active", &lifecycle.Error{Code: lifecycle.CodeDependentsActive, Module: "acme.example", Message: "masih enabled: acme.example-addon", Related: []string{"acme.example-addon"}}, "disable", 422, "business_rule_violated", []string{"dependents_active"}, "acme.example-addon"},
		{"module_required", &lifecycle.Error{Code: lifecycle.CodeModuleRequired, Message: "wajib"}, "disable", 422, "business_rule_violated", []string{"module_required"}, ""},
		{"dependency_not_ready", &lifecycle.Error{Code: lifecycle.CodeDependencyNotReady, Message: "x", Related: []string{"acme.example"}}, "enable", 422, "business_rule_violated", []string{"dependency_not_ready"}, "acme.example"},
		{"downgrade", &lifecycle.Error{Code: lifecycle.CodeDowngradeNotSupported, Message: "x"}, "upgrade", 422, "business_rule_violated", []string{"downgrade_not_supported"}, ""},
		{"module_incompatible 2 issue", &lifecycle.Error{Code: lifecycle.CodeModuleIncompatible, Message: "x", Issues: []resolver.Issue{
			{Code: resolver.CodePlatformIncompatible, Module: "acme.example", Constraint: "^9.0", Found: "0.1.0"},
			{Code: resolver.CodeSDKIncompatible, Module: "acme.example", Constraint: "^9.0", Found: "0.1.0"},
		}}, "install", 422, "business_rule_violated", []string{"module_incompatible", "module_incompatible"}, ""},
		{"module_not_found", &lifecycle.Error{Code: lifecycle.CodeModuleNotFound, Module: "acme.nope"}, "enable", 404, "not_found", nil, ""},
		{"operation_not_found", &lifecycle.Error{Code: lifecycle.CodeOperationNotFound}, "retry", 404, "not_found", nil, ""},
		{"error internal", errors.New("pq: secret detail"), "enable", 500, "internal_error", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, allow{})
			f.be.execFn = func(context.Context, lifecycle.Request) ([]lifecycle.Result, error) { return nil, tc.err }
			f.be.retryFn = func(context.Context, string, string, func(uuid.UUID)) (lifecycle.Result, error) {
				return lifecycle.Result{}, tc.err
			}
			rec := f.do("POST", "/api/platform/modules/acme.example/"+tc.op, "", nil)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body)
			}
			e := errOf(t, rec)
			if e.Error.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", e.Error.Code, tc.wantCode)
			}
			var got []string
			var ds []map[string]any
			_ = json.Unmarshal(e.Error.Details, &ds)
			for _, d := range ds {
				got = append(got, d["code"].(string))
			}
			if strings.Join(got, ",") != strings.Join(tc.wantDetail, ",") {
				t.Errorf("details = %v, want %v", got, tc.wantDetail)
			}
			if tc.wantMsg != "" && !strings.Contains(rec.Body.String(), tc.wantMsg) {
				t.Errorf("respons tidak menyebut %q: %s", tc.wantMsg, rec.Body)
			}
			if tc.wantStatus == 500 && strings.Contains(rec.Body.String(), "secret") {
				t.Errorf("detail internal bocor: %s", rec.Body)
			}
		})
	}
}

func TestAction_UnknownOperationAndNotBound(t *testing.T) {
	f := setup(t, allow{})
	rec := f.do("POST", "/api/platform/modules/acme.example/explode", "", nil)
	if rec.Code != 404 || errOf(t, rec).Error.Code != "not_found" || len(f.be.execReqs) != 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}

	api := New() // belum di-bind
	reg, _ := registry.Build(registry.Options{PlatformVersion: "0.1.0", SDKVersion: "0.1.0", Platform: api.Register, PlatformCapabilities: Capabilities})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpserver.NewRouter(httpserver.Deps{Logger: log,
		Resolver:  resolverFunc(func(*http.Request) (tenancy.TenantID, error) { return testTenant, nil }),
		Connector: database.SingleTenantConnector{Default: testTenant}})
	runtime.Mount(router, reg, runtime.NewSnapshot(), allow{}, log)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest("GET", "/api/platform/modules", nil))
	if rr.Code != 503 || errOf(t, rr).Error.Code != "not_ready" {
		t.Fatalf("belum di-bind: %d %s", rr.Code, rr.Body)
	}
}

func TestAction_Success(t *testing.T) {
	f := setup(t, allow{})
	rec := f.do("POST", "/api/platform/modules/acme.example/install", `{"withDeps":true,"enable":true}`, nil)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if len(f.be.execReqs) != 1 {
		t.Fatal("Execute tidak dipanggil")
	}
	r := f.be.execReqs[0]
	if r.Op != lifecycle.OpInstall || r.Key != "acme.example" || !r.WithDeps || !r.Enable || r.All || r.Actor != "api:dev" || r.Tenant != testTenant {
		t.Errorf("request = %+v", r)
	}
	var body struct {
		Data struct {
			Module     map[string]any
			Operations []map[string]any
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Module["key"] != "acme.example" || len(body.Data.Operations) != 1 || body.Data.Operations[0]["id"] != opID.String() {
		t.Errorf("body = %s", rec.Body)
	}
	// retry memakai operasi terakhir modul dengan actor api:dev.
	if rec := f.do("POST", "/api/platform/modules/acme.example/retry", "", nil); rec.Code != 200 || len(f.be.retried) != 1 || f.be.retried[0] != "acme.example|api:dev" {
		t.Fatalf("retry: %d %s %v", rec.Code, rec.Body, f.be.retried)
	}
}

func TestAction_FailedOperationReturns200(t *testing.T) {
	f := setup(t, allow{})
	failedID := uuid.New()
	f.be.ops[failedID] = store.Operation{ID: failedID, TenantID: testTenant, ModuleKey: "acme.example", Operation: "enable", Status: "failed", Actor: "api:dev", StartedAt: fixedTime,
		Error: &store.OpError{Code: "health_failed", Message: "sakit", Step: "health"}}
	f.be.execFn = func(context.Context, lifecycle.Request) ([]lifecycle.Result, error) {
		return []lifecycle.Result{{Module: "acme.example", Op: lifecycle.OpEnable, Status: "failed", OperationID: failedID, Error: &store.OpError{Code: "health_failed", Message: "sakit", Step: "health"}}}, nil
	}
	rec := f.do("POST", "/api/platform/modules/acme.example/enable", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"failed"`) || !strings.Contains(rec.Body.String(), `"failure":{"code":"health_failed"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestAction_NoopEmptyOperations(t *testing.T) {
	f := setup(t, allow{})
	f.be.execFn = func(context.Context, lifecycle.Request) ([]lifecycle.Result, error) {
		return []lifecycle.Result{{Module: "acme.example", Op: lifecycle.OpEnable, From: runtime.StatusEnabled, To: runtime.StatusEnabled, Status: "noop"}}, nil
	}
	rec := f.do("POST", "/api/platform/modules/acme.example/enable", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"operations":[]`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestAction_SlowOperationReturns202(t *testing.T) {
	f := setup(t, allow{})
	f.api.syncWait = 30 * time.Millisecond
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	f.be.execFn = func(_ context.Context, req lifecycle.Request) ([]lifecycle.Result, error) {
		req.OnStarted(opID)
		<-release
		return nil, nil
	}
	rec := f.do("POST", "/api/platform/modules/acme.example/enable", "", nil)
	if rec.Code != 202 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/platform/modules/operations/"+opID.String() {
		t.Errorf("Location = %q", loc)
	}
	if !strings.Contains(rec.Body.String(), `"operationId":"`+opID.String()+`"`) || !strings.Contains(rec.Body.String(), `"status":"running"`) {
		t.Errorf("body = %s", rec.Body)
	}
}

// B-6 (Q32): rangkaian beberapa operasi (mis. install --with-deps) melaporkan SEMUA ID operasi lewat field aditif
// operationIds, baik pada 202 (yang sudah dimulai) maupun 200 (seluruhnya); Location tetap ke operasi pertama.
func TestAction_MultiOperationIDs(t *testing.T) {
	op2 := uuid.MustParse("0190a1b2-0000-7000-8000-0000000000bb")
	t.Run("202 memuat semua ID yang sudah dimulai", func(t *testing.T) {
		f := setup(t, allow{})
		f.api.syncWait = 30 * time.Millisecond
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		f.be.execFn = func(_ context.Context, req lifecycle.Request) ([]lifecycle.Result, error) {
			req.OnStarted(opID)
			req.OnStarted(op2)
			<-release
			return nil, nil
		}
		rec := f.do("POST", "/api/platform/modules/acme.example/install", `{"withDeps":true}`, nil)
		if rec.Code != 202 || rec.Header().Get("Location") != "/api/platform/modules/operations/"+opID.String() {
			t.Fatalf("%d loc=%q %s", rec.Code, rec.Header().Get("Location"), rec.Body)
		}
		if !strings.Contains(rec.Body.String(), `"operationIds":["`+opID.String()+`","`+op2.String()+`"]`) {
			t.Errorf("body = %s", rec.Body)
		}
	})
	t.Run("200 memuat semua ID operasi", func(t *testing.T) {
		f := setup(t, allow{})
		f.be.ops[opID] = store.Operation{ID: opID, TenantID: testTenant, ModuleKey: "acme.example", Operation: "install", Status: "succeeded"}
		f.be.ops[op2] = store.Operation{ID: op2, TenantID: testTenant, ModuleKey: "acme.example", Operation: "enable", Status: "succeeded"}
		f.be.execFn = func(context.Context, lifecycle.Request) ([]lifecycle.Result, error) {
			return []lifecycle.Result{
				{Module: "acme.example", Op: lifecycle.OpInstall, Status: "succeeded", OperationID: opID},
				{Module: "acme.example", Op: lifecycle.OpEnable, Status: "succeeded", OperationID: op2},
			}, nil
		}
		rec := f.do("POST", "/api/platform/modules/acme.example/install", `{"withDeps":true,"enable":true}`, nil)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"operationIds":["`+opID.String()+`","`+op2.String()+`"]`) {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})
}

// Keputusan 7 berlaku untuk semua mutasi, termasuk retry: lewat SyncWait -> 202 + Location ke operasi retry.
func TestAction_SlowRetryReturns202(t *testing.T) {
	f := setup(t, allow{})
	f.api.syncWait = 30 * time.Millisecond
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	retryOp := uuid.MustParse("0190a1b2-0000-7000-8000-0000000000aa")
	f.be.retryFn = func(_ context.Context, _, _ string, onStarted func(uuid.UUID)) (lifecycle.Result, error) {
		onStarted(retryOp)
		<-release
		return lifecycle.Result{}, nil
	}
	rec := f.do("POST", "/api/platform/modules/acme.example/retry", "", nil)
	if rec.Code != 202 || rec.Header().Get("Location") != "/api/platform/modules/operations/"+retryOp.String() {
		t.Fatalf("%d loc=%q %s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"status":"running"`) {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestAction_IfMatchMismatch412(t *testing.T) {
	f := setup(t, allow{})
	rec := f.do("POST", "/api/platform/modules/acme.example/enable", "", map[string]string{"If-Match": `"6"`})
	if rec.Code != 412 || errOf(t, rec).Error.Code != "version_mismatch" || len(f.be.execReqs) != 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	// RFC 9110: "7", "*", dan daftar entity-tag yang memuat "7" cocok.
	for _, h := range []string{`"7"`, `*`, `"5", "7"`} {
		if rec := f.do("POST", "/api/platform/modules/acme.example/enable", "", map[string]string{"If-Match": h}); rec.Code != 200 {
			t.Fatalf("If-Match %q cocok: %d %s", h, rec.Code, rec.Body)
		}
	}
	// Tag lemah tidak pernah cocok pada perbandingan kuat.
	if rec := f.do("POST", "/api/platform/modules/acme.example/enable", "", map[string]string{"If-Match": `W/"7"`}); rec.Code != 412 {
		t.Fatalf("If-Match lemah: %d %s", rec.Code, rec.Body)
	}
}

func TestAction_BodyStrict(t *testing.T) {
	f := setup(t, allow{})
	const install = "/api/platform/modules/acme.example/install"
	cases := []struct {
		name, body string
		hdr        map[string]string
		status     int
		code       string
		detail     string
		field      string
	}{
		{"field tak dikenal", `{"withDeps":true,"x":1}`, nil, 422, "validation_failed", "unknown_field", "x"},
		{"terlalu besar", `{"withDeps":false,"x":"` + strings.Repeat("a", 5<<10) + `"}`, nil, 400, "bad_request", "body_too_large", ""},
		{"content type salah", `{}`, map[string]string{"Content-Type": "text/plain"}, 400, "bad_request", "unsupported_media_type", ""},
		{"dua objek", `{"enable":true}{"enable":false}`, nil, 400, "bad_request", "trailing_data", ""},
		{"bukan objek", `[1]`, nil, 400, "bad_request", "expected_object", ""},
		{"tipe salah", `{"enable":"ya"}`, nil, 422, "validation_failed", "invalid_type", "enable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := f.do("POST", install, tc.body, tc.hdr)
			if rec.Code != tc.status || errOf(t, rec).Error.Code != tc.code {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
			var ds []map[string]string
			_ = json.Unmarshal(errOf(t, rec).Error.Details, &ds)
			if len(ds) == 0 || ds[0]["code"] != tc.detail || ds[0]["field"] != tc.field {
				t.Errorf("details = %v, want code %q field %q", ds, tc.detail, tc.field)
			}
		})
	}
	// Body kosong tanpa Content-Type diteruskan ke backend.
	if rec := f.do("POST", install, "", nil); rec.Code != 200 {
		t.Errorf("body kosong: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("POST", "/api/platform/modules/acme.example/enable", `{"withDeps":true}`, nil); rec.Code != 400 {
		t.Errorf("withDeps hanya untuk install: %d", rec.Code)
	}
	if len(f.be.execReqs) != 1 { // hanya panggilan body kosong
		t.Errorf("Execute dipanggil %d kali, want 1 (body tidak valid tidak boleh sampai backend)", len(f.be.execReqs))
	}
}

func TestPermission_StubDeny403(t *testing.T) {
	var buf strings.Builder
	f := setup(t, authorization.NewStub("deny", slog.New(slog.NewTextHandler(&buf, nil))))
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/platform/modules"},
		{"GET", "/api/platform/modules/acme.example"},
		{"GET", "/api/platform/modules/acme.example/history"},
		{"GET", "/api/platform/modules/operations/" + opID.String()},
		{"POST", "/api/platform/modules/acme.example/enable"},
	} {
		rec := f.do(tc.method, tc.path, "", nil)
		if rec.Code != 403 || errOf(t, rec).Error.Code != "permission_denied" {
			t.Errorf("%s %s: %d %s", tc.method, tc.path, rec.Code, rec.Body)
		}
	}
	if len(f.be.execReqs) != 0 {
		t.Error("handler tidak boleh jalan")
	}
}

// Keputusan 8 + FR-20 AC: gate mutasi mati (production tanpa ERP_MODULES_ADMIN_API) -> 404 walau stub deny.
func TestMutationGate_Production404(t *testing.T) {
	f := setup(t, authorization.NewStub("deny", slog.New(slog.NewTextHandler(io.Discard, nil))), runtime.SkipRoute(IsMutation))
	rec := f.do("POST", "/api/platform/modules/acme.example/enable", "", nil)
	if rec.Code != 404 || errOf(t, rec).Error.Code != "not_found" {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("GET", "/api/platform/modules", "", nil); rec.Code != 403 {
		t.Fatalf("GET tetap dijaga permission: %d", rec.Code)
	}
	f2 := setup(t, allow{}, runtime.SkipRoute(IsMutation))
	if rec := f2.do("GET", "/api/platform/modules", "", nil); rec.Code != 200 {
		t.Fatalf("GET: %d", rec.Code)
	}
	if rec := f2.do("POST", "/api/platform/modules/acme.example/enable", "", nil); rec.Code != 404 {
		t.Fatalf("POST (allow): %d", rec.Code)
	}
}

func TestIsMutation_OnlyActionRoute(t *testing.T) {
	f := setup(t, allow{})
	_ = f
	reg, _ := registry.Build(registry.Options{PlatformVersion: "0.1.0", SDKVersion: "0.1.0", Platform: New().Register, PlatformCapabilities: Capabilities})
	n := 0
	for _, rt := range reg.Routes() {
		if IsMutation(rt) {
			n++
			if rt.Method != "POST" || rt.Permission != "platform.modules.manage" {
				t.Errorf("route = %+v", rt)
			}
		} else if rt.Method != "GET" || rt.Permission != "platform.modules.read" {
			t.Errorf("route baca = %+v", rt)
		}
	}
	if n != 1 {
		t.Errorf("jumlah route mutasi = %d, want 1", n)
	}
}
