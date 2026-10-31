package desks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"erpora/internal/platform/desks/store"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/module"
)

// ErrNoTenant berarti context tidak membawa tenant (middleware tenant tidak berjalan); tenant tidak pernah datang
// dari parameter (CLAUDE.md: parameter kontrak tidak boleh membawa tenantId/userId).
var ErrNoTenant = errors.New("desks: tenant tidak ada di context")

// StatusSource dipenuhi *runtime.Snapshot: status modul untuk satu tenant.
type StatusSource interface {
	Status(t tenancy.TenantID, key string) runtime.Status
}

// OverrideSource dipenuhi *store.Store: timpaan Desk tingkat situs milik tenant (hanya baca).
type OverrideSource interface {
	ListSite(ctx context.Context, t tenancy.TenantID) ([]store.Override, error)
}

// Deps adalah dependensi Service. Authz nil menolak semua permission (fail closed); Overrides nil = tanpa timpaan.
type Deps struct {
	Registry  *registry.Registry
	Status    StatusSource
	Authz     runtime.Authorizer
	Overrides OverrideSource
	Log       *slog.Logger
}

// Service menyusun daftar Desk per pengguna.
type Service struct {
	reg       *registry.Registry
	status    StatusSource
	authz     runtime.Authorizer
	overrides OverrideSource
	log       *slog.Logger
}

// NewService membuat Service.
func NewService(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{reg: d.Registry, status: d.Status, authz: d.Authz, overrides: d.Overrides, log: log}
}

func (s *Service) allow(ctx context.Context, permission string) bool {
	return s.authz != nil && s.authz.Allow(ctx, permission)
}

// List mengembalikan Desk yang boleh dilihat prinsipal pada ctx untuk tenant pada ctx: Desk Platform lebih dulu,
// lalu Desk modul menurut urutan registry. Modul yang tidak enabled pada tenant tidak membentuk Desk; menu tanpa
// izin tidak dikirim; grup tanpa anak dibuang; Desk tanpa menu non-Dashboard yang diizinkan dibuang (D-5);
// Dashboard selalu menu pertama dan terkunci. Hasil baru pada tiap pemanggilan (tidak dibagi antar pemanggil).
func (s *Service) List(ctx context.Context) (View, error) {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return View{}, ErrNoTenant
	}
	cfgs, err := s.siteConfigs(ctx, t)
	if err != nil {
		return View{}, err
	}
	entries := s.reg.Entries()
	ordered := make([]registry.Entry, 0, len(entries))
	for _, e := range entries { // Platform pertama
		if e.Manifest.Key == registry.PlatformKey {
			ordered = append(ordered, e)
		}
	}
	for _, e := range entries {
		if e.Manifest.Key != registry.PlatformKey {
			ordered = append(ordered, e)
		}
	}

	desks := []Desk{}
	for _, e := range ordered {
		m := e.Manifest
		if m.Desk == nil {
			continue
		}
		if m.Key != registry.PlatformKey && (s.status == nil || s.status.Status(t, m.Key) != runtime.StatusEnabled) {
			continue
		}
		if d, ok := s.build(ctx, m, cfgs[m.Slug]); ok {
			desks = append(desks, d)
		}
	}

	raw, err := json.Marshal(desks)
	if err != nil {
		return View{}, fmt.Errorf("desks: serialisasi: %w", err)
	}
	sum := sha256.Sum256(raw)
	rev := hex.EncodeToString(sum[:16])
	return View{Desks: desks, Revision: rev, ETag: `"` + rev + `"`}, nil
}

// siteConfigs memuat timpaan situs tenant per id Desk. Baris dengan config tidak sah dilewati (dicatat): config
// tidak tepercaya dan hanya kosmetik, sehingga tidak boleh menggagalkan seluruh daftar.
func (s *Service) siteConfigs(ctx context.Context, t tenancy.TenantID) (map[string]Config, error) {
	out := map[string]Config{}
	if s.overrides == nil {
		return out, nil
	}
	rows, err := s.overrides.ListSite(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("desks: muat timpaan: %w", err)
	}
	for _, r := range rows {
		cfg, err := ParseConfig(r.Config)
		if err != nil {
			s.log.WarnContext(ctx, "timpaan Desk dilewati: config tidak sah", "desk", r.DeskID, "error", err)
			continue
		}
		out[r.DeskID] = cfg
	}
	return out, nil
}

// build menyusun satu Desk; ok false bila Desk tidak punya menu non-Dashboard yang diizinkan dan tidak disembunyikan.
func (s *Service) build(ctx context.Context, m module.Manifest, cfg Config) (Desk, bool) {
	d := m.Desk
	slug := m.Slug
	menus := make([]Menu, 0, len(d.Menus))
	for _, dm := range d.Menus {
		if len(dm.Children) == 0 {
			if s.allow(ctx, dm.Permission) {
				menus = append(menus, menuItem(slug, dm))
			}
			continue
		}
		var kids []Menu
		for _, c := range dm.Children {
			if s.allow(ctx, c.Permission) {
				kids = append(kids, menuItem(slug, c))
			}
		}
		if len(kids) > 0 {
			menus = append(menus, Menu{Type: TypeGroup, ID: dm.ID, Label: MessageRef{NS: slug, Key: dm.Label}, Icon: dm.Icon, Children: kids})
		}
	}
	menus = cfg.apply(menus)
	if len(menus) == 0 { // grup tidak pernah kosong, jadi len(menus) > 0 berarti ada item non-Dashboard
		return Desk{}, false
	}

	dash := Menu{
		Type: TypeItem, ID: DashboardID, Label: MessageRef{NS: "platform", Key: DashboardLabelKey},
		To: "/" + slug, Icon: DashboardDefaultIcon, Locked: true, OwnerDesk: slug,
	}
	if db := d.Dashboard; db != nil {
		if db.Label != "" {
			dash.Label = MessageRef{NS: slug, Key: db.Label}
		}
		if db.Icon != "" {
			dash.Icon = db.Icon
		}
	}
	out := Desk{
		ID: slug, Source: SourceModule, ModuleKey: m.Key, Label: MessageRef{NS: slug, Key: d.Label}, Icon: d.Icon,
		Menus: append([]Menu{dash}, menus...),
	}
	if m.Key == registry.PlatformKey {
		out.Source, out.ModuleKey = SourcePlatform, ""
	}
	return out, true
}

func menuItem(slug string, dm module.DeskMenu) Menu {
	return Menu{
		Type: TypeItem, ID: dm.ID, Label: MessageRef{NS: slug, Key: dm.Label}, To: "/" + slug + dm.To,
		Icon: dm.Icon, OwnerDesk: slug,
	}
}
