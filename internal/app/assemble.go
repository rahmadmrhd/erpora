package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"erpora/internal/platform/audit"
	"erpora/internal/platform/authorization"
	"erpora/internal/platform/bootstrap"
	"erpora/internal/platform/buildinfo"
	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/i18n"
	i18nadmin "erpora/internal/platform/i18n/admin"
	i18nstore "erpora/internal/platform/i18n/store"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/modules/admin"
	"erpora/internal/platform/modules/lifecycle"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
	"erpora/web"
)

// assembly adalah hasil perakitan platform + lifecycle modul untuk satu proses serve.
type assembly struct {
	Router    *httpserver.Router
	Lifecycle *lifecycle.Service
	Snapshot  *runtime.Snapshot
	Tenant    tenancy.TenantID
	I18n      *i18n.Service
}

// buildLifecycle merakit lifecycle.Service di atas db. snap dan notifier boleh nil (CLI tidak punya snapshot).
func buildLifecycle(reg *registry.Registry, db *database.DB, tid tenancy.TenantID, snap *runtime.Snapshot,
	notifier runtime.StateNotifier, prov observability.Providers, log *slog.Logger, svc runtime.Services) *lifecycle.Service {
	conn := database.SingleTenantConnector{Default: tid, DB: database.TenantDB{DB: db}}
	deps := lifecycle.Deps{
		Registry: reg,
		Store:    store.New(database.ControlDB{DB: db}, conn),
		Migrator: lifecycle.NewMigrator(conn),
		Tenants:  conn,
		Snapshot: snap,
		Notifier: notifier,
		Tracer:   prov.Tracer,
		Meter:    prov.Meter,
		Log:      log,
		Services: svc,
	}
	lifecycleTestHooks(&deps)
	return lifecycle.New(deps)
}

// defaultTenant membaca tenant default dari platform_state dan memastikan konsisten dengan konfigurasi.
func defaultTenant(ctx context.Context, cfg config.Config, db *database.DB) (tenancy.TenantID, error) {
	id, err := migrate.DefaultTenantID(ctx, db)
	if err != nil {
		return tenancy.TenantID{}, err
	}
	if cfg.DefaultTenantID != nil && *cfg.DefaultTenantID != id {
		return tenancy.TenantID{}, fmt.Errorf("ERP_DEFAULT_TENANT_ID (%s) berbeda dengan tenant default di database (%s)", *cfg.DefaultTenantID, id)
	}
	return tenancy.TenantID(id), nil
}

