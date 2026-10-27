package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/modules/lifecycle"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/httpx"
	"erpora/sdk/module"
)

// Capabilities adalah capability milik platform.core untuk manajemen modul.
var Capabilities = []string{"platform.modules.read", "platform.modules.manage"}

// SyncWait adalah batas tunggu aksi mutasi sebelum API membalas 202 + Location (keputusan 7).
const SyncWait = 30 * time.Second

// APIActor adalah actor audit untuk operasi lewat API sebelum PRD-001 (autentikasi) tersedia.
const APIActor = "api:dev"

const (
	permRead   = "platform.modules.read"
	permManage = "platform.modules.manage"

	defaultPageSize = 25
	maxPageSize     = 200
)

// Backend dipenuhi *lifecycle.Service; fake pada test.
type Backend interface {
	Modules(ctx context.Context, t tenancy.TenantID) ([]lifecycle.ModuleState, int64, error)
	Module(ctx context.Context, t tenancy.TenantID, key string) (lifecycle.ModuleState, int64, error)
	History(ctx context.Context, t tenancy.TenantID, key string, page, pageSize int) ([]store.Event, int64, error)
	Operation(ctx context.Context, t tenancy.TenantID, id uuid.UUID) (store.Operation, error)
	Execute(ctx context.Context, req lifecycle.Request) ([]lifecycle.Result, error)
	RetryModule(ctx context.Context, t tenancy.TenantID, key, actor string, onStarted func(uuid.UUID), ifMatch string) (lifecycle.Result, error)
}

// API adalah handler /api/platform/modules. Backend di-bind setelah perakitan (route didaftarkan lebih dulu
// lewat registry); sebelum itu semua endpoint membalas 503 not_ready.
type API struct {
	backend  atomic.Pointer[Backend]
	syncWait time.Duration
}

// New membuat API dengan SyncWait standar.
func New() *API { return &API{syncWait: SyncWait} }

// Bind memasang backend; dipanggil composition root.
func (a *API) Bind(b Backend) { a.backend.Store(&b) }

func (a *API) log(*http.Request) *slog.Logger { return slog.Default() }

// IsMutation menandai route aksi mutasi POST /api/platform/modules/{key}/{operation}; dipakai
// runtime.SkipRoute agar gate ERP_MODULES_ADMIN_API tidak memasang route itu (404 sebelum permission).
func IsMutation(rt registry.RouteInfo) bool {
	return rt.Module == registry.PlatformKey && rt.Method == http.MethodPost && rt.Pattern == "/api/platform/modules/{key}/{operation}"
}

// Register mendaftarkan kelima endpoint (path relatif platform.core). History dan operations/{id} berbagi satu
// route GET /modules/{key}/{sub}: ServeMux menolak GET /modules/{key}/history bersama
// GET /modules/operations/{id} sebagai bentrok, padahal kedua path ditetapkan PRD-000 FR-20.2.
// Key modul selalu berisi titik sehingga tidak pernah sama dengan "operations".
func (a *API) Register(r module.Registrar) error {
	r.Handle(http.MethodGet, "/modules", permRead, a.wrap(a.list))
	r.Handle(http.MethodGet, "/modules/{key}", permRead, a.wrap(a.get))
	r.Handle(http.MethodGet, "/modules/{key}/{sub}", permRead, a.wrap(a.getSub))
	r.Handle(http.MethodPost, "/modules/{key}/{operation}", permManage, a.wrap(a.action), module.MaxBodyBytes(4<<10))
	return nil
}

// actionBody adalah body opsional POST .../install (batas 4 KiB lewat opsi route, keputusan 20).
type actionBody struct {
	WithDeps bool `json:"withDeps"`
	Enable   bool `json:"enable"`
}

type handlerFn func(w http.ResponseWriter, r *http.Request, be Backend, t tenancy.TenantID)

