package manifest

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"erpora/sdk/module"
)

// Kode problem validasi deklarasi otorisasi manifest (PRD-001 FR-110). Bentuk dokumen (tipe, pola, properti
// wajib) dilaporkan sebagai CodeSchemaInvalid oleh Validate; kode di bawah untuk aturan semantik.
const (
	CodeInvalidResource       = "invalid_resource"
	CodeInvalidScopeDimension = "invalid_scope_dimension"
	CodeInvalidDefaultRole    = "invalid_default_role"
	CodeCapabilityNotDerived  = "capability_not_derived"
)

// OrgUnitDimension adalah dimensi scope bawaan platform (PRD-001 FR-118) yang boleh dirujuk modul mana pun.
const OrgUnitDimension = "core.org_unit"

// ValidateAuthorization memeriksa bagian otorisasi manifest (resources, scope_dimensions, default_roles, dan
// capabilities) terhadap aturan semantik FR-110 yang tidak dapat dinyatakan JSON Schema: aksi standar, aksi
// kustom tidak bentrok, tepat satu grup field default, satu field paling banyak di satu grup, scope_fields hanya
// merujuk dimensi terdaftar (milik modul sendiri, core.org_unit, atau modul pada dependencies), baris default_roles
// merujuk resource dan aksi modul sendiri, dan capabilities hanya berisi permission turunan resource x aksi.
//
// peers adalah manifest modul lain yang dikenal (boleh memuat m sendiri, dilewati menurut Key): dipakai untuk
// keunikan kunci resource/dimensi lintas modul dan dimensi milik dependensi. Satu fungsi ini dipakai registry dan
// `module:check` sehingga keduanya memberi temuan yang sama. Manifest tanpa deklarasi otorisasi lolos (aditif).
// Hasil mengikuti urutan deklarasi; Field berupa JSON pointer ke module.json.
func ValidateAuthorization(m module.Manifest, peers []module.Manifest) []Problem {
	v := &authzValidator{m: m}
	for _, p := range peers {
		if p.Key != m.Key {
			v.peers = append(v.peers, p)
		}
	}
	v.checkCapabilities()
	v.indexDimensions()
	for i, r := range m.Resources {
		v.checkResource(i, r)
	}
	v.checkDimensions()
	for i, r := range m.DefaultRoles {
		v.checkRole(i, r)
	}
	return v.out
}

type authzValidator struct {
	m     module.Manifest
	peers []module.Manifest
	out   []Problem

	ownDims  map[string]bool
	knownDim map[string]bool // milik sendiri, core.org_unit, atau dimensi modul pada dependencies
	byRes    map[string]module.Resource
}

func (v *authzValidator) add(code, field, format string, a ...any) {
	v.out = append(v.out, Problem{Module: v.m.Key, Field: field, Code: code, Message: fmt.Sprintf(format, a...)})
}

func (v *authzValidator) checkCapabilities() {
	derived := v.m.Permissions()
	for i, c := range v.m.Capabilities {
		if !slices.Contains(derived, c) {
			v.add(CodeCapabilityNotDerived, "/capabilities/"+strconv.Itoa(i),
				"capability %q bukan permission turunan resource x aksi; deklarasikan resource dan aksinya di \"resources\" (PRD-001 FR-110: capabilities diturunkan otomatis)", c)
		}
	}
}

func (v *authzValidator) indexDimensions() {
	v.ownDims = map[string]bool{}
	v.knownDim = map[string]bool{OrgUnitDimension: true}
	for _, d := range v.m.ScopeDimensions {
		v.ownDims[d.Key] = true
		v.knownDim[d.Key] = true
	}
	deps := map[string]bool{}
	for _, d := range v.m.Dependencies {
		deps[d.Module] = true
	}
	for _, p := range v.peers {
		if !deps[p.Key] {
			continue
		}
		for _, d := range p.ScopeDimensions {
			v.knownDim[d.Key] = true
		}
	}
	v.byRes = map[string]module.Resource{}
	for _, r := range v.m.Resources {
		if _, dup := v.byRes[r.Key]; !dup {
			v.byRes[r.Key] = r
		}
	}
}

