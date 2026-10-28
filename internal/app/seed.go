package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"erpora/internal/platform/config"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/sdk/module"
)

// seedReason adalah alasan hak sistem teraudit untuk seeding dev dari CLI.
const seedReason = "cli:db:seed"

// RunDevSeeders memanggil SeedDev setiap modul berstatus enabled yang mengimplementasi module.DevSeeder, dalam urutan
// topologis (dependensi lebih dulu), lewat runtime.ModuleContext dengan authz.AsSystem(ctx, "cli:db:seed").
// only != "" hanya memproses modul itu (key, slug, atau slug tanpa tanda hubung = nama direktori): key tidak dikenal
// -> ExitError{2}; modul tidak enabled -> ExitError{3}. Tanpa only, modul yang tidak enabled dilewati. Mengembalikan
// key modul yang di-seed. Pemanggil bertanggung jawab atas guard lingkungan (GuardDestructive).
func RunDevSeeders(ctx context.Context, cfg config.Config, log *slog.Logger, only string) ([]string, error) {
	rt, err := openCLI(ctx, cfg, log)
	if err != nil {
		return nil, err
	}
	defer rt.cleanup()

	target := ""
	if only != "" {
		for _, e := range rt.reg.Entries() {
			slug := e.Manifest.Slug
			if e.Manifest.Key != registry.PlatformKey && (e.Manifest.Key == only || slug == only || strings.ReplaceAll(slug, "-", "") == only) {
				target = e.Manifest.Key
			}
		}
		if target == "" {
			return nil, &ExitError{Code: 2, Err: fmt.Errorf("modul %q tidak dikenal", only)}
		}
	}

	var seeded []string
	for _, key := range rt.reg.Graph().Order() {
		if key == registry.PlatformKey || (target != "" && key != target) {
			continue
		}
		e, ok := rt.reg.Entry(key)
		if !ok {
			continue
		}
		st, _, err := rt.svc.Module(ctx, rt.tid, key)
		if err != nil {
			return seeded, err
		}
		if st.Status != runtime.StatusEnabled {
			if target != "" {
				return seeded, &ExitError{Code: 3, Err: fmt.Errorf("modul %s berstatus %s; aktifkan dengan `erpora modules enable %s`", key, st.Status, key)}
			}
			continue
		}
		seeder, ok := e.Module.(module.DevSeeder)
		if !ok {
			continue
		}
		mctx, env := runtime.ModuleContext(ctx, rt.tid, key, rt.conn, log, seedReason, rt.services)
		if err := seeder.SeedDev(mctx, env); err != nil {
			return seeded, fmt.Errorf("seed modul %s: %w", key, err)
		}
		seeded = append(seeded, key)
	}
	return seeded, nil
}
