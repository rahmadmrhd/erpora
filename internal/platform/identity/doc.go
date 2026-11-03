// Package identity adalah mesin identitas platform (PRD-001 C1): middleware Authenticate dan pembatas laju HTTP
// (middleware.go, limit.go), pembaca keadaan MFA untuk keputusan otorisasi (mfastate.go), serta sub-paket password,
// token, session, guard, mfa, dan store. Antarmuka yang dipakai modul ada di sdk/identity (D9); paket ini hanya
// dirakit di internal/app.
package identity
