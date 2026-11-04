// Package authztest berisi helper authz untuk test modul: masker yang mengizinkan semua field dan mesin
// otorisasi uji dengan hak yang dinyatakan secara deklaratif (tanpa database, tanpa platform).
//
// Mesin uji mengikuti semantik FR-111/FR-112 yang sama dengan mesin platform: hak efektif adalah union
// per baris (scope baris lain tidak pernah meminjamkan cakupannya ke aksi lain), scope baris berupa
// token `all`/`owner`/`assigned:<dimensi>`/`custom:<key>` yang digabung AND, antarbaris OR, dan semuanya
// gagal tertutup. Hanya untuk test; tidak pernah dipakai di produksi.
package authztest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"erpora/sdk/authz"
	"erpora/sdk/identity"
)

type allowAll struct{}

func (allowAll) MaskFields(_ context.Context, _ string, fields map[string]any) map[string]any {
	return maps.Clone(fields)
}

// AllowAllFields mengembalikan salinan field apa adanya; hanya untuk test (tidak pernah di produksi).
var AllowAllFields authz.FieldMasker = allowAll{}

// All adalah scope `all` (dinyatakan eksplisit; daftar scope kosong tidak sah).
var All = []string{"all"}

// Resource mendeskripsikan resource uji dalam bentuk manifest.
type Resource struct {
	Key string // "<slug>.<nama_snake>"
	// OwnerField: kolom pemilik untuk scope `owner`; kosong berarti "created_by".
	OwnerField string
	// ScopeFields: dimensi ke nama field/kolom, untuk scope `assigned:<dimensi>`.
	ScopeFields map[string]string
	// Groups: grup field; tepat satu bertanda Default. Field di luar semua grup masuk grup default.
	Groups []Group
}

// Group adalah satu grup field resource.
type Group struct {
	Key     string
	Fields  []string // nama snake_case manifest
	Default bool
}

// Grant adalah satu baris matriks role: aksi pada resource dengan scope (token FR-111, digabung AND).
type Grant struct {
	Resource string
	Actions  []string
	Scope    []string // wajib tidak kosong dan semua token sah; selain itu baris ditolak (tidak pernah "semua")
}

// User adalah hak efektif satu pengguna uji (gabungan semua role-nya).
type User struct {
	ID     string
	Grants []Grant
	// FieldAccess: resource ke grup ke akses. Grup default minimal mengikuti aksi (update: write, read:
	// read); grup lain tanpa entri: none.
	FieldAccess map[string]map[string]authz.Access
	// Assignments: dimensi ke nilai penugasan; dimensi tanpa nilai: scope assigned bernilai FALSE.
	Assignments map[string][]string
	// Administrator: izin penuh atas semua resource (seperti role sistem administrator).
	Administrator bool
}

// Engine adalah mesin otorisasi uji; memenuhi authz.Engine.
type Engine struct {
	resources map[string]Resource
	providers map[string]authz.ScopeProvider
	users     map[string]User
}

var _ authz.Engine = (*Engine)(nil)

// NewEngine membuat mesin uji untuk resource dan scope kustom yang diberikan.
func NewEngine(resources []Resource, providers ...authz.ScopeProvider) *Engine {
	e := &Engine{
		resources: make(map[string]Resource, len(resources)),
		providers: make(map[string]authz.ScopeProvider, len(providers)),
		users:     map[string]User{},
	}
	for _, r := range resources {
		e.resources[r.Key] = r
	}
	for _, p := range providers {
		e.providers[p.Key] = p
	}
	return e
}

// AddUser mendaftarkan hak pengguna; mengembalikan e agar dapat dirantai.
func (e *Engine) AddUser(u User) *Engine {
	e.users[u.ID] = u
	return e
}

// Context mengembalikan salinan ctx yang membawa Principal userID dan mesin ini (seperti yang
// dilakukan platform di route dan kontrak). userID yang belum didaftarkan: ditolak semua.
func (e *Engine) Context(ctx context.Context, userID string) context.Context {
	ctx = identity.WithPrincipal(ctx, identity.Principal{UserID: userID, Kind: identity.KindHuman})
	return authz.WithEngine(ctx, e)
}

