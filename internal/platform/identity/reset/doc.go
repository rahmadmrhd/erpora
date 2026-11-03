// Package reset menjalankan reset sandi lewat tautan email (PRD-001 FR-101, FR-108; ADR-0013; Review Focus 5). Paket ini
// bebas GORM: persistensi lewat antarmuka Store yang dipenuhi *store.Store.
//
// # API
//
//	New(Config) (*Service, error)
//	(*Service).Request(ctx, email, locale string) error           // forgot: selalu nil kecuali 429/infrastruktur
//	(*Service).Complete(ctx, rawToken, newPassword string) error  // reset: sekali pakai, 30 menit
//	(*Service).Wait()                                             // tunggu pekerjaan kirim latar (tes, shutdown)
//	WithClientIP(ctx, ip) context.Context                         // IP klien terpercaya untuk batas laju per IP
//	SupportedLocales() []string                                   // id, en
//
// Galat yang dipetakan handler (C1.11): *guard.RateLimitedError (guard.AsRateLimited -> httpx.TooManyRequests, 429 +
// Retry-After; dari Request maupun dari Complete bila pembatas hash penuh), ErrInvalidToken (token cacat, tak dikenal,
// kedaluwarsa, terpakai, atau kalah balapan: SATU jawaban seragam, mis. 422/400 generik), *PolicyError (422 dengan
// kode Violation; tautan tidak terpakai), ErrNoTenant dan galat lain (500).
//
// # Keputusan desain
//
//   - Request tidak membedakan akun ada/tak ada/nonaktif: semua pekerjaan yang bergantung pada akun (cari akun,
//     batalkan tautan lama, simpan hash, render, kirim) berjalan di goroutine latar yang terbatas (MaxInFlight; penuh =
//     dibuang tanpa mengubah jawaban), lalu jawaban menunggu lantai MinRequestDuration (bawaan 100 ms; pola
//     guard.Authenticator). Disetujui pemilik produk 3 Nov 2026 (FR-108 diubah, PRD-001 Q23): driver mail tetap
//     sinkron; konsekuensinya email dapat hilang bila proses mati sebelum terkirim dan pengguna mencoba lagi.
//   - Batas laju forgot per IP lalu per email (instans PrincipalLimiter/IPLimiter terpisah dan lebih ketat: bawaan 3
//     per email lalu 1 per 10 menit; 10 per IP lalu 1 per 30 detik), dihitung SEBELUM pekerjaan apa pun dan tidak
//     bergantung pada keberadaan akun. Residual: penyerang dapat menguras jatah per email korban sehingga reset sah
//     memperoleh 429 sesaat (sembuh sendiri), sama seperti batas per email login.
//   - Token 32 byte crypto/rand, base64url tanpa padding; hanya SHA-256-nya disimpan; berlaku 30 menit; permintaan
//     baru membatalkan tautan lama (Invalidate lalu Insert). Dua Request serentak untuk akun yang sama dapat
//     meninggalkan dua tautan berlaku; tidak berbahaya karena Complete menghanguskan semua tautan pengguna itu.
//   - Tautan = URL publik + ResetPath (bawaan /reset-password) + "#token=<token>" (bentuk disetujui pemilik produk 3
//     Nov 2026). URL publik berasal dari settings platform lewat PublicURLSource (dipenuhi settings.Service.PublicURL;
//     BUKAN env dan tidak pernah dari header Host/X-Forwarded-*), dibaca dan divalidasi tiap permintaan. Bila belum
//     dikonfigurasi, galat, atau nilainya tidak sah: Request tetap menjawab seragam, mencatat peringatan terstruktur
//     tanpa data sensitif, dan tidak membatalkan/menyimpan tautan maupun mengirim email. Token di fragmen (bukan query)
//     agar tidak masuk log akses, proksi, atau Referer; halaman frontend membacanya dari location.hash lalu
//     menghapusnya dari URL.
//   - Penerima email adalah alamat tersimpan (bukan masukan mentah); akun disabled tidak menerima email; akun locked
//     menerima (reset adalah salah satu jalur unlock). Locale email: user_preferences.locale, lalu parameter, lalu "id".
//   - Log tidak pernah memuat email, token, atau tautan (hanya user_id).
//   - Complete: token cacat ditolak tanpa DB -> cari hash -> kebijakan sandi (sebelum hash argon2 dan sebelum menyentuh
//     status token) -> hash lewat pembatas hash -> store.ResetPassword atomik -> session.MarkRevoked.
//
// # Cara merakit (internal/app, C1.10/C1.12)
//
//	gate := ... // *guard.HashGate yang SAMA dengan login (satu pembatas hash per proses)
//	svc, err := reset.New(reset.Config{
//		Store: identityStore, Sessions: sessionService, Gate: gate, Sender: sender,
//		PublicURL: settingsService, // *settings.Service (PublicURL(ctx) (string, error))
//		Argon2:    cfg.Argon2, Logger: logger,
//	})
//
// Handler forgot memanggil svc.Request(reset.WithClientIP(ctx, trustedIP), in.Email, in.Locale) dan SELALU menjawab
// 202/200 seragam kecuali 429. Handler reset memanggil svc.Complete. Tenant dari tenantContext. Batas laju per IP untuk
// endpoint reset dipasang middleware (ADR-0013 §4). Pada shutdown panggil svc.Wait().
package reset
