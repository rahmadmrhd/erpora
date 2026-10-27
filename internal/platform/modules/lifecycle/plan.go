package lifecycle

import (
	"context"
	"errors"

	"erpora/internal/platform/modules/runtime"
	"erpora/sdk/module"
)

// PlannedOp adalah satu operasi dalam rencana (dry-run).
type PlannedOp struct {
	Module                             string
	Op                                 Op
	From, To                           runtime.Status
	NoOp                               bool
	Steps                              []string
	PendingMigrations                  []string
	RoutesActivated, RoutesDeactivated []string
	Blocked                            *Error
}

// Plan adalah rencana berurutan; operasi yang diblokir prasyarat memuat Blocked dan menjadi yang terakhir.
type Plan struct{ Ops []PlannedOp }

func plannedSteps(op Op) []string {
	switch op {
	case OpInstall:
		return []string{"resolve", "compatibility", "migrate", "hook:install"}
	case OpUpgrade:
		return []string{"resolve", "compatibility", "migrate", "hook:upgrade"}
	case OpEnable:
		return []string{"dependencies", "hook:enable", "health"}
	case OpDisable:
		return []string{"dependents", "hook:disable"}
	case OpUninstall:
		return []string{"dependents"}
	}
	return nil
}

// Plan menyusun rencana tanpa lock dan tanpa menulis apa pun (read-only): validasi prasyarat seluruh
// rangkaian, migrasi tertunda, dan route yang aktif/nonaktif.
func (s *Service) Plan(ctx context.Context, req Request) (Plan, error) {
	items, err := s.expand(req)
	if err != nil {
		return Plan{}, err
	}
	rows, err := s.loadRows(ctx, req.Tenant)
	if err != nil {
		return Plan{}, err
	}
	planned, failed, perr := s.simulate(items, rows, req.All)
	var out Plan
	for _, p := range planned {
		op := PlannedOp{Module: p.key, Op: p.op, From: p.from, To: p.to, NoOp: p.noop}
		if !p.noop {
			op.Steps = plannedSteps(p.op)
			e, _ := s.d.Registry.Entry(p.key)
			if (p.op == OpInstall || p.op == OpUpgrade) && e.Manifest.Migrations.Strategy != "none" {
				if src, ok := e.Module.(module.MigrationSource); ok {
					pending, err := s.d.Migrator.Pending(ctx, req.Tenant, e.Manifest.Slug, src.Migrations())
					if err != nil {
						return Plan{}, err
					}
					op.PendingMigrations = pending
				}
			}
			switch p.op {
			case OpEnable:
				op.RoutesActivated = s.routesOf(p.key)
			case OpDisable:
				op.RoutesDeactivated = s.routesOf(p.key)
			}
		}
		out.Ops = append(out.Ops, op)
	}
	if perr != nil {
		var le *Error
		if !errors.As(perr, &le) {
			return Plan{}, perr
		}
		out.Ops = append(out.Ops, PlannedOp{Module: failed.key, Op: failed.op, Blocked: le})
	}
	return out, nil
}

func (s *Service) routesOf(key string) []string {
	var out []string
	for _, r := range s.d.Registry.Routes() {
		if r.Module == key {
			out = append(out, r.Method+" "+r.Pattern)
		}
	}
	return out
}
