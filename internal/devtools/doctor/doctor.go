// Package doctor memeriksa lingkungan developer (go, bun, task, docker, PostgreSQL, .env) untuk `erpora doctor`.
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"

	"erpora/internal/platform/config"
)

// minTask adalah versi go-task minimum (sama dengan yang dipin di Taskfile.yml).
const minTask = "3.54.0"

// Status hasil satu pemeriksaan.
type Status string

// Nilai Status.
const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// Result adalah hasil satu pemeriksaan.
type Result struct {
	Name   string
	Status Status
	Detail string
}

// Probe adalah akses ke sistem; fake pada test.
type Probe interface {
	LookPath(name string) (string, error)
	Output(ctx context.Context, name string, args ...string) (string, error)
	FileExists(path string) bool
}

// Inputs adalah masukan Run.
type Inputs struct {
	Root, EnvFile string // EnvFile kosong = <Root>/.env
	DSN           string // kosong = ERP_DATABASE_URL belum diatur (kecuali ConfigErr terisi)
	// ConfigErr adalah pesan galat pemuatan konfigurasi selain DSN yang belum diatur (mis. ERP_ENV tidak valid, env-file rusak);
	// bila terisi, koneksi tidak diperiksa dan pesan ini yang ditampilkan.
	ConfigErr string
	Ping      func(ctx context.Context, dsn string) error
}

// ExecProbe adalah Probe sungguhan (os/exec + os.Stat).
type ExecProbe struct{}

// LookPath mencari executable di PATH.
func (ExecProbe) LookPath(name string) (string, error) { return exec.LookPath(name) }

// Output menjalankan perintah dan mengembalikan stdout.
func (ExecProbe) Output(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // dipanggil hanya dengan nama tool tetap (go, bun, task, docker)
	return string(out), err
}

// FileExists melaporkan apakah path ada.
func (ExecProbe) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

var (
	goVersionRe   = regexp.MustCompile(`go(\d+\.\d+(?:\.\d+)?)`)
	plainVersion  = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?`)
	bunPinPattern = regexp.MustCompile(`"packageManager"\s*:\s*"bun@([^"]+)"`)
)

// Run menjalankan semua pemeriksaan dengan urutan tetap: go, bun, task, docker, postgres, .env.
func Run(ctx context.Context, in Inputs, p Probe) []Result {
	return []Result{checkGo(ctx, in, p), checkBun(ctx, in, p), checkTask(ctx, p), checkDocker(ctx, p), checkPostgres(ctx, in), checkEnvFile(in, p)}
}

// Failed melaporkan apakah ada hasil fail.
func Failed(rs []Result) bool {
	for _, r := range rs {
		if r.Status == StatusFail {
			return true
		}
	}
	return false
}

func semverOf(v string) string { return "v" + strings.TrimPrefix(v, "v") }

func checkGo(ctx context.Context, in Inputs, p Probe) Result {
	r := Result{Name: "go"}
	data, err := os.ReadFile(filepath.Join(in.Root, "go.mod")) //nolint:gosec // root repo dari pemanggil
	if err != nil {
		return Result{Name: "go", Status: StatusFail, Detail: "go.mod root tidak terbaca: " + err.Error()}
	}
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil || f.Toolchain == nil {
		return Result{Name: "go", Status: StatusFail, Detail: "direktif toolchain di go.mod root tidak ditemukan"}
	}
	want := strings.TrimPrefix(f.Toolchain.Name, "go")
	if _, err := p.LookPath("go"); err != nil {
		return Result{Name: "go", Status: StatusFail, Detail: "go tidak ditemukan di PATH; butuh go" + want}
	}
	out, err := p.Output(ctx, "go", "version")
	if err != nil {
		return Result{Name: "go", Status: StatusFail, Detail: "go version gagal: " + err.Error()}
	}
	m := goVersionRe.FindStringSubmatch(out)
	if m == nil {
		return Result{Name: "go", Status: StatusFail, Detail: "versi go tidak terbaca dari: " + strings.TrimSpace(out)}
	}
	if semver.Compare(semverOf(m[1]), semverOf(want)) < 0 {
		r.Status, r.Detail = StatusFail, fmt.Sprintf("go%s lebih rendah dari toolchain go%s", m[1], want)
		return r
	}
	r.Status, r.Detail = StatusOK, "go"+m[1]
	return r
}

