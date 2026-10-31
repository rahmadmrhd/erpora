// Package desks menyusun daftar Desk per pengguna (revisi M3a, design.md Komponen 6, Q40 butir 10-13): Desk
// modul dari manifest (properti "desk") ditambah Desk platform, disaring status modul tenant, permission pengguna
// (runtime.Authorizer), dan timpaan situs (tabel desk_overrides, hanya dibaca). Backend adalah otoritas: menu yang
// tidak diizinkan tidak pernah dikirim ke klien.
package desks

// Jenis menu dan sumber Desk.
const (
	TypeItem  = "item"
	TypeGroup = "group"

	SourceModule   = "module"
	SourcePlatform = "platform"
	SourceCustom   = "custom" // Desk kustom (M3b); belum dihasilkan revisi M3a
)

// Item Dashboard: dibangkitkan platform sebagai menu pertama setiap Desk (D-8), terkunci.
const (
	DashboardID          = "dashboard"
	DashboardLabelKey    = "desk.dashboard" // namespace platform
	DashboardDefaultIcon = "layout-dashboard"
)

// MessageRef menunjuk pesan i18n: namespace katalog (slug modul atau "platform") dan kunci; diterjemahkan klien.
type MessageRef struct {
	NS  string `json:"ns"`
	Key string `json:"key"`
}

// Menu adalah satu item menu (To dan OwnerDesk terisi) atau satu grup satu tingkat (Children terisi).
// To berupa path absolut aplikasi ("/<slug>/<halaman>"); OwnerDesk adalah id Desk pemilik item.
type Menu struct {
	Type      string     `json:"type"`
	ID        string     `json:"id"`
	Label     MessageRef `json:"label"`
	To        string     `json:"to,omitempty"`
	Icon      string     `json:"icon,omitempty"`
	Locked    bool       `json:"locked,omitempty"`
	OwnerDesk string     `json:"ownerDesk,omitempty"`
	Children  []Menu     `json:"children,omitempty"`
}

// Desk adalah satu Desk yang boleh dilihat pengguna. ID = slug modul (D-3) atau "platform".
type Desk struct {
	ID        string     `json:"id"`
	Source    string     `json:"source"`
	ModuleKey string     `json:"moduleKey,omitempty"`
	Label     MessageRef `json:"label"`
	Icon      string     `json:"icon,omitempty"`
	Menus     []Menu     `json:"menus"`
}

// View adalah hasil List. Revision adalah hash isi Desks (SHA-256, 16 byte heksadesimal) dan ETag adalah
// Revision yang dikutip; berbasis isi respons akhir, bukan revisi modul, sehingga berubah bila izin berubah (D-6).
type View struct {
	Desks    []Desk
	Revision string
	ETag     string
}