func (a *API) wrap(fn handlerFn) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bp := a.backend.Load()
		if bp == nil {
			httpserver.WriteError(w, r, http.StatusServiceUnavailable, "not_ready", "layanan belum siap", nil)
			return
		}
		t, ok := tenancy.FromContext(r.Context())
		if !ok {
			httpserver.WriteError(w, r, http.StatusBadRequest, "tenant_unresolved", "tenant tidak dapat ditentukan", nil)
			return
		}
		fn(w, r, *bp, t)
	})
}

func badRequest(w http.ResponseWriter, r *http.Request, field, msg string) {
	httpserver.WriteError(w, r, http.StatusBadRequest, "bad_request", msg,
		[]map[string]string{{"field": field, "code": "invalid", "message": msg}})
}

// pageParams membaca page dan pageSize (api-guidelines section 6); ok false bila sudah membalas 400.
func pageParams(w http.ResponseWriter, r *http.Request) (page, size int, ok bool) {
	page, size = 1, defaultPageSize
	q := r.URL.Query()
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			badRequest(w, r, "page", "page harus bilangan bulat >= 1")
			return 0, 0, false
		}
		page = n
	}
	if v := q.Get("pageSize"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxPageSize {
			badRequest(w, r, "pageSize", fmt.Sprintf("pageSize harus antara 1 dan %d", maxPageSize))
			return 0, 0, false
		}
		size = n
	}
	return page, size, true
}

func etagOf(rev int64) string { return `"` + strconv.FormatInt(rev, 10) + `"` }

func (a *API) list(w http.ResponseWriter, r *http.Request, be Backend, t tenancy.TenantID) {
	page, size, ok := pageParams(w, r)
	if !ok {
		return
	}
	mods, rev, err := be.Modules(r.Context(), t)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if httpserver.CheckETag(w, r, etagOf(rev)) {
		return
	}
	total := len(mods)
	from := min((page-1)*size, total)
	to := min(from+size, total)
	views := make([]ModuleView, 0, to-from)
	for _, m := range mods[from:to] {
		views = append(views, toModuleView(m))
	}
	httpserver.WritePage(w, http.StatusOK, views, httpserver.PageInfo{Page: page, PageSize: size, Total: int64(total), HasMore: to < total})
}

