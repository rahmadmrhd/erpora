package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/config"
	"erpora/internal/platform/settings"
	settingsstore "erpora/internal/platform/settings/store"
	"erpora/internal/platform/tenancy"
)

// fakeSettingsStore adalah settings.Store berbasis memori (hanya kunci platform.public_url yang dipakai).
type fakeSettingsStore struct{ value json.RawMessage }

func (f *fakeSettingsStore) Get(ctx context.Context, _, _, _ string) (settingsstore.Setting, error) {
	if _, ok := tenancy.FromContext(ctx); !ok {
		return settingsstore.Setting{}, settingsstore.ErrNoTenant
	}
	if f.value == nil {
		return settingsstore.Setting{}, settingsstore.ErrNotFound
	}
	return settingsstore.Setting{Value: f.value}, nil
}

func (f *fakeSettingsStore) Upsert(_ context.Context, s settingsstore.Setting, _ string) error {
	f.value = s.Value
	return nil
}

// Sumber URL publik: settings platform terikat tenant default; development memakai ERP_HTTP_ADDR hanya bila belum
// diisi; production tidak punya default; pemanggil tanpa tenant di context tetap terlayani.
func TestSettingsURLSource(t *testing.T) {
	tid := tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	ctx := context.Background() // sengaja tanpa tenant
	mk := func(cfg config.Config) (PublicURLSource, *settings.Service, *fakeSettingsStore) {
		fs := &fakeSettingsStore{}
		svc := settings.NewService(fs, settings.Options{Production: cfg.Env == config.EnvProduction})
		return newSettingsURLSource(cfg, svc, tid), svc, fs
	}

	dev, svc, _ := mk(config.Config{Env: config.EnvDevelopment, HTTPAddr: "127.0.0.1:8080"})
	if u, err := dev.PublicURL(ctx); err != nil || u != "http://127.0.0.1:8080" {
		t.Fatalf("development tanpa setting = %q, %v", u, err)
	}
	wild, _, _ := mk(config.Config{Env: config.EnvDevelopment, HTTPAddr: "0.0.0.0:9000"})
	if u, _ := wild.PublicURL(ctx); u != "http://localhost:9000" {
		t.Fatalf("alamat wildcard = %q", u)
	}
	none, _, _ := mk(config.Config{Env: config.EnvDevelopment})
	if u, _ := none.PublicURL(ctx); u != fallbackDevURL {
		t.Fatalf("tanpa ERP_HTTP_ADDR = %q", u)
	}
	// Setting yang sudah diisi menang atas default development.
	if err := svc.SetPublicURL(tenancy.WithTenant(ctx, tid), "https://erp.example.com", "uji"); err != nil {
		t.Fatal(err)
	}
	if u, err := dev.PublicURL(ctx); err != nil || u != "https://erp.example.com" {
		t.Fatalf("setting terisi = %q, %v", u, err)
	}
	// Production tanpa setting: tidak ada default (ERP_PUBLIC_URL ditolak pemilik produk 3 Nov 2026).
	prod, _, _ := mk(config.Config{Env: config.EnvProduction, HTTPAddr: "0.0.0.0:8080"})
	if _, err := prod.PublicURL(ctx); !errors.Is(err, ErrPublicURLNotConfigured) || !errors.Is(err, settings.ErrNotConfigured) {
		t.Fatalf("production tanpa settings harus ErrNotConfigured, err = %v", err)
	}
}

func TestCheckPublicURL_ProductionFailsWithoutURL(t *testing.T) {
	ctx := context.Background()
	notSet := publicURLFunc(func(context.Context) (string, error) { return "", ErrPublicURLNotConfigured })
	if err := checkPublicURL(ctx, config.Config{Env: config.EnvProduction}, notSet); !errors.Is(err, ErrPublicURLNotConfigured) {
		t.Fatalf("production: err = %v", err)
	}
	if err := checkPublicURL(ctx, config.Config{Env: config.EnvDevelopment}, notSet); err != nil {
		t.Fatalf("development tidak boleh gagal: %v", err)
	}
	set := publicURLFunc(func(context.Context) (string, error) { return "https://erp.example.com", nil })
	if err := checkPublicURL(ctx, config.Config{Env: config.EnvProduction}, set); err != nil {
		t.Fatalf("production dengan URL: %v", err)
	}
}

// Pemeriksa Origin dihitung malas dari sumber: perubahan URL berlaku tanpa restart, dan galat sumber menolak semua.
func TestLazyOriginChecker_FollowsSourceAndFailsClosed(t *testing.T) {
	cur, curErr := "https://a.example.com", error(nil)
	src := publicURLFunc(func(context.Context) (string, error) { return cur, curErr })
	chk := lazyOriginChecker(config.Config{Env: config.EnvProduction}, src)
	if !chk("https://a.example.com") || chk("https://b.example.com") || chk("") || chk("null") {
		t.Fatal("origin awal salah")
	}
	cur = "https://b.example.com/app"
	if !chk("https://b.example.com") || chk("https://a.example.com") {
		t.Fatal("perubahan URL harus berlaku tanpa restart")
	}
	curErr = ErrPublicURLNotConfigured
	if chk("https://b.example.com") {
		t.Fatal("galat sumber harus menolak semua origin")
	}
	curErr, cur = nil, "bukan url"
	if chk("https://b.example.com") {
		t.Fatal("URL tak sah harus menolak semua origin")
	}
}

func TestAllowedOrigins_DevelopmentAddsLoopbackAliases(t *testing.T) {
	got := allowedOrigins(config.Config{Env: config.EnvDevelopment}, "http://127.0.0.1:8080")
	want := map[string]bool{"http://127.0.0.1:8080": true, "http://localhost:8080": true}
	if len(got) != 3 {
		t.Fatalf("origins = %v", got)
	}
	for _, o := range got {
		if !want[o] {
			t.Errorf("origin tak terduga %q", o)
		}
	}
	if got := allowedOrigins(config.Config{Env: config.EnvProduction}, "https://erp.example.com"); len(got) != 1 || got[0] != "https://erp.example.com" {
		t.Fatalf("production = %v", got)
	}
}

// waitWithin menunggu pekerjaan latar sampai batas ctx: selesai tepat waktu -> true; macet -> false tanpa menggantung.
func TestWaitWithin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	finished := false
	if !waitWithin(ctx, func() { time.Sleep(10 * time.Millisecond); finished = true }) || !finished {
		t.Fatal("pekerjaan yang selesai tepat waktu harus ditunggu")
	}
	release := make(chan struct{})
	defer close(release)
	short, cancel2 := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel2()
	start := time.Now()
	if waitWithin(short, func() { <-release }) {
		t.Fatal("pekerjaan macet harus dilaporkan belum selesai")
	}
	if time.Since(start) > time.Second {
		t.Fatal("waitWithin menggantung melewati batas ctx")
	}
}
