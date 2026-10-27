package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"strings"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/platform/config"
	"erpora/internal/platform/modules/lifecycle"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
)

// modulesBackend dipenuhi *lifecycle.Service; fake pada golden test.
type modulesBackend interface {
	Modules(ctx context.Context, t tenancy.TenantID) ([]lifecycle.ModuleState, int64, error)
	Module(ctx context.Context, t tenancy.TenantID, key string) (lifecycle.ModuleState, int64, error)
	History(ctx context.Context, t tenancy.TenantID, key string, page, pageSize int) ([]store.Event, int64, error)
	Operation(ctx context.Context, t tenancy.TenantID, id uuid.UUID) (store.Operation, error)
	Execute(ctx context.Context, req lifecycle.Request) ([]lifecycle.Result, error)
	Retry(ctx context.Context, t tenancy.TenantID, id uuid.UUID, actor string) (lifecycle.Result, error)
	Plan(ctx context.Context, req lifecycle.Request) (lifecycle.Plan, error)
}

// backendFactory membuka backend (DB) dan mengembalikan tenant default serta fungsi cleanup.
type backendFactory func(ctx context.Context, opts *rootOpts, errOut io.Writer) (modulesBackend, tenancy.TenantID, func(), error)

func defaultBackend(ctx context.Context, opts *rootOpts, errOut io.Writer) (modulesBackend, tenancy.TenantID, func(), error) {
	cfg, err := config.Load(opts.envFile)
	if err != nil {
		return nil, tenancy.TenantID{}, nil, err
	}
	log := observability.NewLogger(errOut, cfg.LogFormat, slog.LevelInfo)
	svc, tid, cleanup, err := app.NewLifecycle(ctx, cfg, log)
	if err != nil {
		return nil, tenancy.TenantID{}, nil, err
	}
	return svc, tid, cleanup, nil
}

// cliActor adalah actor audit untuk operasi dari CLI: "cli:<os-user>".
func cliActor() string {
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	if name == "" {
		name = os.Getenv("USERNAME")
	}
	if name == "" {
		name = os.Getenv("USER")
	}
	if name == "" {
		name = "unknown"
	}
	return "cli:" + name
}

// lifecycleExitCode memetakan error lifecycle ke exit code: key/operasi tidak dikenal -> 2;
// prasyarat/konflik (invalid_transition, dependents_active, module_required, operation_in_progress,
// module_incompatible, dst.) -> 3. Selain itu 1.
func lifecycleExitCode(err error) int {
	var le *lifecycle.Error
	if !errors.As(err, &le) {
		return 1
	}
	switch le.Code {
	case lifecycle.CodeModuleNotFound, lifecycle.CodeOperationNotFound:
		return 2
	}
	return 3
}

func usageErr(format string, a ...any) error {
	return &app.ExitError{Code: 2, Err: fmt.Errorf(format, a...)}
}

// allHelp menjelaskan perilaku --all (keputusan perencanaan M2b); dipakai di help install/enable/upgrade.
const allHelp = `Dengan --all, operasi diterapkan ke semua modul terkompilasi (kecuali platform.core) dalam urutan
dependensi. Modul yang transisinya tidak sah untuk operasi itu dilewati (mis. "enable --all" melewati modul
yang belum ter-install). Prasyarat yang gagal pada modul mana pun (dependensi belum siap, versi/platform tidak
kompatibel, modul wajib) menolak seluruh rencana: tidak ada yang dijalankan dan exit code 3. Kegagalan
operasi di tengah rangkaian menghentikan sisanya (exit code 4).`

type modulesCtx struct {
	opts   *rootOpts
	open   backendFactory
	tenant string
}

func newModulesCmd(opts *rootOpts, open backendFactory) *cobra.Command {
	mc := &modulesCtx{opts: opts, open: open}
	cmd := &cobra.Command{
		Use:     "modules",
		Short:   "Kelola siklus hidup modul (install, enable, disable, upgrade, uninstall, retry)",
		GroupID: groupRuntime,
		Long: `Kelola modul untuk tenant default. Exit code: 0 sukses atau tidak ada perubahan; 2 validasi
(argumen, key/operasi tidak dikenal, --tenant bukan tenant default, konfirmasi hilang); 3 konflik atau
prasyarat tidak terpenuhi; 4 operasi gagal; 1 lainnya.`,
	}
	cmd.PersistentFlags().StringVar(&mc.tenant, "tenant", "", "ID tenant (hanya tenant default yang didukung)")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &app.ExitError{Code: 2, Err: err} })
	cmd.AddCommand(
		mc.listCmd(), mc.statusCmd(),
		mc.opCmd(lifecycle.OpInstall, "Install modul (--with-deps, --enable, --all)", true, true, true, false),
		mc.opCmd(lifecycle.OpEnable, "Aktifkan modul", true, true, false, false),
		mc.opCmd(lifecycle.OpDisable, "Nonaktifkan modul", false, false, false, false),
		mc.opCmd(lifecycle.OpUpgrade, "Upgrade modul ke versi binary (--all)", true, false, false, false),
		mc.opCmd(lifecycle.OpUninstall, "Uninstall modul (data tetap disimpan)", false, false, false, true),
		mc.retryCmd(), mc.historyCmd(), mc.planCmd(),
	)
	return cmd
}

