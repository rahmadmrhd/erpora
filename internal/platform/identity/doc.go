// Package identity adalah mesin identitas platform (PRD-001 C1): middleware Authenticate dan pembatas laju HTTP
// (middleware.go, limit.go), pembaca keadaan MFA untuk keputusan otorisasi (mfastate.go), implementasi
// sdk/identity.Service dan Admin (service.go, admin.go; dependensi lewat antarmuka kecil di deps.go, galat platform
// diterjemahkan ke galat sdk di errmap.go), serta sub-paket password, token, session, guard, mfa, reset, dan store.
// Antarmuka yang dipakai modul ada di sdk/identity (D9); paket ini hanya dirakit di internal/app.
package identity
