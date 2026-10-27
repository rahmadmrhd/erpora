# Siklus hidup modul

Panduan untuk penulis modul: apa yang dipanggil platform, kapan, dan apa yang wajib dipenuhi hook Anda. Semantik lengkap dan alasan keputusannya ada di [ADR-007](../adr/0007-semantik-install-enable-disable-uninstall.md); contoh di sini memakai `modules/example` (`acme.example`) dan `modules/exampleaddon` (`acme.example-addon`).

## 1. Status dan operasi

Status tersimpan per tenant: `installed`, `enabled`, `disabled`, `upgrade_pending`, `failed`, `uninstalled`. Modul yang belum pernah dipasang adalah `available`; `missing` dan `incompatible` adalah status turunan. Route modul hanya melayani request saat `enabled` (`installed`/`disabled` → `403 module_disabled`, `failed`/`incompatible`/`upgrade_pending` → `503 module_unavailable`, selain itu `404 module_not_found`).

Operasi dijalankan lewat CLI `erpora modules …` atau API `/api/platform/modules`. Operasi yang status targetnya sudah tercapai adalah no-op sukses (tanpa event, tanpa kenaikan revision), jadi aman diulang dari skrip.

Setiap commit operasi menaikkan `revision` (ETag API), termasuk operasi gagal yang tidak mengubah status. Respons API mutasi memuat semua ID operasi rangkaian di `operationIds` (aditif; `operationId` dan `Location` menunjuk operasi pertama) dan mensyaratkan `Content-Type: application/json` (body kosong atau `{}` boleh; selain itu `415`). `If-Match` mengikuti RFC 9110 (`*`, daftar entity-tag, perbandingan kuat) dan dievaluasi di dalam lock.

## 2. Urutan hook per operasi

| Operasi | Langkah (dicatat di `module_operations.steps`) | Hook yang dipanggil |
| --- | --- | --- |
| `install` | `resolve` → `compatibility` → `migrate` → `hook:install` | `Installer.Install` |
| `enable` | `dependencies` → `hook:enable` → `health` | `EnableHook.OnEnable`, `HealthChecker.Health`; bila `Health` gagal: `DisableHook.OnDisable` dan status tetap status asal |
| `disable` | `dependents` → `hook:disable` | `DisableHook.OnDisable` |
| `upgrade` | `resolve` → `compatibility` → `migrate` → `hook:upgrade` | `Upgrader.Upgrade(from, to)` |
| `uninstall` | `dependents` | tidak ada (data tidak dihapus) |

Hook yang tidak diimplementasikan modul dicatat `skipped`. `DevSeeder.SeedDev` bukan bagian lifecycle produksi (dipanggil developer tools, M2d).

## 3. Idempotensi (wajib)

Platform dapat memanggil hook ulang: `retry` mengulang langkah yang gagal, dan operasi yang terputus dapat dilanjutkan. Karena itu setiap hook harus aman dijalankan lebih dari sekali. Contoh dari `example`:

```go
// Install menyemai 3 notes bila tabel kosong (idempoten).
func (m *Module) Install(ctx context.Context, _ module.Env) error { return m.svc.SeedInitial(ctx) }

// SeedInitial menambah 3 notes awal hanya bila tabel kosong.
func (s *Service) SeedInitial(ctx context.Context) error {
	n, err := s.repo.Count(ctx)
	if err != nil || n > 0 {
		return err
	}
	return s.add(ctx, "Catatan contoh", 3)
}
```

`OnEnable`/`OnDisable` pada `exampleaddon` hanya mencatat log, sehingga idempoten secara alami. Hook yang panic diubah menjadi kegagalan langkah (`hook_panic`) tanpa menjatuhkan proses; stack hanya masuk log.

## 4. Konteks hook

Semua hook menerima `ctx` dan `module.Env` yang disiapkan `runtime.ModuleContext`:

- **Tenant**: `tenancy` di context; `Env.TenantID` berisi tenant yang sama. Parameter kontrak tidak boleh membawa `tenantId`.
- **Database**: `data.DB(ctx)` mengembalikan DB tenant aktif. Modul tidak pernah memegang pool (aturan D3b: hanya dari `internal/infrastructure`).
- **Hak sistem**: `authz.SystemReason(ctx)` bernilai `module.lifecycle:<operasi>` (mis. `module.lifecycle:install`). Command CLI bernilai `cli:<slug>:<name>`. Setiap pemakaian dicatat di log dengan `system_reason`.
- **Logger**: `Env.Logger` sudah ter-scope `erp.module.key`; jangan menambahkan atribut itu lagi.

## 5. Aturan migrasi

Migrasi modul (`migrations/*.sql`, goose, diekspos lewat `module.MigrationSource`) dijalankan oleh platform pada `install` dan `upgrade`, dengan tabel versi `goose_module_<slug_snake>` (slug `example-addon` → `goose_module_example_addon`).