// session membuka backend, memeriksa --tenant, dan mengembalikan tenant default.
func (mc *modulesCtx) session(c *cobra.Command) (modulesBackend, tenancy.TenantID, func(), error) {
	be, tid, cleanup, err := mc.open(c.Context(), mc.opts, c.ErrOrStderr())
	if err != nil {
		return nil, tenancy.TenantID{}, nil, err
	}
	if mc.tenant != "" {
		id, perr := uuid.Parse(mc.tenant)
		if perr != nil {
			cleanup()
			return nil, tenancy.TenantID{}, nil, usageErr("--tenant %q bukan UUID", mc.tenant)
		}
		if tenancy.TenantID(id) != tid {
			cleanup()
			return nil, tenancy.TenantID{}, nil, usageErr("--tenant %s bukan tenant default; hanya tenant default yang didukung", mc.tenant)
		}
	}
	return be, tid, cleanup, nil
}

func exactArgs(n int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != n {
			return usageErr("argumen: %s", usage)
		}
		return nil
	}
}

type depJSON struct {
	Module   string `json:"module"`
	Version  string `json:"version"`
	Optional bool   `json:"optional"`
}

type moduleJSON struct {
	Key              string    `json:"key"`
	Slug             string    `json:"slug"`
	Name             string    `json:"name"`
	BinaryVersion    string    `json:"binaryVersion"`
	InstalledVersion *string   `json:"installedVersion"`
	Status           string    `json:"status"`
	Required         bool      `json:"required"`
	Compatible       bool      `json:"compatible"`
	Dependencies     []depJSON `json:"dependencies"`
	Dependents       []string  `json:"dependents"`
}

