package module

import "slices"

// Resource adalah satu resource permission modul (PRD-001 FR-110): kumpulan aksi, field group, dan pemetaan
// scope. Permission turunannya berbentuk "<Key>.<aksi>". Key berbentuk "<slug>.<nama_snake>" dan berawalan slug
// modul pemiliknya. Label dan label turunan lain berupa kunci i18n namespace modul (PRD-001 Q40), bukan teks.
type Resource struct {
	Key           string         `json:"key"`
	Label         string         `json:"label"`
	Actions       []string       `json:"actions"`
	CustomActions []CustomAction `json:"custom_actions,omitempty"`
	// OwnerField dipakai scope "owner"; kosong = "created_by".
	OwnerField string `json:"owner_field,omitempty"`
	// ScopeFields memetakan dimensi scope (kunci dimensi) ke nama field resource yang dibandingkan.
	ScopeFields map[string]string `json:"scope_fields,omitempty"`
	FieldGroups []FieldGroup      `json:"field_groups"`
	// Audit hanya disimpan di manifest; penerapannya milik PRD-001 FR-131.
	Audit *ResourceAudit `json:"audit,omitempty"`
}

// CustomAction adalah aksi kustom resource; Key berpola ^[a-z_]+$ dan tidak boleh sama dengan aksi standar.
type CustomAction struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// FieldGroup adalah kelompok field bernama dengan akses none/read/write per role (FR-112). Tepat satu grup per
// resource bertanda Default; field yang tidak tercantum di grup mana pun masuk grup itu. Fields opsional bila
// resource punya metadata (PRD-000 Q41).
type FieldGroup struct {
	Key            string   `json:"key"`
	Label          string   `json:"label"`
	Classification string   `json:"classification,omitempty"`
	Fields         []string `json:"fields,omitempty"`
	Default        bool     `json:"default,omitempty"`
	// Sensitive hanya disimpan di manifest (FR-131).
	Sensitive bool `json:"sensitive,omitempty"`
}

// ResourceAudit adalah pengaturan audit resource (FR-131), hanya disimpan di manifest.
type ResourceAudit struct {
	DataChanges bool `json:"data_changes,omitempty"`
}

// ScopeDimension adalah dimensi scope data milik modul (FR-111), mis. gudang. Source menamai sumber nilai yang
// dipilih admin saat menugaskan dimensi ke pengguna (K-C2.3); kosong = Key.
type ScopeDimension struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Source string `json:"source,omitempty"`
}

// DefaultRole adalah role bawaan modul (FR-113) beserta matriks awalnya.
type DefaultRole struct {
	Key         string            `json:"key"`
	Label       string            `json:"label"`
	Permissions []RoleRow         `json:"permissions"`
	FieldAccess []RoleFieldAccess `json:"field_access,omitempty"`
}

// RoleRow adalah satu baris matriks role: aksi pada satu resource dengan scope berupa daftar kondisi yang digabung
// AND ("all", "owner", "assigned:<dimensi>", "custom:<key>"); daftar kosong tidak sah.
type RoleRow struct {
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
	Scope    []string `json:"scope"`
}

// RoleFieldAccess menetapkan akses ("none", "read", "write") sebuah field group resource untuk role bawaan.
type RoleFieldAccess struct {
	Resource   string `json:"resource"`
	FieldGroup string `json:"field_group"`
	Access     string `json:"access"`
}

// Nilai akses field group (FR-112).
const (
	FieldAccessNone  = "none"
	FieldAccessRead  = "read"
	FieldAccessWrite = "write"
)

// StandardActions adalah aksi standar resource (PRD-001 FR-110). Aksi kustom tidak boleh bentrok dengan daftar ini.
var StandardActions = []string{
	"read", "create", "update", "delete", "print", "export", "report",
	"submit", "cancel", "amend", "approve", "import", "share", "email",
}

// IsStandardAction melaporkan apakah a termasuk StandardActions.
func IsStandardAction(a string) bool { return slices.Contains(StandardActions, a) }

// ActionKeys mengembalikan seluruh kunci aksi resource: Actions lalu CustomActions, urutan deklarasi.
func (r Resource) ActionKeys() []string {
	out := make([]string, 0, len(r.Actions)+len(r.CustomActions))
	out = append(out, r.Actions...)
	for _, c := range r.CustomActions {
		out = append(out, c.Key)
	}
	return out
}

// PermissionsOf menurunkan permission "<resource>.<aksi>" dari resource (aksi lalu aksi kustom, urutan
// deklarasi). Inilah satu-satunya sumber himpunan permission modul (FR-110: capabilities diturunkan otomatis).
func PermissionsOf(resources []Resource) []string {
	var out []string
	for _, r := range resources {
		for _, a := range r.ActionKeys() {
			out = append(out, r.Key+"."+a)
		}
	}
	return out
}

// Permissions adalah permission turunan resource manifest (lihat PermissionsOf), terurut deklarasi.
func (m Manifest) Permissions() []string { return PermissionsOf(m.Resources) }
