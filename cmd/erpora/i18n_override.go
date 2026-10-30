package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/platform/config"
	"erpora/internal/platform/i18n"
	"erpora/internal/platform/i18n/store"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
)

// i18nOverrideBackend dipenuhi *i18n.Service (dibungkus guard penanda lingkungan); fake pada test.
type i18nOverrideBackend interface {
	SetOverride(ctx context.Context, t tenancy.TenantID, o i18n.Override, actor string, pre store.Precondition) (bool, error)
	DeleteOverride(ctx context.Context, t tenancy.TenantID, ns, loc, key, actor string, pre store.Precondition) (bool, error)
	ListOverrides(ctx context.Context, t tenancy.TenantID, f store.Filter, page, pageSize int) ([]store.Override, int64, error)
	// GuardWrite menolak mutasi oleh binary dev terhadap DB berpenanda production (FR-22.5).
	GuardWrite(ctx context.Context) error
}

type guardedI18n struct {
	*i18n.Service
	guard func(ctx context.Context) error
}

func (g guardedI18n) GuardWrite(ctx context.Context) error { return g.guard(ctx) }

// i18nBackendFactory membuka backend (DB) dan mengembalikan tenant default serta fungsi cleanup.
type i18nBackendFactory func(ctx context.Context, opts *rootOpts, errOut io.Writer) (i18nOverrideBackend, tenancy.TenantID, func(), error)

func defaultI18nBackend(ctx context.Context, opts *rootOpts, errOut io.Writer) (i18nOverrideBackend, tenancy.TenantID, func(), error) {
	cfg, err := config.Load(opts.envFile)
	if err != nil {
		return nil, tenancy.TenantID{}, nil, err
	}
	log := observability.NewLogger(errOut, cfg.LogFormat, slog.LevelInfo)
	svc, tid, guard, cleanup, err := app.NewGuardedI18n(ctx, cfg, log)
	if err != nil {
		return nil, tenancy.TenantID{}, nil, err
	}
	return guardedI18n{Service: svc, guard: guard}, tid, cleanup, nil
}

const i18nHelp = `Kelola terjemahan platform. Subperintah: override.`

const overrideHelp = `Kelola override istilah: mengganti teks katalog bawaan untuk tenant default tanpa build ulang
(PRD-000 FR-25). Antarmuka utama di produksi (API mutasi mati di luar development). Exit code: 0 sukses atau
tidak ada perubahan; 2 validasi (argumen salah, kunci/namespace/bahasa tak dikenal, pesan ditolak, konfirmasi
hilang); 3 prasyarat atau konfigurasi gagal (penyimpanan override tidak tersedia, migrasi belum diterapkan,
guard lingkungan); 4 operasi gagal; 1 lainnya.

Pesan memakai sintaks ICU MessageFormat. Argumen harus sama dengan pesan bawaan, tanpa HTML, paling banyak
2000 karakter.`

func newI18nCmd(opts *rootOpts, open i18nBackendFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "i18n",
		Short:   "Kelola terjemahan platform (override istilah)",
		Long:    i18nHelp,
		GroupID: groupRuntime,
	}
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &app.ExitError{Code: 2, Err: err} })
	ov := &cobra.Command{
		Use:   "override",
		Short: "Simpan, daftar, atau hapus override istilah (set, list, rm)",
		Long:  overrideHelp,
	}
	ov.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &app.ExitError{Code: 2, Err: err} })
	ov.AddCommand(overrideSetCmd(opts, open), overrideListCmd(opts, open), overrideRmCmd(opts, open))
	cmd.AddCommand(ov)
	return cmd
}

// overrideExit memetakan galat override ke exit code: validasi dan target tak dikenal 2, penyimpanan tidak
// tersedia 3, selain itu 4. ExitError dipertahankan.
func overrideExit(err error) error {
	if err == nil {
		return nil
	}
	var xe *app.ExitError
	if errors.As(err, &xe) {
		return err
	}
	var oe *i18n.OverrideError
	switch {
	case errors.As(err, &oe):
		return &app.ExitError{Code: 2, Err: fmt.Errorf("override ditolak (%s) pada %s: %s", oe.Code, oe.Field, oe.Detail)}
	case errors.Is(err, i18n.ErrNotFound), errors.Is(err, store.ErrNotFound):
		return &app.ExitError{Code: 2, Err: errors.New("namespace atau bahasa tidak dikenal (atau modul tidak aktif)")}
	case errors.Is(err, i18n.ErrOverridesUnavailable):
		return &app.ExitError{Code: 3, Err: errors.New("penyimpanan override tidak tersedia")}
	}
	return &app.ExitError{Code: 4, Err: err}
}

