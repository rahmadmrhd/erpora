package authorization

import (
	"context"
	"log/slog"
	"sync"

	"erpora/internal/platform/observability"
)

// StubMode adalah mode stub otorisasi: "allow" atau "deny".
type StubMode string

// Stub adalah Authorizer sementara sampai mesin permission PRD-001 tersedia.
// Mode selain "allow" selalu menolak (fail closed). Keputusan allow dicatat
// pada level INFO sekali per permission per logger (agar allow-all terlihat di log tanpa membanjiri),
// keputusan deny pada level WARN setiap kali.
type Stub struct {
	Mode StubMode
	Log  *slog.Logger
}

// allowSeen mencatat (logger, permission) yang keputusan allow-nya sudah dicatat.
var allowSeen sync.Map

type allowKey struct {
	log        *slog.Logger
	permission string
}

// Allow memutuskan menurut Mode dan mencatat permission yang diminta.
func (s Stub) Allow(ctx context.Context, permission string) bool {
	allowed := s.Mode == "allow"
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	level := slog.LevelWarn
	if allowed {
		// PRD-000 FR-05: allow-all harus terlihat di log; cukup sekali per permission.
		if _, seen := allowSeen.LoadOrStore(allowKey{log, permission}, struct{}{}); seen {
			return true
		}
		level = slog.LevelInfo
	}
	attrs := []any{"permission", permission, "mode", string(s.Mode), "allowed", allowed}
	if m := observability.ModuleFromContext(ctx); m != "" {
		attrs = append(attrs, "erp.module.key", m)
	}
	log.Log(ctx, level, "otorisasi stub", attrs...)
	return allowed
}
