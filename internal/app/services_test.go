package app

import (
	"context"
	"testing"

	"erpora/internal/platform/config"
	"erpora/sdk/authz"
	"erpora/sdk/export"
)

// M-5 (FR-24.2): platform selalu memasang masker dan auditor ekspor di context modul, untuk setiap mode otorisasi
// dan lingkungan, sehingga audit ekspor tidak bergantung pada klasifikasi kolom (internal pun tercatat).
func TestPlatformServices_AlwaysInstallMaskerAndAuditor(t *testing.T) {
	for _, tc := range []config.Config{
		{Env: config.EnvDevelopment, AuthzStub: "allow"},
		{Env: config.EnvProduction, AuthzStub: "deny"},
		{Env: config.EnvProduction},
		{},
	} {
		svc := platformServices(tc, testLog())
		if svc.Masker == nil || svc.Auditor == nil {
			t.Fatalf("cfg %+v: Masker=%v Auditor=%v, keduanya wajib terpasang", tc, svc.Masker, svc.Auditor)
		}
		ctx := authz.WithFieldMasker(context.Background(), svc.Masker)
		ctx = export.WithAuditor(ctx, svc.Auditor)
		if !export.HasAuditor(ctx) {
			t.Errorf("cfg %+v: HasAuditor false", tc)
		}
	}
}