func (v *authzValidator) checkResource(i int, r module.Resource) {
	base := "/resources/" + strconv.Itoa(i)
	prefix := v.m.Slug + "."
	if !strings.HasPrefix(r.Key, prefix) {
		v.add(CodeInvalidResource, base+"/key", "kunci resource %q harus berawalan %q (slug modul)", r.Key, prefix)
	}
	for j := 0; j < i; j++ {
		if v.m.Resources[j].Key == r.Key {
			v.add(CodeInvalidResource, base+"/key", "kunci resource %q sudah dideklarasikan pada /resources/%d", r.Key, j)
			break
		}
	}
	for _, p := range v.peers {
		if slices.ContainsFunc(p.Resources, func(o module.Resource) bool { return o.Key == r.Key }) {
			v.add(CodeInvalidResource, base+"/key", "kunci resource %q sudah dideklarasikan modul %s; kunci resource unik lintas modul", r.Key, p.Key)
			break
		}
	}

	seenAction := map[string]bool{}
	for j, a := range r.Actions {
		if !module.IsStandardAction(a) {
			v.add(CodeInvalidResource, base+"/actions/"+strconv.Itoa(j),
				"aksi %q bukan aksi standar (%s); aksi di luar standar dideklarasikan lewat custom_actions", a, strings.Join(module.StandardActions, ", "))
		}
		seenAction[a] = true
	}
	for j, c := range r.CustomActions {
		field := base + "/custom_actions/" + strconv.Itoa(j) + "/key"
		switch {
		case module.IsStandardAction(c.Key):
			v.add(CodeInvalidResource, field, "aksi kustom %q bentrok dengan aksi standar; gunakan nama lain atau pindahkan ke actions", c.Key)
		case seenAction[c.Key]:
			v.add(CodeInvalidResource, field, "aksi kustom %q sudah dideklarasikan pada resource %q", c.Key, r.Key)
		}
		seenAction[c.Key] = true
	}
	if len(r.Actions)+len(r.CustomActions) == 0 {
		v.add(CodeInvalidResource, base+"/actions", "resource %q wajib punya minimal satu aksi (actions atau custom_actions)", r.Key)
	}

	v.checkFieldGroups(base, r)

	dims := make([]string, 0, len(r.ScopeFields))
	for d := range r.ScopeFields {
		dims = append(dims, d)
	}
	sort.Strings(dims)
	for _, d := range dims {
		if !v.knownDim[d] {
			v.add(CodeInvalidResource, base+"/scope_fields/"+escape(d),
				"dimensi scope %q tidak terdaftar: deklarasikan di scope_dimensions modul ini, atau pakai dimensi milik modul pada dependencies atau %s", d, OrgUnitDimension)
		}
	}
}

func (v *authzValidator) checkFieldGroups(base string, r module.Resource) {
	var defaultKey string
	defaults := 0
	groupKeys := map[string]bool{}
	fieldOwner := map[string]string{}
	for j, g := range r.FieldGroups {
		gb := base + "/field_groups/" + strconv.Itoa(j)
		if groupKeys[g.Key] {
			v.add(CodeInvalidResource, gb+"/key", "kunci grup field %q sudah dipakai pada resource %q", g.Key, r.Key)
		}
		groupKeys[g.Key] = true
		if g.Default {
			defaults++
			if defaults == 1 {
				defaultKey = g.Key
			} else {
				v.add(CodeInvalidResource, gb+"/default",
					"resource %q sudah punya grup default %q; hanya satu grup yang boleh bertanda default", r.Key, defaultKey)
			}
		}
		for k, f := range g.Fields {
			if owner, dup := fieldOwner[f]; dup {
				v.add(CodeInvalidResource, gb+"/fields/"+strconv.Itoa(k),
					"field %q sudah ada di grup %q pada resource %q; satu field hanya boleh berada di satu grup", f, owner, r.Key)
				continue
			}
			fieldOwner[f] = g.Key
		}
	}
	if defaults == 0 {
		v.add(CodeInvalidResource, base+"/field_groups",
			"resource %q tidak punya grup default; tandai tepat satu grup field dengan \"default\": true (field yang tidak tercantum di grup mana pun masuk grup itu)", r.Key)
	}
}