func checkBun(ctx context.Context, in Inputs, p Probe) Result {
	data, err := os.ReadFile(filepath.Join(in.Root, "package.json")) //nolint:gosec // root repo dari pemanggil
	if err != nil {
		return Result{Name: "bun", Status: StatusFail, Detail: "package.json root tidak terbaca: " + err.Error()}
	}
	m := bunPinPattern.FindSubmatch(data)
	if m == nil {
		return Result{Name: "bun", Status: StatusFail, Detail: "packageManager bun@<versi> tidak ditemukan di package.json"}
	}
	want := string(m[1])
	if _, err := p.LookPath("bun"); err != nil {
		return Result{Name: "bun", Status: StatusFail, Detail: "bun tidak ditemukan di PATH; butuh bun " + want}
	}
	out, err := p.Output(ctx, "bun", "--version")
	if err != nil {
		return Result{Name: "bun", Status: StatusFail, Detail: "bun --version gagal: " + err.Error()}
	}
	got := plainVersion.FindString(out)
	if got != want {
		return Result{Name: "bun", Status: StatusWarn, Detail: fmt.Sprintf("bun %s berbeda dari versi terpin %s", got, want)}
	}
	return Result{Name: "bun", Status: StatusOK, Detail: "bun " + got}
}

func checkTask(ctx context.Context, p Probe) Result {
	if _, err := p.LookPath("task"); err != nil {
		return Result{Name: "task", Status: StatusFail, Detail: "task tidak ditemukan di PATH; butuh go-task >= " + minTask}
	}
	out, err := p.Output(ctx, "task", "--version")
	if err != nil {
		return Result{Name: "task", Status: StatusFail, Detail: "task --version gagal: " + err.Error()}
	}
	got := plainVersion.FindString(out)
	if got == "" || semver.Compare(semverOf(got), semverOf(minTask)) < 0 {
		return Result{Name: "task", Status: StatusFail, Detail: fmt.Sprintf("task %q lebih rendah dari %s", got, minTask)}
	}
	return Result{Name: "task", Status: StatusOK, Detail: "task " + got}
}

func checkDocker(ctx context.Context, p Probe) Result {
	const need = "hanya diperlukan untuk `task db:up` dan test integrasi"
	if _, err := p.LookPath("docker"); err != nil {
		return Result{Name: "docker", Status: StatusWarn, Detail: "docker tidak ditemukan; " + need}
	}
	if _, err := p.Output(ctx, "docker", "info"); err != nil {
		return Result{Name: "docker", Status: StatusWarn, Detail: "daemon docker tidak berjalan; " + need}
	}
	return Result{Name: "docker", Status: StatusOK, Detail: "daemon berjalan"}
}

func checkPostgres(ctx context.Context, in Inputs) Result {
	if in.ConfigErr != "" {
		return Result{Name: "postgres", Status: StatusWarn, Detail: "konfigurasi tidak dapat dimuat (koneksi tidak diperiksa): " + in.ConfigErr}
	}
	if strings.TrimSpace(in.DSN) == "" {
		return Result{Name: "postgres", Status: StatusWarn, Detail: "ERP_DATABASE_URL belum diatur"}
	}
	host, secrets := config.DSNParts(in.DSN)
	if in.Ping == nil {
		return Result{Name: "postgres", Status: StatusWarn, Detail: "tidak diperiksa (host=" + host + ")"}
	}
	if err := in.Ping(ctx, in.DSN); err != nil {
		msg := err.Error()
		for _, s := range secrets {
			if s != "" {
				msg = strings.ReplaceAll(msg, s, "****")
			}
		}
		return Result{Name: "postgres", Status: StatusFail, Detail: "host=" + host + ": " + msg}
	}
	return Result{Name: "postgres", Status: StatusOK, Detail: "terjangkau (host=" + host + ")"}
}

func checkEnvFile(in Inputs, p Probe) Result {
	path := in.EnvFile
	if path == "" {
		path = filepath.Join(in.Root, ".env")
	}
	if p.FileExists(path) {
		return Result{Name: ".env", Status: StatusOK, Detail: path}
	}
	return Result{Name: ".env", Status: StatusWarn, Detail: path + " tidak ada; salin .env.example ke .env"}
}
