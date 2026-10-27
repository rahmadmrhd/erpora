package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	tnoop "go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/platform/buildinfo"
	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
)

// databaseEnvTimeout membatasi pembacaan penanda lingkungan oleh `inspect config` (command ini tetap cepat dan exit 0).
const databaseEnvTimeout = 3 * time.Second

// databaseEnvironment membaca platform_state.environment (read-only). Hasil "unknown (<alasan>)" bila DSN kosong,
// DB tidak terjangkau, atau penanda belum ada; alasan tidak pernah memuat DSN.
func databaseEnvironment(ctx context.Context, c config.Config) string {
	if c.DatabaseURL == "" {
		return "unknown (ERP_DATABASE_URL kosong)"
	}
	ctx, cancel := context.WithTimeout(ctx, databaseEnvTimeout)
	defer cancel()
	db, err := database.Open(ctx, c, tnoop.NewTracerProvider(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		return "unknown (database tidak terjangkau)"
	}
	defer func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	}()
	env, ok, err := migrate.Environment(ctx, db)
	switch {
	case err != nil:
		return "unknown (penanda tidak terbaca)"
	case !ok:
		return "unknown (penanda belum ada; jalankan `erpora migrate up`)"
	}
	return string(env)
}

func newInspectCmd(opts *rootOpts) *cobra.Command {
	inspect := &cobra.Command{
		Use:     "inspect",
		Short:   "Inspeksi keadaan platform",
		GroupID: groupInspect,
	}
	inspect.AddCommand(&cobra.Command{
		Use:   "config",
		Short: "Tampilkan konfigurasi efektif (password DSN disamarkan)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := config.Load(opts.envFile)
			if err != nil {
				return err
			}
			tenant := ""
			if c.DefaultTenantID != nil {
				tenant = c.DefaultTenantID.String()
			}
			kv := map[string]string{
				"ERP_DATABASE_URL":          config.RedactDSN(c.DatabaseURL),
				"ERP_ENV":                   string(c.Env),
				"ERP_HTTP_ADDR":             c.HTTPAddr,
				"ERP_LOG_FORMAT":            c.LogFormat,
				"ERP_TENANCY_MODE":          c.TenancyMode,
				"ERP_DEFAULT_TENANT_ID":     tenant,
				"ERP_AUTHZ_STUB":            c.AuthzStub,
				"ERP_MODULES_ADMIN_API":     strconv.FormatBool(c.ModulesAdminAPI),
				"ERP_DB_NPLUSONE_THRESHOLD": strconv.Itoa(c.DBNPlusOneThreshold),
				"ERP_DB_SLOW_QUERY_MS":      strconv.FormatInt(c.DBSlowQuery.Milliseconds(), 10),
				"flavor":                    buildinfo.Flavor(),
				"database.environment":      databaseEnvironment(cmd.Context(), c),
			}
			keys := make([]string, 0, len(kv))
			for k := range kv {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", k, kv[k])
			}
			return nil
		},
	})
	inspect.AddCommand(newInspectRoutesCmd("routes", false), newInspectModulesCmd(), newInspectPermissionsCmd("permissions", false))
	return inspect
}