func toModuleJSON(m lifecycle.ModuleState) moduleJSON {
	out := moduleJSON{
		Key: m.Key, Slug: m.Slug, Name: m.Name, BinaryVersion: m.BinaryVersion, Status: string(m.Status),
		Required: m.Required, Compatible: m.Compatible, Dependencies: []depJSON{}, Dependents: []string{},
	}
	if m.InstalledVersion != "" {
		v := m.InstalledVersion
		out.InstalledVersion = &v
	}
	for _, d := range m.Dependencies {
		out.Dependencies = append(out.Dependencies, depJSON{Module: d.Module, Version: d.Version, Optional: d.Optional})
	}
	out.Dependents = append(out.Dependents, m.Dependents...)
	return out
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func depsLabel(m lifecycle.ModuleState) string {
	var parts []string
	for _, d := range m.Dependencies {
		if d.Module == "platform.core" {
			continue
		}
		if d.Optional {
			parts = append(parts, d.Module+"?")
		} else {
			parts = append(parts, d.Module)
		}
	}
	return dash(strings.Join(parts, ","))
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func (mc *modulesCtx) listCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use: "list", Short: "Daftar modul beserta statusnya", Args: exactArgs(0, "list tidak menerima argumen"),
		RunE: func(c *cobra.Command, _ []string) error {
			be, tid, cleanup, err := mc.session(c)
			if err != nil {
				return err
			}
			defer cleanup()
			mods, _, err := be.Modules(c.Context(), tid)
			if err != nil {
				return err
			}
			if asJSON {
				out := make([]moduleJSON, 0, len(mods))
				for _, m := range mods {
					out = append(out, toModuleJSON(m))
				}
				return writeJSON(c.OutOrStdout(), out)
			}
			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "KEY\tBINARY\tINSTALLED\tSTATUS\tDEPENDENCIES\tCOMPATIBLE")
			for _, m := range mods {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", m.Key, dash(m.BinaryVersion), dash(m.InstalledVersion), m.Status, depsLabel(m), yesNo(m.Compatible))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "keluaran JSON")
	return cmd
}

func (mc *modulesCtx) statusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use: "status <key>", Short: "Detail satu modul dan operasi terakhirnya", Args: exactArgs(1, "status <key>"),
		RunE: func(c *cobra.Command, args []string) error {
			be, tid, cleanup, err := mc.session(c)
			if err != nil {
				return err
			}
			defer cleanup()
			m, _, err := be.Module(c.Context(), tid, args[0])
			if err != nil {
				return err
			}
			w := c.OutOrStdout()
			if asJSON {
				v := struct {
					moduleJSON
					LastOperation *operationJSON `json:"lastOperation"`
				}{moduleJSON: toModuleJSON(m)}
				if m.LastOperation != nil {
					o := toOperationJSON(*m.LastOperation)
					v.LastOperation = &o
				}
				return writeJSON(w, v)
			}
			fmt.Fprintf(w, "key: %s\nname: %s\nbinary: %s\ninstalled: %s\nstatus: %s\nstored: %s\nrequired: %s\ncompatible: %s\ndependencies: %s\ndependents: %s\n",
				m.Key, dash(m.Name), dash(m.BinaryVersion), dash(m.InstalledVersion), m.Status, dash(m.Stored),
				yesNo(m.Required), yesNo(m.Compatible), depsLabel(m), dash(strings.Join(m.Dependents, ",")))
			for _, is := range m.Incompatible {
				fmt.Fprintf(w, "incompatible: %s\n", is.String())
			}
			if m.LastOperation == nil {
				fmt.Fprintln(w, "last operation: -")
				return nil
			}
			op := *m.LastOperation
			fmt.Fprintf(w, "last operation: %s %s %s actor=%s started=%s\n", op.ID, op.Operation, op.Status, op.Actor, op.StartedAt.UTC().Format("2006-01-02T15:04:05Z"))
			if op.Error != nil {
				fmt.Fprintf(w, "error: %s (%s): %s\n", op.Error.Code, dash(op.Error.Step), op.Error.Message)
			}
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "STEP\tSTATUS\tDURATION\tERROR")
			for _, s := range op.Steps {
				fmt.Fprintf(tw, "%s\t%s\t%dms\t%s\n", s.Name, s.Status, s.DurationMS, dash(s.Error))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "keluaran JSON")
	return cmd
}

type stepJSON struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	StartedAt  string `json:"startedAt"`
	DurationMS int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

type operationJSON struct {
	ID        string         `json:"id"`
	ModuleKey string         `json:"moduleKey"`
	Operation string         `json:"operation"`
	Status    string         `json:"status"`
	Actor     string         `json:"actor"`
	Steps     []stepJSON     `json:"steps"`
	Error     *store.OpError `json:"failure"`
	StartedAt string         `json:"startedAt"`
}

func toOperationJSON(op store.Operation) operationJSON {
	out := operationJSON{
		ID: op.ID.String(), ModuleKey: op.ModuleKey, Operation: op.Operation, Status: op.Status, Actor: op.Actor,
		Steps: []stepJSON{}, Error: op.Error, StartedAt: op.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	for _, s := range op.Steps {
		out.Steps = append(out.Steps, stepJSON{Name: s.Name, Status: s.Status, StartedAt: s.StartedAt.UTC().Format("2006-01-02T15:04:05Z"), DurationMS: s.DurationMS, Error: s.Error})
	}
	return out
}

// printResults mencetak satu baris per hasil; mengembalikan hasil failed pertama (nil bila tidak ada).
func printResults(w io.Writer, results []lifecycle.Result) *lifecycle.Result {
	var failed *lifecycle.Result
	for i, r := range results {
		switch r.Status {
		case "noop":
			fmt.Fprintf(w, "%s  %s  %s  tidak ada perubahan\n", r.Module, r.Op, r.From)
		default:
			fmt.Fprintf(w, "%s  %s  %s → %s  %s  op=%s\n", r.Module, r.Op, r.From, r.To, r.Status, r.OperationID)
			if r.Status == "failed" && r.Error != nil {
				fmt.Fprintf(w, "  gagal pada langkah %s: %s: %s\n", dash(r.Error.Step), r.Error.Code, r.Error.Message)
				if failed == nil {
					failed = &results[i]
				}
			}
		}
	}
	return failed
}

func failedErr(r *lifecycle.Result) error {
	return &app.ExitError{Code: 4, Err: fmt.Errorf("operasi %s %s gagal (%s): %s; lihat `erpora modules status %s`", r.Op, r.Module, r.Error.Code, r.Error.Message, r.Module)}
}

// confirm membaca y/N dari stdin. Stdin kosong (EOF tanpa data) = konfirmasi hilang (exit 2).
func confirm(c *cobra.Command, prompt string) (bool, error) {
	fmt.Fprint(c.ErrOrStderr(), prompt+" [y/N] ")
	line, err := bufio.NewReader(c.InOrStdin()).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return false, usageErr("konfirmasi diperlukan; pakai --yes")
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}

func (mc *modulesCtx) opCmd(op lifecycle.Op, short string, allowAll, allowDeps, allowEnable, needConfirm bool) *cobra.Command {
	var all, withDeps, enable, yes bool
	use := string(op) + " <key>"
	if allowAll {
		use = string(op) + " <key> | --all"
	}
	cmd := &cobra.Command{
		Use: use, Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			if all && len(args) > 0 {
				return usageErr("--all tidak dapat dipakai bersama key modul")
			}
			if !all && len(args) != 1 {
				return usageErr("argumen: %s", use)
			}
			key := ""
			if len(args) == 1 {
				key = args[0]
			}
			if needConfirm && !yes {
				ok, err := confirm(c, fmt.Sprintf("%s %s? Data modul tetap disimpan.", op, key))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(c.OutOrStdout(), "dibatalkan")
					return nil
				}
			}
			be, tid, cleanup, err := mc.session(c)
			if err != nil {
				return err
			}
			defer cleanup()
			results, err := be.Execute(c.Context(), lifecycle.Request{
				Tenant: tid, Op: op, Key: key, All: all, WithDeps: withDeps, Enable: enable, Actor: cliActor(),
			})
			failed := printResults(c.OutOrStdout(), results)
			if err != nil {
				return err
			}
			if failed != nil {
				return failedErr(failed)
			}
			return nil
		},
	}
	if allowAll {
		cmd.Flags().BoolVar(&all, "all", false, "terapkan ke semua modul terkompilasi")
		cmd.Long = short + "\n\n" + allHelp
	}
	if allowDeps {
		cmd.Flags().BoolVar(&withDeps, "with-deps", false, "sertakan dependensi wajib (urut topologis)")
	}
	if allowEnable {
		cmd.Flags().BoolVar(&enable, "enable", false, "aktifkan juga setelah install")
	}
	if needConfirm {
		cmd.Flags().BoolVarP(&yes, "yes", "y", false, "lewati konfirmasi")
	}
	return cmd
}

