package authorization

import (
	"context"
	"log/slog"
)

// StubMode adalah mode stub otorisasi: "allow" atau "deny".
type StubMode string

// Stub adalah Authorizer sementara sampai mesin permission PRD-001 tersedia.
// Mode selain "allow" selalu menolak (fail closed). Keputusan allow dicatat
// pada level DEBUG, keputusan deny pada level WARN.
type Stub struct {
	Mode StubMode
	Log  *slog.Logger
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
		level = slog.LevelDebug
	}
	log.Log(ctx, level, "otorisasi stub", "permission", permission, "mode", string(s.Mode), "allowed", allowed)
	return allowed
}
