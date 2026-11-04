// Package guard menegakkan pembatasan tebakan kredensial PRD-001 C1 (FR-101, FR-108, Q16, Q17; ADR-0013 §4 dan §5):
// batas laju per IP dan per principal/identitas sasaran, jeda dan penguncian bertingkat, pembatas hash serentak, dan
// Authenticator yang menggabungkan semuanya menjadi satu hasil seragam untuk service login (C1.10/C1.11).
//
// Paket ini tidak mengimpor GORM: persistensi lewat antarmuka kecil AttemptStore dan LoginAccounts yang dipenuhi
// *store.Store. Tenant selalu dari context (tenancy.FromContext; tanpa tenant: ErrNoTenant). Jam dapat diinjeksi.
//
// # Komponen
//
//	NewIPLimiter(LimiterConfig) (*IPLimiter, error)              // Allow(ip) (ok bool, retryAfter time.Duration)
//	NewPrincipalLimiter(LimiterConfig) (*PrincipalLimiter, error) // Allow(key) (ok bool, retryAfter time.Duration)
//	TargetKey / EmailTargetKey(scope, tenant, ...)               // kunci sasaran ter-hash (tanpa email di memori)
//	NormalizeIP(ip) string                                       // IPv6 -> /64, IPv4-mapped -> IPv4, tak terurai -> "unknown"; IDEMPOTEN
//	NewLockout(AttemptStore, LockoutConfig, now) (*Lockout, error)
//	  Check(ctx, userID, ip) (State, error)                      // BACA-SAJA: jeda pasangan + akun locked
//	  RegisterFailure / RegisterMFAFailure(ctx, userID, ip) (State, error)
//	  RegisterSuccess(ctx, userID, ip) error
//	  Reserve(ctx, kind, userID, ip) (Reservation, State, error) // atomik per pasangan: cek + catat gagal SEBELUM verifikasi
//	  Fail(ctx, Reservation) (State, error) / Release(ctx, Reservation) error // selesaikan: gagal (kunci akun) / bukan gagal
//	  Unlock(ctx, userID) error                                  // idempoten
//	NewHashGate(capacity, maxWait) (*HashGate, error)            // Do, Verify, Hash; ErrHashBusy -> *RateLimitedError
//	NewAuthenticator(AuthenticatorConfig) (*Authenticator, error)
//	  Verify(ctx, LoginInput{Email, Password, IP}) (Outcome, error)
//	  RegisterSuccess(ctx, userID, ip) error
//	AsRateLimited(err) (retryAfter, ok)                          // jawab httpx.TooManyRequests(w, r, retryAfter)
//
// # Aturan pemakaian
//
//   - Urutan middleware (ADR-0013 §4): batas IP (IPLimiter) sebelum Authenticate, batas principal setelahnya. Untuk
//     login, batas per email ada di dalam Authenticator (kunci EmailTargetKey("login", ...)); isi
//     AuthenticatorConfig.IP hanya bila middleware IP tidak dipasang (hindari hitung ganda). Endpoint `forgot` memakai
//     PrincipalLimiter terpisah yang jauh lebih ketat (kunci EmailTargetKey("forgot", ...)).
//   - Authenticator.Verify mengembalikan Success, InvalidCredentials (401 seragam), atau RateLimited (429 + Retry-After).
//     Reason hanya untuk log keamanan. Galat (bukan Outcome) berarti kegagalan infrastruktur.
//   - Success belum mencatat sukses: panggil RegisterSuccess HANYA setelah autentikasi selesai penuh (setelah MFA bila
//     akun berfaktor aktif), karena sukses mereset hitungan pasangan. Hash dengan NeedsRehash disimpan ulang (bersyarat pada
//     Outcome.RehashFrom lewat Store.RehashPassword, agar tidak menimpa reset sandi serentak) lewat
//     HashGate.Hash.
//   - Pemeriksa guess-limit lain (MFA, C1.9) memanggil Lockout.Reserve sebelum memverifikasi kode: bila PairBlocked atau
//     AccountLocked ditolak tanpa memverifikasi dan tanpa mencatat; selain itu percobaan sudah tercatat sebagai kegagalan
//     (atomik lintas instance lewat kunci advisory per pasangan, sehingga permintaan serentak tidak lolos bersama-sama).
//     Kode salah: Fail; kode benar atau keluar bukan-kegagalan: Release. (Check + RegisterMFAFailure tetap ada untuk
//     pemanggil lama; tidak membatasi tebakan serentak.)
//     guard.State identik bentuknya dengan mfa.LockState (PairBlocked, AccountLocked, RetryAfter): konversi
//     mfa.LockState(st) cukup di perekat (internal/app).
//
// # Jadwal lockout (Q16, DefaultLockoutConfig)
//
// Per pasangan (akun, IP): gagal ke-5 memicu jeda 30 detik, berlipat dua tiap kegagalan berikutnya (60, 120, 240, 480,
// lalu 900 detik = maksimum 15 menit), gagal ke-20 memblokir pasangan 1 jam; semuanya dihitung sejak kegagalan
// terakhir dan hanya atas kegagalan dalam jendela 24 jam terakhir (LockoutConfig.PairWindow; keputusan pemilik produk
// 3 Nov 2026, PRD-001 Q19): kegagalan lebih tua tidak dihitung, jadi blokir 1 jam tidak memberi jatah baru 20
// tebakan (hitungan masih 20 dalam jendela; satu gagal lagi memblokir 1 jam lagi). Percobaan yang ditolak selama jeda TIDAK dicatat. Akun berstatus locked hanya bila >= 50 gagal lintas >= 5
// IP berbeda dalam 1 jam; tidak terbuka sendiri (unlock lewat reset sandi, core:unlock, atau admin). Sukses
// mereset hitungan pasangan itu saja.
//
// # Keseragaman respons (anti-enumerasi, Review Focus 1)
//
//   - Email tak dikenal, sandi salah, akun terkunci, akun dinonaktifkan, dan hash tersimpan cacat semuanya dijawab
//     InvalidCredentials; sandi BENAR pada akun terkunci tetap ditolak seragam.
//   - Verifikasi untuk semua jalur itu berbiaya argon2 sama: akun nyata aktif memakai hash tersimpan, selainnya memakai
//     password.NewDummy(cfg.Argon2) (parameter terkonfigurasi, bukan password.Dummy()); ErrInvalidHash (gagal sebelum
//     argon2) diikuti verifikasi tiruan. Semua lewat HashGate.
//   - Urutan panggilan database identik pada semua jalur gagal (jalur email tak dikenal memanggil store yang sama
//     dengan UUID acak: INSERT yang tak memasukkan baris, dua SELECT), dan jawaban gagal baru dikirim setelah lantai
//     MinFailureDuration (default 200 ms).
//   - Jeda pasangan untuk email tak dikenal dilacak di memori (unknownPairs) dengan jadwal identik, sebab
//     login_attempts mensyaratkan user_id NOT NULL (Q18). Karena itu respons 429 jeda tidak membedakan akun ada atau
//     tidak. Residual (keputusan implementer, lihat laporan C1.7): pelacak ini per instance (jendela 24 jam sama), jadi pada penerapan
//     multi-instance di balik penyeimbang beban hitungan email tak dikenal tersebar per instance sedangkan akun nyata
//     berbagi hitungan di database; selisih itu hanya teramati bila penyerang mampu memisahkan instance.
//
// # Residual yang diketahui
//
//   - Batas per email (login) dapat dikuras penyerang berIP banyak sehingga login sah memperoleh 429 sesaat (sembuh
//     sendiri); melekat pada batas per sasaran yang diwajibkan ADR-0013 §4.
//   - Retensi login_attempts (store.PruneLoginAttempts) harus >= PairWindow (24 jam) agar jendela jeda pasangan tidak
//     terpotong; tugas pemeliharaan itu belum dijadwalkan di mana pun.
//   - Lockout.Unlock (store.UnlockUser) SELALU menghapus seluruh catatan percobaan akun, juga pada akun active yang
//     hanya terblokir per pasangan: jeda/blokir pasangan hilang bersamanya.
package guard