func (mc *modulesCtx) retryCmd() *cobra.Command {
	return &cobra.Command{
		Use: "retry <operation-id>", Short: "Ulangi operasi failed (operasi terakhir modul)", Args: exactArgs(1, "retry <operation-id>"),
		RunE: func(c *cobra.Command, args []string) error {
			id, err := uuid.Parse(args[0])
			if err != nil {
				return usageErr("operation-id %q bukan UUID", args[0])
			}
			be, tid, cleanup, err := mc.session(c)
			if err != nil {
				return err
			}
			defer cleanup()
			res, err := be.Retry(c.Context(), tid, id, cliActor())
			if err != nil {
				return err
			}
			if failed := printResults(c.OutOrStdout(), []lifecycle.Result{res}); failed != nil {
				return failedErr(failed)
			}
			return nil
		},
	}
}

func (mc *modulesCtx) historyCmd() *cobra.Command {
	var asJSON bool
	var page, pageSize int
	cmd := &cobra.Command{
		Use: "history <key>", Short: "Riwayat event lifecycle modul (terbaru dulu)", Args: exactArgs(1, "history <key>"),
		RunE: func(c *cobra.Command, args []string) error {
			if page < 1 || pageSize < 1 || pageSize > 200 {
				return usageErr("--page harus >= 1 dan --page-size antara 1 dan 200")
			}
			be, tid, cleanup, err := mc.session(c)
			if err != nil {
				return err
			}
			defer cleanup()
			evs, total, err := be.History(c.Context(), tid, args[0], page, pageSize)
			if err != nil {
				return err
			}
			if asJSON {
				type ev struct {
					ID          string         `json:"id"`
					ModuleKey   string         `json:"moduleKey"`
					Event       string         `json:"event"`
					Actor       string         `json:"actor"`
					OperationID *string        `json:"operationId"`
					Details     map[string]any `json:"details"`
					OccurredAt  string         `json:"occurredAt"`
				}
				out := make([]ev, 0, len(evs))
				for _, e := range evs {
					v := ev{ID: e.ID.String(), ModuleKey: e.ModuleKey, Event: e.Event, Actor: e.Actor, Details: e.Details, OccurredAt: e.OccurredAt.UTC().Format("2006-01-02T15:04:05Z")}
					if v.Details == nil {
						v.Details = map[string]any{}
					}
					if e.OperationID != uuid.Nil {
						s := e.OperationID.String()
						v.OperationID = &s
					}
					out = append(out, v)
				}
				return writeJSON(c.OutOrStdout(), map[string]any{"data": out, "page": map[string]any{"page": page, "pageSize": pageSize, "total": total}})
			}
			tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "TIME\tEVENT\tACTOR\tOPERATION")
			for _, e := range evs {
				op := "-"
				if e.OperationID != uuid.Nil {
					op = e.OperationID.String()
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.OccurredAt.UTC().Format("2006-01-02T15:04:05Z"), e.Event, e.Actor, op)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "keluaran JSON")
	cmd.Flags().IntVar(&page, "page", 1, "halaman (mulai 1)")
	cmd.Flags().IntVar(&pageSize, "page-size", 50, "jumlah event per halaman (1-200)")
	return cmd
}

func (mc *modulesCtx) planCmd() *cobra.Command {
	var all, withDeps, enable bool
	cmd := &cobra.Command{
		Use: "plan <operation> <key> | --all", Short: "Tampilkan rencana operasi tanpa menjalankannya (dry-run)",
		Long: "Tampilkan rencana operasi tanpa lock dan tanpa menulis apa pun. Operasi: install, enable, disable, upgrade, uninstall.\n" +
			"--all hanya untuk install, enable, dan upgrade.\n\n" + allHelp,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) < 1 {
				return usageErr("argumen: plan <operation> <key> | --all")
			}
			op := lifecycle.Op(args[0])
			switch op {
			case lifecycle.OpInstall, lifecycle.OpEnable, lifecycle.OpUpgrade:
			case lifecycle.OpDisable, lifecycle.OpUninstall:
				if all {
					return usageErr("--all hanya untuk install, enable, dan upgrade")
				}
			default:
				return usageErr("operasi %q tidak dikenal; pilih install, enable, disable, upgrade, atau uninstall", args[0])
			}
			key := ""
			switch {
			case all && len(args) != 1:
				return usageErr("--all tidak dapat dipakai bersama key modul")
			case !all && len(args) != 2:
				return usageErr("argumen: plan <operation> <key> | --all")
			case !all:
				key = args[1]
			}
			be, tid, cleanup, err := mc.session(c)
			if err != nil {
				return err
			}
			defer cleanup()
			p, err := be.Plan(c.Context(), lifecycle.Request{Tenant: tid, Op: op, Key: key, All: all, WithDeps: withDeps, Enable: enable})
			if err != nil {
				return err
			}
			return printPlan(c.OutOrStdout(), p)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "rencanakan untuk semua modul (install, enable, upgrade)")
	cmd.Flags().BoolVar(&withDeps, "with-deps", false, "sertakan dependensi wajib")
	cmd.Flags().BoolVar(&enable, "enable", false, "sertakan enable setelah install")
	return cmd
}