- **Satu file = satu transaksi.** File yang gagal di-rollback penuh (tidak ada tabel/kolom parsial); file sebelumnya tetap diterapkan, dan `retry` hanya menerapkan sisanya. Jejak per file ada di `module_migrations` (tenant DB) dengan checksum.
- **Jangan menulis penanda anotasi goose di komentar** (tanda plus diikuti kata goose di luar `Up`/`Down` yang sebenarnya): goose mem-parse-nya.
- **Backfill besar dilakukan bertahap.** Satu `UPDATE` seperti `migrations/0002_add_example_notes_version.sql` hanya pantas untuk tabel kecil, karena mengunci dan menulis ulang semua baris dalam satu transaksi. Untuk tabel besar gunakan migrasi tanpa transaksi dengan batch.
- Migrasi tidak pernah berjalan saat `serve` atau pada request HTTP; hanya lewat `migrate up` (modul `required`) dan operasi lifecycle.
- `down` hanya untuk pengembangan; produksi memulihkan dari backup (deployment.md §7.1).

## 6. Upgrade dan downgrade

Saat binary membawa versi lebih baru dari yang ter-install, `serve` (reconcile) menandai modul `upgrade_pending` dan routenya `503` (`Retry-After: 30`) sampai `erpora modules upgrade <key>` dijalankan. Upgrade memanggil `Upgrade(ctx, env, from, to)` setelah migrasi baru diterapkan; status sebelumnya (mis. `enabled`) dipulihkan bila sukses. Upgrade yang akan mengembalikan modul ke `enabled` mensyaratkan semua dependensi wajib `enabled` (ditolak sebelum dimulai bila tidak). Bila binary kembali ke versi ter-install saat status `upgrade_pending` (rollback binary), reconcile mengembalikan status sebelumnya dengan event `reconciled` (`version_restored`). `from` adalah versi ter-install, `to` versi binary. Downgrade ditolak: modul `failed` dengan `downgrade_not_supported`, dan pulih otomatis bila binary kembali ke versi ter-install.

## 7. Kegagalan dan `retry`

- Langkah yang mengubah data (`migrate`, `hook:install`, `hook:upgrade`) gagal → modul `failed`. Langkah lain (`hook:enable`, `health`, `hook:disable`) gagal → status modul tidak berubah, operasi `failed`.
- Kegagalan `migration_failed` dan `internal_error` dilaporkan dengan `message` ringkas (nama berkas migrasi, `operation_id`, `trace_id`) tanpa teks SQL atau hook mentah; detail penuh ada di log server. `code` dan `step` tetap. Event `interrupted` mencatat actor pelaku penandaan dan actor asal di `details.originalActor`.
- `erpora modules retry <operation-id>` (atau `POST /api/platform/modules/{key}/retry`) mengulang operasi `failed` terakhir modul: langkah yang sudah `succeeded` dilewati kecuali `migrate` dan `hook:enable` (efek `hook:enable` dibatalkan kompensasi `hook:disable` saat health gagal, jadi hook itu selalu dijalankan ulang; hook wajib idempoten). Prasyarat diperiksa ulang sebelum operasi dimulai: retry atau upgrade yang akan membuat modul `enabled` ditolak (`dependency_not_ready`) bila ada dependensi wajib yang tidak `enabled`, dan `disable` menolak (`dependents_active`) selama masih ada dependen `enabled` atau `upgrade_pending`/`failed` yang sebelumnya `enabled`. Proses yang mati di tengah operasi meninggalkan operasi `running`; startup/operasi berikutnya menandainya `failed` dengan alasan `interrupted`, lalu dapat di-retry.

## 8. Perilaku `--all`

Dengan `--all`, operasi diterapkan ke semua modul terkompilasi (kecuali platform.core) dalam urutan dependensi (terbalik untuk disable dan uninstall). Modul yang transisinya tidak sah untuk operasi itu dilewati (mis. `enable --all` melewati modul yang belum ter-install). Prasyarat yang gagal pada modul mana pun (dependensi belum siap, versi atau platform tidak kompatibel, modul wajib) menolak seluruh rencana: tidak ada yang dijalankan dan exit code 3. Kegagalan operasi di tengah rangkaian menghentikan sisanya (exit code 4) dan melaporkan hasil yang sudah selesai. `--with-deps` menambahkan dependensi wajib transitif; `--enable` menambahkan rangkaian `enable` setelah seluruh `install`. `erpora modules plan <operasi> …` menampilkan rencana tanpa menjalankannya.

## 9. Menguji hook

Hook adalah method biasa pada modul, jadi uji sebagai unit test dengan `ctx` yang membawa `data.Provider` (database uji) dan `module.Env` buatan:

```go
func TestInstall_SeedsOnlyWhenEmpty(t *testing.T) {
	ctx := data.WithProvider(context.Background(), testProvider{db: db}) // db: PostgreSQL uji
	m := example.New()
	env := module.Env{TenantID: tenantID, Logger: slog.Default()}
	for range 2 { // idempoten: panggilan kedua tidak menambah baris
		if err := m.Install(ctx, env); err != nil {
			t.Fatal(err)
		}
	}
	// assert jumlah notes == 3
}
```

Untuk memverifikasi bahwa `Register` mendaftarkan route, permission, dan command yang benar, pakai `moduletest.NewRecorder()` (`sdk/module/moduletest`) sebagai `module.Registrar`. Alur lifecycle penuh (migrasi, upgrade, dependensi, API) diuji lewat perakitan platform seperti `internal/app/reference_modules_integration_test.go`.
