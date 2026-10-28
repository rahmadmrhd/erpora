package lifecycle

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"sync"
	"testing"

	"erpora/internal/platform/modules/registry"
	"erpora/sdk/module"
)

const (
	fxPlatformVersion = "0.1.0"
	fxSDKVersion      = "0.1.0"
)

// fxRec merekam urutan panggilan hook lintas modul ("<key>:<Hook>").
type fxRec struct {
	mu    sync.Mutex
	calls []string
}

func (r *fxRec) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, s)
}

func (r *fxRec) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *fxRec) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = nil
}

type fxSpec struct {
	key, slug, version string
	deps               []module.Dependency // dependensi platform.core ditambahkan otomatis
	required           bool
	platformC          string // default ^0.1
	migrations         fs.FS  // non-nil => strategy versioned + MigrationSource
}

// fxFS adalah filesystem migrasi yang dapat ditukar saat test berjalan.
type fxFS struct {
	mu sync.Mutex
	f  fs.FS
}

func (x *fxFS) get() fs.FS {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.f
}

func (x *fxFS) set(f fs.FS) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.f = f
}

// fxModule adalah modul uji dengan hook yang dapat diatur dan merekam panggilan.
type fxModule struct {
	mf    module.Manifest
	rec   *fxRec
	mig   *fxFS
	mu    sync.Mutex
	hooks map[string]func(ctx context.Context, env module.Env) error
	ctxs  map[string]context.Context
	envs  map[string]module.Env
	upg   [][2]string
}

func newFx(rec *fxRec, s fxSpec) *fxModule {
	if s.version == "" {
		s.version = "1.0.0"
	}
	if s.slug == "" {
		for i := len(s.key) - 1; i >= 0; i-- {
			if s.key[i] == '.' {
				s.slug = s.key[i+1:]
				break
			}
		}
	}
	if s.platformC == "" {
		s.platformC = "^0.1"
	}
	strategy := "none"
	if s.migrations != nil {
		strategy = "versioned"
	}
	deps := append([]module.Dependency{{Module: registry.PlatformKey, Version: "^0.1"}}, s.deps...)
	doc := map[string]any{
		"schema_version": "1", "key": s.key, "slug": s.slug, "name": s.slug, "version": s.version,
		"publisher":     map[string]string{"id": "acme", "name": "Acme"},
		"trust":         "builtin",
		"compatibility": map[string]string{"platform": s.platformC, "sdk": "^0.1"},
		"dependencies":  deps,
		"capabilities":  []string{s.slug + ".items.read"},
		"migrations":    map[string]string{"strategy": strategy},
		"offline":       map[string]string{"mode": "online-only"},
	}
	if s.required {
		doc["required"] = true
	}
	b, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	f := &fxModule{
		mf: module.MustParseManifest(b), rec: rec,
		hooks: map[string]func(context.Context, module.Env) error{},
		ctxs:  map[string]context.Context{}, envs: map[string]module.Env{},
	}
	if s.migrations != nil {
		f.mig = &fxFS{f: s.migrations}
	}
	return f
}

// module mengembalikan module.Module yang sesuai (dengan MigrationSource bila versioned).
func (f *fxModule) module() module.Module {
	if f.mig != nil {
		return fxMigModule{f}
	}
	return f
}

func (f *fxModule) Manifest() module.Manifest { return f.mf }

func (f *fxModule) Register(r module.Registrar) error {
	slug := f.mf.Slug
	r.Handle("GET", "/items", slug+".items.read", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	return nil
}

// on mengatur perilaku hook (nama: Install, Upgrade, OnEnable, OnDisable, Health).
func (f *fxModule) on(name string, fn func(ctx context.Context, env module.Env) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hooks[name] = fn
}

func (f *fxModule) call(ctx context.Context, name string, env module.Env) error {
	f.rec.add(f.mf.Key + ":" + name)
	f.mu.Lock()
	f.ctxs[name], f.envs[name] = ctx, env
	fn := f.hooks[name]
	f.mu.Unlock()
	if fn == nil {
		return nil
	}
	return fn(ctx, env)
}

func (f *fxModule) seen(name string) (context.Context, module.Env) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ctxs[name], f.envs[name]
}

func (f *fxModule) Install(ctx context.Context, env module.Env) error {
	return f.call(ctx, "Install", env)
}

func (f *fxModule) OnEnable(ctx context.Context, env module.Env) error {
	return f.call(ctx, "OnEnable", env)
}

func (f *fxModule) OnDisable(ctx context.Context, env module.Env) error {
	return f.call(ctx, "OnDisable", env)
}

func (f *fxModule) Health(ctx context.Context, env module.Env) error {
	return f.call(ctx, "Health", env)
}

func (f *fxModule) Upgrade(ctx context.Context, env module.Env, from, to string) error {
	f.mu.Lock()
	f.upg = append(f.upg, [2]string{from, to})
	f.mu.Unlock()
	return f.call(ctx, "Upgrade", env)
}

// fxMigModule menambah module.MigrationSource pada fxModule.
type fxMigModule struct{ *fxModule }

func (f fxMigModule) Migrations() fs.FS { return f.mig.get() }

func buildFxRegistry(t *testing.T, mods ...*fxModule) *registry.Registry {
	t.Helper()
	list := make([]module.Module, 0, len(mods))
	for _, m := range mods {
		list = append(list, m.module())
	}
	reg, err := registry.Build(registry.Options{PlatformVersion: fxPlatformVersion, SDKVersion: fxSDKVersion}, list...)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func dep(key string) module.Dependency { return module.Dependency{Module: key, Version: "^1.0"} }

func optDep(key string) module.Dependency {
	return module.Dependency{Module: key, Version: "^1.0", Optional: true}
}

// moduleDep mempersingkat deklarasi dependensi pada fxSpec.
type moduleDep = module.Dependency
