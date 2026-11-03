package app

import (
	"context"
	"errors"
	"testing"

	"erpora/internal/platform/config"
)

func TestDefaultPublicURLSource(t *testing.T) {
	ctx := context.Background()
	dev := config.Config{Env: config.EnvDevelopment, HTTPAddr: "127.0.0.1:8080"}
	if u, err := defaultPublicURLSource(dev).PublicURL(ctx); err != nil || u != "http://127.0.0.1:8080" {
		t.Fatalf("development = %q, %v", u, err)
	}
	wild := config.Config{Env: config.EnvDevelopment, HTTPAddr: "0.0.0.0:9000"}
	if u, _ := defaultPublicURLSource(wild).PublicURL(ctx); u != "http://localhost:9000" {
		t.Fatalf("alamat wildcard = %q", u)
	}
	// ERP_PUBLIC_URL ditolak pemilik produk 3 Nov 2026: tanpa settings, production belum punya sumber.
	prod := config.Config{Env: config.EnvProduction}
	if _, err := defaultPublicURLSource(prod).PublicURL(ctx); !errors.Is(err, ErrPublicURLNotConfigured) {
		t.Fatalf("production tanpa settings harus ErrPublicURLNotConfigured, err = %v", err)
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