// assemble merakit router, snapshot, dan lifecycle: reconcile saat startup (FR-19.6), langganan notifikasi
// revision, watcher antarproses, dan route modul dengan snapshot yang sama. Error reconcile (selain lock
// timeout, yang dilaporkan sebagai LockSkipped) menggagalkan startup.
func assemble(ctx context.Context, cfg config.Config, log *slog.Logger, db *database.DB, prov observability.Providers, reg *registry.Registry, apis *platformAPIs) (*assembly, error) {
	tid, err := defaultTenant(ctx, cfg, db)
	if err != nil {
		return nil, err
	}
	snap := runtime.NewSnapshot()
	notifier := runtime.NewLocalNotifier()
	// Katalog terjemahan divalidasi sebelum apa pun dilayani: galat struktural menggagalkan startup (exit 2).
	i18nSvc, err := newI18nService(log, cfg.Env == config.EnvDevelopment, reg, moduleActive(reg, func(t tenancy.TenantID, key string) bool {
		return snap.Status(t, key) == runtime.StatusEnabled
	}), i18nstore.New(database.ControlDB{DB: db}))
	if err != nil {
		return nil, &ExitError{Code: 2, Err: fmt.Errorf("katalog terjemahan tidak valid: %w", err)}
	}
	services := platformServices(cfg, log)
	services.I18n = i18nHook(i18nSvc, reg)
	svc := buildLifecycle(reg, db, tid, snap, notifier, prov, log, services)

	rep, err := svc.Reconcile(ctx, tid)
	if err != nil {
		return nil, fmt.Errorf("reconcile status modul gagal: %w", err)
	}
	log.Info("reconcile modul selesai", "upgrade_pending", rep.UpgradePending, "downgraded", rep.Downgraded,
		"restored", rep.Restored, "missing", rep.Missing, "interrupted", len(rep.Interrupted), "lock_skipped", rep.LockSkipped)

	notifier.Subscribe(func(ctx context.Context, t tenancy.TenantID, revision int64) {
		if revision > snap.Revision(t) {
			if _, err := svc.Refresh(ctx, t); err != nil {
				log.WarnContext(ctx, "refresh snapshot modul gagal", "error", err)
			}
		}
	})
	go svc.Watch(ctx, tid, lifecycle.DefaultPollInterval)

	router := httpserver.NewRouter(httpserver.Deps{
		Logger:     log,
		Tracer:     prov.Tracer,
		Resolver:   tenancy.SingleTenantResolver{ID: tid},
		Connector:  database.SingleTenantConnector{Default: tid, DB: database.TenantDB{DB: db}},
		Readiness:  dbReadiness{db: db},
		Assets:     web.Dist(),
		AppVersion: buildinfo.Version,
		Dev:        cfg.Env == config.EnvDevelopment,
		// Deteksi N+1 hanya di development (ERP_DB_NPLUSONE_THRESHOLD, 0 = mati).
		NPlusOneThreshold: cfg.DBNPlusOneThreshold,
		I18n:              i18nSvc,
		Bootstrap:         bootstrapFrom(reg, snap, i18nSvc, cfg.ModulesAdminAPIEnabled()),
		// Autentikasi nyata datang dari PRD-001; sementara pass-through.
		Authenticate: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				log.DebugContext(r.Context(), "authN stub")
				next.ServeHTTP(w, r)
			})
		},
	})
	apis.Modules.Bind(svc)
	apis.I18n.Bind(i18nSvc)
	// Gate mutasi (keputusan 8): di luar development tanpa ERP_MODULES_ADMIN_API=true, route aksi mutasi tidak
	// dilayani (404 sebelum permission), sementara route baca tetap tunduk pada permission.
	var mountOpts []runtime.MountOption
	if !cfg.ModulesAdminAPIEnabled() {
		mountOpts = append(mountOpts, runtime.SkipRoute(func(rt registry.RouteInfo) bool {
			return admin.IsMutation(rt) || i18nadmin.IsMutation(rt)
		}))
	}
	mountOpts = append(mountOpts, runtime.WithServices(services))
	if prov.Meter != nil {
		mountOpts = append(mountOpts, runtime.WithMeter(prov.Meter.Meter("erpora/modules")))
	}
	runtime.Mount(router, reg, snap, authorization.NewStub(authorization.StubMode(cfg.AuthzStub), log), log, mountOpts...)
	registerTestRoutes(router)
	return &assembly{Router: router, Lifecycle: svc, Snapshot: snap, Tenant: tid, I18n: i18nSvc}, nil
}

// bootstrapFrom membentuk isi /api/bootstrap dari katalog (tanpa platform.core) dan snapshot runtime.
// Bagian i18n memuat bahasa terdaftar dan revisi katalog namespace aktif; ETag mengikuti revision snapshot
// dan revisi katalog (bootstrap.Manifest.ETag). Bidang tambahan kelak (mis. features) ditambahkan sebagai
// struct bersarang opsional pada bootstrap.Manifest seperti I18n, tanpa mengubah fungsi ini.
func bootstrapFrom(reg *registry.Registry, snap *runtime.Snapshot, i18nSvc *i18n.Service, modulesAdminAPI bool) func(context.Context, tenancy.TenantID) bootstrap.Manifest {
	return func(ctx context.Context, t tenancy.TenantID) bootstrap.Manifest {
		statuses, rev := snap.Statuses(t)
		var mods []bootstrap.ModuleInput
		for _, e := range reg.Entries() {
			m := e.Manifest
			if m.Key == registry.PlatformKey {
				continue
			}
			st := statuses[m.Key]
			if st == "" {
				st = runtime.StatusAvailable
			}
			mods = append(mods, bootstrap.ModuleInput{
				Key: m.Key, Slug: m.Slug, Version: m.Version, Status: string(st), OfflineMode: m.Offline.Mode,
			})
		}
		m := bootstrap.Build(buildinfo.Version, t, mods, rev)
		m.Features = &bootstrap.Features{ModulesAdminAPI: modulesAdminAPI}
		if i18nSvc != nil {
			lr := i18nSvc.Registry()
			locales := make([]bootstrap.I18nLocale, 0, len(lr.Locales))
			for _, l := range lr.Locales {
				locales = append(locales, bootstrap.I18nLocale{Code: l.Code, Name: l.Name})
			}
			m.I18n = bootstrap.NewI18n(lr.Default, locales, i18nSvc.Revisions(ctx, t))
		}
		return m
	}
}

// platformServices membangun layanan platform yang dipasang ke context modul (route, hook lifecycle,
// command): masker field dan auditor ekspor stub sampai PRD-001 (FR-112, FR-130).
func platformServices(cfg config.Config, log *slog.Logger) runtime.Services {
	return runtime.Services{
		Masker:  authorization.NewStubFieldMasker(authorization.StubMode(cfg.AuthzStub), log),
		Auditor: audit.NewExportLog(log),
	}
}
