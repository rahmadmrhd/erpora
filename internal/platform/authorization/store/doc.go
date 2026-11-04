// Package store adalah persistensi otorisasi PRD-001 C2 (FR-110 sampai FR-113, FR-118; ADR-0013 §10): matriks
// permission per role (role_permissions), akses grup field (role_field_access), penugasan scope per pengguna
// (user_scope_assignments), dan revisi otorisasi (authz_state) di tenant DB (migrasi tenant 0005_authz; tabel roles
// dan user_roles dari 0003_roles_min). Satu-satunya pemakai GORM untuk tabel itu (D3b, K-C1.7). Tenant selalu
// diambil dari context (tenancy.FromContext) dan database dipilih lewat TenantConnector (isolasi per database,
// ADR-0004; Q18): tabel tanpa tenant_id. Skema hanya lewat migrasi goose (tanpa AutoMigrate); semua nilai
// parameter terikat, tidak ada identifier dinamis.
//
// # Aturan yang berlaku untuk semua penulisan
//
//   - Revisi naik dalam transaksi yang sama. Setiap metode tulis di paket ini menjalankan perubahannya dan
//     BumpRevision dalam SATU transaksi tenant: keduanya tercatat atau keduanya batal. Pemanggil menerima revisi
//     baru (dasar pemberitahuan antarinstance oleh mesin, C2.4, setelah commit).
//   - user_roles milik identity/store (GrantRole, RevokeRole) juga menaikkan revisi lewat BumpRevision(tx) pada
//     transaksi mereka sendiri; paket identity/store mengimpor paket ini (satu arah: paket ini tidak mengimpor
//     identity). Dengan begitu pemberian role administrator oleh core:create-admin tidak dapat melewati revisi.
//   - Gagal tertutup. Scope kosong atau berisi token tak sah tidak dapat disimpan (validasi di Go dan CHECK di
//     database). Pembaca yang menemukan nilai tersimpan tak terurai menandai PermissionRow.ScopeInvalid; mesin wajib
//     menolak baris itu.
//   - Union per baris dan "tanpa deny" adalah urusan mesin (C2.4); paket ini hanya menyimpan dan memuat baris.
//   - Aturan service tidak ada di sini: administrator aktif terakhir (butuh status pengguna di control DB, dan
//     serialisasi lewat kunci advisory transaksi), pencegahan eskalasi hak (K-C2.6), dan permission pemanggil adalah
//     tanggung jawab authz.Admin (C2.10). Pengecualian yang melindungi invarian data: matriks role administrator
//     tidak dapat diganti (ErrImmutable) dan hanya role custom yang dapat dihapus.
//
// # API
//
// Pembacaan:
//
//	Revision(ctx) (int64, error)
//	LoadUserGrants(ctx, userID) (Grants, error)   // snapshot REPEATABLE READ: revisi + role + matriks + field access + penugasan
//	ListRoles(ctx) ([]Role, error)
//	GetRole(ctx, id) (Role, error)
//	RoleGrantsOf(ctx, roleID) (RoleGrants, error)
//	UserRoleIDs(ctx, userID) ([]string, error)
//	UserScopeAssignments(ctx, userID) ([]ScopeAssignment, error)
//
// Penulisan (semua menaikkan revisi pada transaksi yang sama; hasil int64 adalah revisi baru):
//
//	CreateRole(ctx, NewRole) (Role, int64, error)
//	UpdateRole(ctx, id, RoleUpdate) (Role, int64, error)              // label/deskripsi; role bawaan menjadi customized
//	DeleteRole(ctx, id) (int64, error)                                // hanya custom tanpa pemegang
//	ReplaceRoleGrants(ctx, roleID, RoleGrants, ifUpdatedAt) (int64, error) // penggantian utuh matriks + field access
//	DuplicateRole(ctx, sourceID, NewRole) (Role, int64, error)
//	SetUserRoles(ctx, userID, roleIDs) (int64, error)                 // penggantian himpunan role eksplisit
//	SetScopeAssignments(ctx, userID, []ScopeAssignment) (int64, error) // penggantian utuh; nilai kosong = hapus
//	SyncModuleDefaultRoles(ctx, module, []DefaultRoleDef) (SyncResult, error) // install/upgrade modul
//
// Fungsi paket (dipanggil dengan transaksi pemanggil):
//
//	BumpRevision(tx *gorm.DB) (int64, error)
//
// Org unit (tabel org_units, migrasi 0006) ditambahkan C2.10 secara aditif di paket ini.
package store