// AsAdministrator mengembalikan ctx dengan pengguna administrator (izin penuh) dan mesin uji tanpa
// resource; untuk test yang tidak peduli otorisasi.
func AsAdministrator(ctx context.Context, userID string) context.Context {
	return NewEngine(nil).AddUser(User{ID: userID, Administrator: true}).Context(ctx, userID)
}

var errNoUser = errors.New("authztest: pengguna tidak terdaftar")

// actor menentukan pelaku: hak sistem (izin penuh), pengguna terdaftar, atau tidak ada.
func (e *Engine) actor(ctx context.Context) (u User, system bool, err error) {
	if _, ok := authz.SystemReason(ctx); ok {
		return User{}, true, nil
	}
	p, ok := identity.FromContext(ctx)
	if !ok || p.UserID == "" {
		return User{}, false, authz.ErrUnauthenticated
	}
	u, ok = e.users[p.UserID]
	if !ok {
		return User{}, false, errNoUser
	}
	return u, false, nil
}

func splitPermission(permission string) (resource, action string, ok bool) {
	i := strings.LastIndex(permission, ".")
	if i <= 0 || i == len(permission)-1 {
		return "", "", false
	}
	return permission[:i], permission[i+1:], true
}

// rows mengembalikan baris matriks pengguna yang memuat aksi pada resource.
func (u User) rows(resource, action string) []Grant {
	var out []Grant
	for _, g := range u.Grants {
		if g.Resource != resource {
			continue
		}
		for _, a := range g.Actions {
			if a == action {
				out = append(out, g)
				break
			}
		}
	}
	return out
}

// Require mengimplementasi authz.Engine.
func (e *Engine) Require(ctx context.Context, permission string) error {
	u, system, err := e.actor(ctx)
	if err != nil {
		return denyErr(err)
	}
	if system || u.Administrator {
		return nil
	}
	resource, action, ok := splitPermission(permission)
	if !ok || len(u.rows(resource, action)) == 0 {
		return authz.ErrForbidden
	}
	return nil
}

func denyErr(err error) error {
	if errors.Is(err, authz.ErrUnauthenticated) {
		return err
	}
	return authz.ErrForbidden
}

// Filter mengimplementasi authz.Engine: satu cabang per baris yang memuat aksi; baris dengan scope tak
// sah dilepas (tidak pernah dianggap "semua").
func (e *Engine) Filter(ctx context.Context, resource, action string) (authz.RowFilter, error) {
	u, system, err := e.actor(ctx)
	if err != nil {
		return authz.RowFilter{}, denyErr(err)
	}
	if system || u.Administrator {
		return authz.RowFilter{Branches: []authz.RowBranch{{Conditions: []authz.RowCondition{authz.AllRows()}}}}, nil
	}
	var rf authz.RowFilter
	for _, g := range u.rows(resource, action) {
		if b, ok := e.branch(resource, u, g.Scope); ok {
			rf.Branches = append(rf.Branches, b)
		}
	}
	return rf, nil
}

func (e *Engine) ownerField(resource string) string {
	if f := e.resources[resource].OwnerField; f != "" {
		return f
	}
	return "created_by"
}

func (e *Engine) branch(resource string, u User, scope []string) (authz.RowBranch, bool) {
	if len(scope) == 0 {
		return authz.RowBranch{}, false
	}
	var b authz.RowBranch
	for _, tok := range scope {
		switch {
		case tok == "all":
			b.Conditions = append(b.Conditions, authz.AllRows())
		case tok == "owner":
			b.Conditions = append(b.Conditions, authz.OwnerIs(e.ownerField(resource), u.ID))
		case strings.HasPrefix(tok, "assigned:"):
			dim := strings.TrimPrefix(tok, "assigned:")
			col, ok := e.resources[resource].ScopeFields[dim]
			if !ok {
				return authz.RowBranch{}, false
			}
			b.Conditions = append(b.Conditions, authz.AssignedIn(col, u.Assignments[dim]))
		case strings.HasPrefix(tok, "custom:"):
			p, ok := e.providers[strings.TrimPrefix(tok, "custom:")]
			if !ok {
				return authz.RowBranch{}, false
			}
			b.Conditions = append(b.Conditions, authz.CustomScope(p))
		default:
			return authz.RowBranch{}, false
		}
	}
	return b, true
}

