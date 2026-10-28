package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeProbe struct {
	paths   map[string]bool   // tool yang ada di PATH
	outputs map[string]string // "name arg..." -> stdout
	failing map[string]bool   // "name arg..." -> exit non-nol
	files   map[string]bool
}

func (f fakeProbe) LookPath(name string) (string, error) {
	if f.paths[name] {
		return "/bin/" + name, nil
	}
	return "", errors.New("not found")
}

func (f fakeProbe) Output(_ context.Context, name string, args ...string) (string, error) {
	k := strings.Join(append([]string{name}, args...), " ")
	if f.failing[k] {
		return "", errors.New("exit status 1")
	}
	return f.outputs[k], nil
}

func (f fakeProbe) FileExists(path string) bool { return f.files[path] }

func goodProbe(root string) fakeProbe {
	return fakeProbe{
		paths: map[string]bool{"go": true, "bun": true, "task": true, "docker": true},
		outputs: map[string]string{
			"go version":     "go version go1.27.3 windows/amd64\n",
			"bun --version":  "1.4.2\n",
			"task --version": "Task version: v3.54.0 (h1:abc)\n",
			"docker info":    "Server: ok\n",
		},
		files: map[string]bool{filepath.Join(root, ".env"): true},
	}
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module erpora\n\ngo 1.27\n\ntoolchain go1.27.3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{\n  \"packageManager\": \"bun@1.4.2\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func statusOf(rs []Result, name string) (Status, string) {
	for _, r := range rs {
		if r.Name == name {
			return r.Status, r.Detail
		}
	}
	return "", ""
}

const secretDSN = "postgres://erp:S3cr3tPass@db.local:5432/erp"

func TestRun_Table(t *testing.T) {
	ok := func(context.Context, string) error { return nil }
	leaky := func(_ context.Context, dsn string) error { return errors.New("dial gagal untuk " + dsn) }
	tests := []struct {
		name   string
		mutate func(p *fakeProbe)
		in     func(root string) Inputs
		want   map[string]Status
		failed bool
	}{
		{"semua ok", func(*fakeProbe) {}, func(r string) Inputs { return Inputs{Root: r, DSN: secretDSN, Ping: ok} },
			map[string]Status{"go": StatusOK, "bun": StatusOK, "task": StatusOK, "docker": StatusOK, "postgres": StatusOK, ".env": StatusOK}, false},
		{"go lebih rendah", func(p *fakeProbe) { p.outputs["go version"] = "go version go1.26.9 linux/amd64" },
			func(r string) Inputs { return Inputs{Root: r, DSN: secretDSN, Ping: ok} }, map[string]Status{"go": StatusFail}, true},
		{"go lebih tinggi", func(p *fakeProbe) { p.outputs["go version"] = "go version go1.28.0 linux/amd64" },
			func(r string) Inputs { return Inputs{Root: r, DSN: secretDSN, Ping: ok} }, map[string]Status{"go": StatusOK}, false},
		{"go tidak ada", func(p *fakeProbe) { delete(p.paths, "go") },
			func(r string) Inputs { return Inputs{Root: r, DSN: secretDSN, Ping: ok} }, map[string]Status{"go": StatusFail}, true},
		{"bun beda versi", func(p *fakeProbe) { p.outputs["bun --version"] = "1.3.0" },
			func(r string) Inputs { return Inputs{Root: r, DSN: secretDSN, Ping: ok} }, map[string]Status{"bun": StatusWarn}, false},
		{"bun tidak ada", func(p *fakeProbe) { delete(p.paths, "bun") },
			func(r string) Inputs { return Inputs{Root: r, DSN: secretDSN, Ping: ok} }, map[string]Status{"bun": StatusFail}, true},
		{"task terlalu rendah", func(p *fakeProbe) { p.outputs["task --version"] = "Task version: v3.40.1" },
			func(r string) Inputs { return Inputs{Root: r, DSN: secretDSN, Ping: ok} }, map[string]Status{"task": StatusFail}, true},
		{"task tidak ada", func(p *fakeProbe) { delete(p.paths, "task") },
			func(r string) Inputs { return Inputs{Root: r, DSN: secretDSN, Ping: ok} }, map[string]Status{"task": StatusFail}, true},
		{"docker mati", func(p *fakeProbe) { p.failing["docker info"] = true },
			func(r string) Inputs { return Inputs{Root: r, DSN: secretDSN, Ping: ok} }, map[string]Status{"docker": StatusWarn}, false},
		{"docker tidak ada", func(p *fakeProbe) { delete(p.paths, "docker") },
			func(r string) Inputs { return Inputs{Root: r, DSN: secretDSN, Ping: ok} }, map[string]Status{"docker": StatusWarn}, false},
		{"dsn kosong", func(*fakeProbe) {}, func(r string) Inputs { return Inputs{Root: r, Ping: ok} }, map[string]Status{"postgres": StatusWarn}, false},
		{"ping gagal", func(*fakeProbe) {}, func(r string) Inputs { return Inputs{Root: r, DSN: secretDSN, Ping: leaky} },
			map[string]Status{"postgres": StatusFail}, true},
		{".env tidak ada", func(p *fakeProbe) { p.files = map[string]bool{} },
			func(r string) Inputs { return Inputs{Root: r, DSN: secretDSN, Ping: ok} }, map[string]Status{".env": StatusWarn}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := fixtureRoot(t)
			p := goodProbe(root)
			p.failing = map[string]bool{}
			tc.mutate(&p)
			rs := Run(context.Background(), tc.in(root), p)
			if len(rs) != 6 {
				t.Fatalf("harus 6 hasil, got %d: %+v", len(rs), rs)
			}
			for name, want := range tc.want {
				if got, detail := statusOf(rs, name); got != want {
					t.Errorf("%s = %q (%s), want %q", name, got, detail, want)
				}
			}
			if Failed(rs) != tc.failed {
				t.Errorf("Failed = %v, want %v: %+v", Failed(rs), tc.failed, rs)
			}
			for _, r := range rs {
				if strings.Contains(r.Detail, "S3cr3tPass") {
					t.Errorf("detail %s membocorkan password: %q", r.Name, r.Detail)
				}
			}
		})
	}
}