// printPlan mencetak rencana; bila ada operasi yang diblokir, mengembalikan error exit 3.
func printPlan(w io.Writer, p lifecycle.Plan) error {
	var blocked *lifecycle.Error
	for i, o := range p.Ops {
		if o.Blocked != nil {
			fmt.Fprintf(w, "%d. %s  %s  DIBLOKIR  %s: %s\n", i+1, o.Module, o.Op, o.Blocked.Code, o.Blocked.Message)
			if blocked == nil {
				blocked = o.Blocked
			}
			continue
		}
		if o.NoOp {
			fmt.Fprintf(w, "%d. %s  %s  %s  tidak ada perubahan\n", i+1, o.Module, o.Op, o.From)
			continue
		}
		fmt.Fprintf(w, "%d. %s  %s  %s → %s\n", i+1, o.Module, o.Op, o.From, o.To)
		if len(o.Steps) > 0 {
			fmt.Fprintf(w, "   langkah: %s\n", strings.Join(o.Steps, ", "))
		}
		if len(o.PendingMigrations) > 0 {
			fmt.Fprintf(w, "   migrasi tertunda: %s\n", strings.Join(o.PendingMigrations, ", "))
		}
		if len(o.RoutesActivated) > 0 {
			fmt.Fprintf(w, "   route diaktifkan: %s\n", strings.Join(o.RoutesActivated, ", "))
		}
		if len(o.RoutesDeactivated) > 0 {
			fmt.Fprintf(w, "   route dinonaktifkan: %s\n", strings.Join(o.RoutesDeactivated, ", "))
		}
	}
	if blocked != nil {
		return blocked
	}
	return nil
}