func overrideSetCmd(opts *rootOpts, open i18nBackendFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "set <namespace> <locale> <key> <message>",
		Short: "Simpan override (idempoten: nilai yang sama tidak mengubah apa pun)",
		Args:  exactArgs(4, "set <namespace> <locale> <key> <message>"),
		RunE: func(c *cobra.Command, args []string) error {
			be, tid, cleanup, err := open(c.Context(), opts, c.ErrOrStderr())
			if err != nil {
				return err
			}
			defer cleanup()
			if err := be.GuardWrite(c.Context()); err != nil {
				return err
			}
			changed, err := be.SetOverride(c.Context(), tid, i18n.Override{Namespace: args[0], Locale: args[1], Key: args[2], Message: args[3]}, cliActor(), nil)
			if err != nil {
				return overrideExit(err)
			}
			target := args[0] + "/" + args[1] + "/" + args[2]
			if !changed {
				fmt.Fprintf(c.OutOrStdout(), "tidak ada perubahan: %s sudah bernilai sama\n", target)
				return nil
			}
			fmt.Fprintf(c.OutOrStdout(), "override disimpan: %s\n", target)
			return nil
		},
	}
}

type overrideJSON struct {
	Namespace string `json:"namespace"`
	Locale    string `json:"locale"`
	Key       string `json:"key"`
	Message   string `json:"message"`
	UpdatedBy string `json:"updatedBy"`
	UpdatedAt string `json:"updatedAt"`
}

func overrideListCmd(opts *rootOpts, open i18nBackendFactory) *cobra.Command {
	var ns, loc string
	var asJSON bool
	cmd := &cobra.Command{
		Use: "list", Short: "Daftar override tenant (--namespace, --locale, --json)", Args: exactArgs(0, "list tidak menerima argumen"),
		RunE: func(c *cobra.Command, _ []string) error {
			be, tid, cleanup, err := open(c.Context(), opts, c.ErrOrStderr())
			if err != nil {
				return err
			}
			defer cleanup()
			var rows []store.Override
			for page := 1; ; page++ {
				batch, total, err := be.ListOverrides(c.Context(), tid, store.Filter{Namespace: ns, Locale: loc}, page, 200)
				if err != nil {
					return overrideExit(err)
				}
				rows = append(rows, batch...)
				if len(batch) == 0 || int64(len(rows)) >= total {
					break
				}
			}
			if asJSON {
				out := make([]overrideJSON, 0, len(rows))
				for _, o := range rows {
					out = append(out, overrideJSON{Namespace: o.Namespace, Locale: o.Locale, Key: o.Key, Message: o.Message,
						UpdatedBy: o.UpdatedBy, UpdatedAt: o.UpdatedAt.UTC().Format(time.RFC3339)})
				}
				return writeJSON(c.OutOrStdout(), out)
			}
			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAMESPACE\tLOCALE\tKEY\tMESSAGE\tUPDATED BY\tUPDATED AT")
			for _, o := range rows {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", o.Namespace, o.Locale, o.Key, strconv.Quote(o.Message), dash(o.UpdatedBy), o.UpdatedAt.UTC().Format(time.RFC3339))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&ns, "namespace", "", "filter namespace")
	cmd.Flags().StringVar(&loc, "locale", "", "filter bahasa")
	cmd.Flags().BoolVar(&asJSON, "json", false, "keluaran JSON")
	return cmd
}

func overrideRmCmd(opts *rootOpts, open i18nBackendFactory) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm <namespace> <locale> <key>",
		Short: "Hapus override (kembali ke teks bawaan); meminta konfirmasi, --yes untuk skrip",
		Args:  exactArgs(3, "rm <namespace> <locale> <key>"),
		RunE: func(c *cobra.Command, args []string) error {
			target := args[0] + "/" + args[1] + "/" + args[2]
			if !yes {
				ok, err := confirm(c, "Hapus override "+target+"?")
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(c.OutOrStdout(), "dibatalkan")
					return nil
				}
			}
			be, tid, cleanup, err := open(c.Context(), opts, c.ErrOrStderr())
			if err != nil {
				return err
			}
			defer cleanup()
			if err := be.GuardWrite(c.Context()); err != nil {
				return err
			}
			existed, err := be.DeleteOverride(c.Context(), tid, args[0], args[1], args[2], cliActor(), nil)
			if err != nil {
				return overrideExit(err)
			}
			if !existed {
				fmt.Fprintf(c.OutOrStdout(), "override %s tidak ada; tidak ada perubahan\n", target)
				return nil
			}
			fmt.Fprintf(c.OutOrStdout(), "override dihapus: %s\n", target)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "lewati konfirmasi")
	return cmd
}