// Check mengimplementasi authz.Engine: tanpa baris yang memuat aksi: ErrForbidden; ada tetapi tak satu
// pun scope-nya cocok: ErrNotFound.
func (e *Engine) Check(ctx context.Context, permission string, rec authz.Record) error {
	u, system, err := e.actor(ctx)
	if err != nil {
		return authz.ErrNotFound
	}
	if system || u.Administrator {
		return nil
	}
	resource, action, ok := splitPermission(permission)
	if !ok {
		return authz.ErrForbidden
	}
	rows := u.rows(resource, action)
	if len(rows) == 0 {
		return authz.ErrForbidden
	}
	for _, g := range rows {
		if e.rowMatches(ctx, resource, u, g.Scope, rec) {
			return nil
		}
	}
	return authz.ErrNotFound
}

func (e *Engine) rowMatches(ctx context.Context, resource string, u User, scope []string, rec authz.Record) bool {
	if len(scope) == 0 || rec == nil {
		return false
	}
	for _, tok := range scope {
		if !e.tokenMatches(ctx, resource, u, tok, rec) {
			return false
		}
	}
	return true
}

func (e *Engine) tokenMatches(ctx context.Context, resource string, u User, tok string, rec authz.Record) (match bool) {
	switch {
	case tok == "all":
		return true
	case tok == "owner":
		v, ok := rec.AuthzValue(e.ownerField(resource))
		return ok && v != nil && fmt.Sprint(v) == u.ID
	case strings.HasPrefix(tok, "assigned:"):
		dim := strings.TrimPrefix(tok, "assigned:")
		col, ok := e.resources[resource].ScopeFields[dim]
		if !ok {
			return false
		}
		v, ok := rec.AuthzValue(col)
		if !ok || v == nil {
			return false
		}
		for _, a := range u.Assignments[dim] {
			if a == fmt.Sprint(v) {
				return true
			}
		}
		return false
	case strings.HasPrefix(tok, "custom:"):
		p, ok := e.providers[strings.TrimPrefix(tok, "custom:")]
		if !ok || p.Match == nil {
			return false
		}
		defer func() {
			if recover() != nil {
				match = false
			}
		}()
		m, err := p.Match(ctx, rec)
		return err == nil && m
	}
	return false
}

// Fields mengimplementasi authz.Engine.
func (e *Engine) Fields(ctx context.Context, resource string) authz.FieldAccess {
	u, system, err := e.actor(ctx)
	if err != nil {
		return authz.NewFieldAccess(authz.FieldAccessSpec{Resource: resource})
	}
	r, known := e.resources[resource]
	spec := authz.FieldAccessSpec{Resource: resource, Groups: map[string]authz.Access{}, FieldGroups: map[string]string{}}
	for _, g := range r.Groups {
		if g.Default {
			spec.DefaultGroup = g.Key
		}
		for _, f := range g.Fields {
			spec.FieldGroups[f] = g.Key
		}
		spec.Groups[g.Key] = e.groupAccess(u, system, resource, g)
	}
	if !known && (system || u.Administrator) {
		return allWrite{}
	}
	return authz.NewFieldAccess(spec)
}

func (e *Engine) groupAccess(u User, system bool, resource string, g Group) authz.Access {
	if system || u.Administrator {
		return authz.AccessWrite
	}
	acc := u.FieldAccess[resource][g.Key]
	if g.Default {
		derived := authz.AccessNone
		if len(u.rows(resource, "update")) > 0 {
			derived = authz.AccessWrite
		} else if len(u.rows(resource, "read")) > 0 {
			derived = authz.AccessRead
		}
		if derived > acc {
			acc = derived
		}
	}
	return acc
}

// MaskFields mengimplementasi authz.Engine.
func (e *Engine) MaskFields(ctx context.Context, resource string, fields map[string]any) map[string]any {
	return authz.ApplyMask(e.Fields(ctx, resource), fields)
}

// CheckWrite mengimplementasi authz.Engine.
func (e *Engine) CheckWrite(ctx context.Context, resource string, changedFields []string) error {
	return authz.CheckWriteAccess(e.Fields(ctx, resource), resource, changedFields)
}

type allWrite struct{}

func (allWrite) Group(string) authz.Access { return authz.AccessWrite }
func (allWrite) Field(string) authz.Access { return authz.AccessWrite }
func (allWrite) Readable(...string) error  { return nil }