func (a *API) get(w http.ResponseWriter, r *http.Request, be Backend, t tenancy.TenantID) {
	m, rev, err := be.Module(r.Context(), t, r.PathValue("key"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if httpserver.CheckETag(w, r, etagOf(rev)) {
		return
	}
	httpserver.WriteData(w, http.StatusOK, toModuleView(m))
}

// getSub melayani GET /modules/operations/{id} dan GET /modules/{key}/history.
func (a *API) getSub(w http.ResponseWriter, r *http.Request, be Backend, t tenancy.TenantID) {
	key, sub := r.PathValue("key"), r.PathValue("sub")
	if key == "operations" {
		id, err := uuid.Parse(sub)
		if err != nil {
			badRequest(w, r, "id", "id operasi bukan UUID")
			return
		}
		op, err := be.Operation(r.Context(), t, id)
		if err != nil {
			a.writeError(w, r, err)
			return
		}
		httpserver.WriteData(w, http.StatusOK, toOperationView(op))
		return
	}
	if sub != "history" {
		httpserver.WriteError(w, r, http.StatusNotFound, "not_found", "endpoint tidak ditemukan", nil)
		return
	}
	page, size, ok := pageParams(w, r)
	if !ok {
		return
	}
	evs, total, err := be.History(r.Context(), t, key, page, size)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	views := make([]EventView, 0, len(evs))
	for _, e := range evs {
		views = append(views, toEventView(e))
	}
	httpserver.WritePage(w, http.StatusOK, views, httpserver.PageInfo{Page: page, PageSize: size, Total: total, HasMore: int64(page*size) < total})
}

type outcome struct {
	results []lifecycle.Result
	err     error
}

var validOps = map[string]lifecycle.Op{
	"install": lifecycle.OpInstall, "enable": lifecycle.OpEnable, "disable": lifecycle.OpDisable,
	"upgrade": lifecycle.OpUpgrade, "uninstall": lifecycle.OpUninstall, "retry": lifecycle.OpRetry,
}

func (a *API) action(w http.ResponseWriter, r *http.Request, be Backend, t tenancy.TenantID) {
	key := r.PathValue("key")
	op, ok := validOps[r.PathValue("operation")]
	if !ok {
		httpserver.WriteError(w, r, http.StatusNotFound, "not_found", "endpoint tidak ditemukan", nil)
		return
	}
	body, err := httpx.DecodeJSON[actionBody](w, r, httpx.AllowEmptyBody())
	if err != nil {
		httpx.RespondError(w, r, err)
		return
	}
	if op != lifecycle.OpInstall && (body.WithDeps || body.Enable) {
		badRequest(w, r, "body", "withDeps dan enable hanya berlaku untuk install")
		return
	}
	// If-Match dievaluasi oleh lifecycle di dalam advisory lock (RFC 9110: "*", daftar, perbandingan kuat), bukan di sini.
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	// Idempotency-Key diterima tetapi tidak disimpan: setiap operasi idempoten lewat aturan no-op (keputusan 19).

	started := make(chan uuid.UUID, 1)
	done := make(chan outcome, 1)
	// Operasi berjalan lepas dari pembatalan request (klien putus tidak menghentikannya) tetapi tetap membawa nilai context.
	ctx := context.WithoutCancel(r.Context())
	go func() {
		var o outcome
		defer func() {
			if p := recover(); p != nil {
				a.log(r).ErrorContext(ctx, "panic pada operasi API modul", "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
				o = outcome{err: fmt.Errorf("panic: %v", p)}
			}
			done <- o
		}()
		onStarted := func(id uuid.UUID) {
			select {
			case started <- id:
			default:
			}
		}
		if op == lifecycle.OpRetry {
			res, err := be.RetryModule(ctx, t, key, APIActor, onStarted, ifMatch)
			o = outcome{results: []lifecycle.Result{res}, err: err}
			return
		}
		res, err := be.Execute(ctx, lifecycle.Request{
			Tenant: t, Op: op, Key: key, WithDeps: body.WithDeps, Enable: body.Enable, Actor: APIActor, OnStarted: onStarted, IfMatch: ifMatch,
		})
		o = outcome{results: res, err: err}
	}()

	timer := time.NewTimer(a.syncWait)
	defer timer.Stop()
	select {
	case o := <-done:
		a.respond(w, r, be, t, key, o)
		return
	case <-timer.C:
	}
	// Melewati SyncWait: balas 202 bila operasi sudah punya ID; bila belum (masih menunggu lock), tunggu salah satunya.
	select {
	case id := <-started:
		w.Header().Set("Location", "/api/platform/modules/operations/"+id.String())
		httpserver.WriteData(w, http.StatusAccepted, map[string]string{"operationId": id.String(), "status": "running"})
	case o := <-done:
		a.respond(w, r, be, t, key, o)
	}
}

func (a *API) respond(w http.ResponseWriter, r *http.Request, be Backend, t tenancy.TenantID, key string, o outcome) {
	if o.err != nil {
		a.writeError(w, r, o.err)
		return
	}
	ops := make([]OperationView, 0, len(o.results))
	for _, res := range o.results {
		if res.Status == "noop" || res.OperationID == uuid.Nil {
			continue
		}
		op, err := be.Operation(r.Context(), t, res.OperationID)
		if err != nil {
			a.writeError(w, r, err)
			return
		}
		ops = append(ops, toOperationView(op))
	}
	m, _, err := be.Module(r.Context(), t, key)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	httpserver.WriteData(w, http.StatusOK, map[string]any{"module": toModuleView(m), "operations": ops})
}
