package main

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/rahmadmrhd/erp-go/internal/platform/buildinfo"
	"github.com/rahmadmrhd/erp-go/internal/platform/config"
)

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
				"ERP_DATABASE_URL":      config.RedactDSN(c.DatabaseURL),
				"ERP_ENV":               string(c.Env),
				"ERP_HTTP_ADDR":         c.HTTPAddr,
				"ERP_LOG_FORMAT":        c.LogFormat,
				"ERP_TENANCY_MODE":      c.TenancyMode,
				"ERP_DEFAULT_TENANT_ID": tenant,
				"ERP_DB_SLOW_QUERY_MS":  strconv.FormatInt(c.DBSlowQuery.Milliseconds(), 10),
				"flavor":                buildinfo.Flavor(),
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
	return inspect
}
