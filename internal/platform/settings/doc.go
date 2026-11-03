// Package settings adalah layanan Settings platform. Saat ini hanya SUBSET MINIMAL C1 (keputusan pemilik produk
// 3 Nov 2026, PRD-001 Q24): satu API tertutup untuk kunci platform "platform.public_url" (URL publik aplikasi untuk
// tautan email reset sandi dan pemeriksaan Origin cookie refresh) di atas settings/store (tabel tenant settings,
// migrasi tenant 0004_settings). Tidak ada env ERP_PUBLIC_URL dan URL tidak pernah diturunkan dari header permintaan.
//
// API:
//
//	NewService(st Store, Options{Production, CacheTTL, Now}) *Service
//	(*Service).PublicURL(ctx) (string, error)                         // cache dalam memori; ErrNotConfigured bila belum diisi
//	(*Service).SetPublicURL(ctx, rawURL, updatedBy string) error      // validasi + normalisasi; dipanggil core:settings:set (C1.12)
//	(*Service).PublicURLOrDevDefault(ctx, devDefault string) (string, error) // default HANYA di development dan HANYA bila diminta
//	(*Service).Invalidate(t) / InvalidateAll()                        // untuk notifikasi perubahan lintas instance
//	NormalizePublicURL(raw string, production bool) (string, error)
//	OriginOf(rawURL string) (string, error)                           // skema://host[:port] untuk pemeriksa Origin
//	DevURLFromHTTPAddr(addr string) string                            // ERP_HTTP_ADDR -> http://host:port (development)
//
// Penegakan di production (serve gagal start bila ErrNotConfigured) dilakukan perakit, bukan paket ini. Settings
// penuh (registri schema, validasi per tipe, audit, cache revisi, Get[T], UI) adalah C3 (FR-120) dan memperluas
// paket ini secara aditif.
package settings
