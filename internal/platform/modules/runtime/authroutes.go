package runtime

import (
	"erpora/internal/platform/modules/registry"
)

// CoreModuleKey adalah key modul `core` (PRD-001; slug `core`, required). Daftar di bawah hanya berlaku bagi route
// milik modul ini: modul lain yang kebetulan memakai path serupa tidak mendapat pelonggaran apa pun.
const CoreModuleKey = "erp.core"

// coreAuthPrefix adalah awalan path endpoint auth/token modul core (ADR-0013 §4).
const coreAuthPrefix = "/api/core/auth/"

// Route auth core yang boleh dipanggil sesi yang belum memenuhi MFA (ADR-0013 §4). Daftar ini milik PLATFORM, bukan
// opsi route modul: bila modul dapat menandai route sendiri sebagai "boleh tanpa MFA", modul pihak ketiga dapat
// melewati MFA. C1.11 cukup mendaftarkan route dengan method dan path persis ini; tidak ada anotasi yang perlu diingat.
var (
	// mfaPendingVerify: sesi dengan faktor aktif tetapi belum terverifikasi (amr tanpa otp).
	mfaPendingVerify = map[string]bool{
		"POST /api/core/auth/mfa/verify":   true,
		"POST /api/core/auth/mfa/recovery": true,
		"POST /api/core/auth/logout":       true,
	}
	// mfaPendingEnroll: administrator yang belum punya faktor.
	mfaPendingEnroll = map[string]bool{
		"POST /api/core/auth/mfa/enroll":  true,
		"POST /api/core/auth/mfa/confirm": true,
		"POST /api/core/auth/logout":      true,
	}
)

// isCoreRoute melaporkan apakah rt milik modul core (key persis erp.core).
func isCoreRoute(rt registry.RouteInfo) bool { return rt.Module == CoreModuleKey }

// isAuthEndpoint: endpoint auth/token modul core, yang dibatasi lajunya per IP (dan per principal bila terautentikasi).
func isAuthEndpoint(rt registry.RouteInfo) bool {
	return isCoreRoute(rt) && len(rt.Pattern) > len(coreAuthPrefix) && rt.Pattern[:len(coreAuthPrefix)] == coreAuthPrefix
}

// routeOptionsFor menurunkan RouteOptions dari route terdaftar.
func routeOptionsFor(rt registry.RouteInfo) RouteOptions {
	o := RouteOptions{RequireMFA: rt.RequireMFA, AuthenticatedOnly: rt.Authenticated}
	if isCoreRoute(rt) {
		key := rt.Method + " " + rt.Pattern
		o.PendingVerify = mfaPendingVerify[key]
		o.PendingEnroll = mfaPendingEnroll[key]
	}
	return o
}