// Catatan reviewer: kegagalan config.Load karena hal lain (mis. ERP_ENV tidak valid, env-file rusak) tidak boleh
// dilaporkan sebagai "ERP_DATABASE_URL belum diatur"; pesan aslinya yang ditampilkan (status warn).
func TestRun_ConfigLoadErrorReportedAccurately(t *testing.T) {
	root := fixtureRoot(t)
	rs := Run(context.Background(), Inputs{Root: root, ConfigErr: "ERP_ENV: harus development atau production"}, goodProbe(root))
	st, detail := statusOf(rs, "postgres")
	if st != StatusWarn || !strings.Contains(detail, "ERP_ENV: harus development atau production") || strings.Contains(detail, "belum diatur") {
		t.Fatalf("status=%s detail=%q", st, detail)
	}
	// Tanpa galat config dan tanpa DSN tetap "belum diatur".
	rs = Run(context.Background(), Inputs{Root: root}, goodProbe(root))
	if _, d := statusOf(rs, "postgres"); !strings.Contains(d, "belum diatur") {
		t.Errorf("detail = %q", d)
	}
}

func TestRun_PostgresDetailRedactsPassword(t *testing.T) {
	root := fixtureRoot(t)
	p := goodProbe(root)
	rs := Run(context.Background(), Inputs{Root: root, DSN: secretDSN, Ping: func(_ context.Context, dsn string) error {
		return errors.New("gagal konek " + dsn)
	}}, p)
	st, detail := statusOf(rs, "postgres")
	if st != StatusFail || strings.Contains(detail, "S3cr3tPass") || !strings.Contains(detail, "db.local") {
		t.Fatalf("status=%s detail=%q", st, detail)
	}
}
