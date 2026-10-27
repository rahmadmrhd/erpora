package lifecycle

import (
	"context"
	"slices"
	"testing"

	"erpora/internal/platform/modules/runtime"
)

func TestPlan_DisableBlockedByActiveDependent(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}), newFx(rec, fxSpec{key: "acme.b", deps: []moduleDep{dep("acme.a")}}))
	e.st.seed(tenantA, "acme.a", runtime.StatusEnabled, "1.0.0")
	e.st.seed(tenantA, "acme.b", runtime.StatusEnabled, "1.0.0")

	p, err := e.svc.Plan(context.Background(), Request{Tenant: tenantA, Op: OpDisable, Key: "acme.a"})
	if err != nil || len(p.Ops) != 1 {
		t.Fatalf("Plan = %+v, %v", p, err)
	}
	if b := p.Ops[0].Blocked; b == nil || b.Code != CodeDependentsActive || !slices.Equal(b.Related, []string{"acme.b"}) {
		t.Errorf("Blocked = %+v", b)
	}
	if len(e.st.startOps) != 0 || len(e.st.commits) != 0 || len(rec.list()) != 0 {
		t.Error("Plan harus read-only")
	}
}

func TestPlan_InstallWithDepsListsPendingMigrations(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec,
		newFx(rec, fxSpec{key: "acme.a", migrations: fxMigFS()}),
		newFx(rec, fxSpec{key: "acme.b", deps: []moduleDep{dep("acme.a")}, migrations: fxMigFS()}),
	)
	p, err := e.svc.Plan(context.Background(), Request{Tenant: tenantA, Op: OpInstall, Key: "acme.b", WithDeps: true})
	if err != nil || len(p.Ops) != 2 {
		t.Fatalf("Plan = %+v, %v", p, err)
	}
	if p.Ops[0].Module != "acme.a" || p.Ops[1].Module != "acme.b" {
		t.Errorf("urutan = %s, %s", p.Ops[0].Module, p.Ops[1].Module)
	}
	for _, op := range p.Ops {
		if !slices.Equal(op.PendingMigrations, []string{"0001_one.sql", "0002_two.sql"}) || op.Blocked != nil {
			t.Errorf("op = %+v", op)
		}
		if !slices.Equal(op.Steps, []string{"resolve", "compatibility", "migrate", "hook:install"}) {
			t.Errorf("steps = %v", op.Steps)
		}
	}
	if e.mig.calls != 0 || len(e.st.startOps) != 0 {
		t.Error("Plan tidak boleh menjalankan migrasi atau menulis")
	}
}

func TestPlan_EnableListsRoutesActivated(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}))
	e.st.seed(tenantA, "acme.a", runtime.StatusInstalled, "1.0.0")
	p, err := e.svc.Plan(context.Background(), Request{Tenant: tenantA, Op: OpEnable, Key: "acme.a"})
	if err != nil || len(p.Ops) != 1 {
		t.Fatalf("Plan = %+v, %v", p, err)
	}
	if got := p.Ops[0].RoutesActivated; !slices.Equal(got, []string{"GET /api/a/items"}) {
		t.Errorf("RoutesActivated = %v", got)
	}
	if !slices.Equal(p.Ops[0].Steps, []string{"dependencies", "hook:enable", "health"}) {
		t.Errorf("steps = %v", p.Ops[0].Steps)
	}
	// disable melaporkan route yang dinonaktifkan.
	e.st.seed(tenantA, "acme.a", runtime.StatusEnabled, "1.0.0")
	p, _ = e.svc.Plan(context.Background(), Request{Tenant: tenantA, Op: OpDisable, Key: "acme.a"})
	if got := p.Ops[0].RoutesDeactivated; !slices.Equal(got, []string{"GET /api/a/items"}) {
		t.Errorf("RoutesDeactivated = %v", got)
	}
}

func TestPlan_UnknownKey(t *testing.T) {
	rec := &fxRec{}
	e := newEnv(t, rec, newFx(rec, fxSpec{key: "acme.a"}))
	_, err := e.svc.Plan(context.Background(), Request{Tenant: tenantA, Op: OpEnable, Key: "acme.nope"})
	wantCode(t, err, CodeModuleNotFound)
}