func (v *authzValidator) checkDimensions() {
	prefix := v.m.Slug + "."
	for i, d := range v.m.ScopeDimensions {
		field := "/scope_dimensions/" + strconv.Itoa(i) + "/key"
		if !strings.HasPrefix(d.Key, prefix) {
			v.add(CodeInvalidScopeDimension, field, "kunci dimensi %q harus berawalan %q (slug modul)", d.Key, prefix)
		}
		for j := 0; j < i; j++ {
			if v.m.ScopeDimensions[j].Key == d.Key {
				v.add(CodeInvalidScopeDimension, field, "kunci dimensi %q sudah dideklarasikan pada /scope_dimensions/%d", d.Key, j)
				break
			}
		}
		for _, p := range v.peers {
			if slices.ContainsFunc(p.ScopeDimensions, func(o module.ScopeDimension) bool { return o.Key == d.Key }) {
				v.add(CodeInvalidScopeDimension, field, "kunci dimensi %q sudah dideklarasikan modul %s; kunci dimensi unik lintas modul", d.Key, p.Key)
				break
			}
		}
	}
}

func (v *authzValidator) checkRole(i int, role module.DefaultRole) {
	base := "/default_roles/" + strconv.Itoa(i)
	prefix := v.m.Slug + "."
	if !strings.HasPrefix(role.Key, prefix) {
		v.add(CodeInvalidDefaultRole, base+"/key", "kunci role bawaan %q harus berawalan %q (slug modul)", role.Key, prefix)
	}
	for j := 0; j < i; j++ {
		if v.m.DefaultRoles[j].Key == role.Key {
			v.add(CodeInvalidDefaultRole, base+"/key", "kunci role bawaan %q sudah dideklarasikan pada /default_roles/%d", role.Key, j)
			break
		}
	}
	for j, row := range role.Permissions {
		rb := base + "/permissions/" + strconv.Itoa(j)
		res, ok := v.byRes[row.Resource]
		if !ok {
			v.add(CodeInvalidDefaultRole, rb+"/resource", "resource %q tidak dideklarasikan di modul ini (role bawaan hanya boleh merujuk resource modulnya sendiri)", row.Resource)
			continue
		}
		keys := res.ActionKeys()
		for k, a := range row.Actions {
			if !slices.Contains(keys, a) {
				v.add(CodeInvalidDefaultRole, rb+"/actions/"+strconv.Itoa(k), "aksi %q tidak dideklarasikan pada resource %q (tersedia: %s)", a, res.Key, strings.Join(keys, ", "))
			}
		}
		for k, tok := range row.Scope {
			dim, found := strings.CutPrefix(tok, "assigned:")
			if !found {
				continue
			}
			if _, mapped := res.ScopeFields[dim]; !mapped {
				v.add(CodeInvalidDefaultRole, rb+"/scope/"+strconv.Itoa(k), "dimensi %q tidak dipetakan di scope_fields resource %q sehingga scope %q tidak dapat dievaluasi", dim, res.Key, tok)
			}
		}
	}
	for j, fa := range role.FieldAccess {
		fb := base + "/field_access/" + strconv.Itoa(j)
		res, ok := v.byRes[fa.Resource]
		if !ok {
			v.add(CodeInvalidDefaultRole, fb+"/resource", "resource %q tidak dideklarasikan di modul ini (role bawaan hanya boleh merujuk resource modulnya sendiri)", fa.Resource)
			continue
		}
		if !slices.ContainsFunc(res.FieldGroups, func(g module.FieldGroup) bool { return g.Key == fa.FieldGroup }) {
			v.add(CodeInvalidDefaultRole, fb+"/field_group", "grup field %q tidak ada pada resource %q", fa.FieldGroup, res.Key)
		}
	}
}
