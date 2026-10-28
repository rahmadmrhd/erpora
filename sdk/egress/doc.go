// Package egress adalah satu-satunya HTTP client keluar yang boleh dipakai modul untuk menghubungi
// URL yang dipengaruhi data (security-guidelines §3.3, PRD-000 FR-24.2). Client memvalidasi skema dan
// host sebelum request, memvalidasi IP sesudah resolve di DialContext lalu dial langsung ke IP
// tervalidasi (menutup DNS rebinding), membatasi redirect, ukuran respons, dan waktu, serta tidak
// memakai proxy dari environment. HTTP client mentah (net/http.Client, http.Get, dst.) dilarang di
// modul oleh aturan arch EGRESS.
//
// Cara pakai:
//
//	c, err := egress.New(egress.Config{AllowHosts: []string{"api.partner.example"}})
//	resp, err := c.Get(ctx, "https://api.partner.example/v1/rates")
package egress
