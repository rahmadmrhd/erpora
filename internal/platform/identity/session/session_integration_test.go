//go:build integration

package session_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/pgtest"
)

// pgBackend memakai *store.Store sungguhan di PostgreSQL (migrasi penuh).
type pgBackend struct{ *store.Store }

func (b pgBackend) NewUser(t *testing.T, ctx context.Context) string {
	t.Helper()
	tn, _ := tenancy.FromContext(ctx)
	u, err := b.CreateUser(ctx, tn, store.NewUser{Email: uuid.NewString() + "@example.test", Name: "Uji", PasswordHash: "$argon2id$stub"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u.ID
}

func (b pgBackend) SetStatus(t *testing.T, ctx context.Context, userID, status string) {
	t.Helper()
	tn, _ := tenancy.FromContext(ctx)
	var err error
	switch status {
	case store.UserLocked:
		var ok bool
		ok, err = b.LockUser(ctx, tn, userID, time.Now())
		if err == nil && !ok {
			t.Fatal("LockUser: tidak ada efek")
		}
	default:
		err = b.SetUserStatus(ctx, tn, userID, status)
	}
	if err != nil {
		t.Fatalf("SetStatus(%s): %v", status, err)
	}
}

func newPGWorld(t *testing.T, dsn string) *world {
	t.Helper()
	db, err := database.Open(context.Background(),
		config.Config{DatabaseURL: dsn, DBSlowQuery: 200 * time.Millisecond}, noop.NewTracerProvider(), slog.Default())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	return newWorld(t, pgBackend{store.New(database.ControlDB{DB: db}, nil)}, time.Now())
}

// TestIntegrationSuite menjalankan seluruh suite terhadap PostgreSQL sungguhan dan store.Store asli. Satu
// database untuk semua sub-uji (migrasi sekali); tiap sub-uji memakai pengguna dan jam sendiri.
func TestIntegrationSuite(t *testing.T) {
	dsn := pgtest.Start(t)
	{
		db, err := database.Open(context.Background(),
			config.Config{DatabaseURL: dsn, DBSlowQuery: 200 * time.Millisecond}, noop.NewTracerProvider(), slog.Default())
		if err != nil {
			t.Fatal(err)
		}
		if err := migrate.Up(context.Background(), db, config.EnvProduction); err != nil {
			t.Fatal(err)
		}
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	}
	cases := []struct {
		name string
		fn   func(*testing.T, *world)
	}{
		{"RotateIssuesNewTokenAndMarksParentUsed", suiteRotateIssuesNewTokenAndMarksParentUsed},
		{"ReuseOfUsedTokenRevokesFamilyAndSession", suiteReuseOfUsedTokenRevokesFamilyAndSession},
		{"ConcurrentRefreshSameTokenExactlyOneSucceeds", suiteConcurrentRefreshSameTokenExactlyOneSucceeds},
		{"AbsoluteLifetime90DaysEnforced", suiteAbsoluteLifetime90DaysEnforced},
		{"SlidingExpiry14Days", suiteSlidingExpiry14Days},
		{"RevocationVisibleToOtherInstanceViaNotifier", suiteRevocationVisibleToOtherInstanceViaNotifier},
		{"RevocationListLoadedOnStartup", suiteRevocationListLoadedOnStartup},
		{"RevocationEntriesRetainedForAccessTokenTTLPlusLeeway", suiteRevocationEntriesRetainedForAccessTokenTTLPlusLeeway},
		{"LostNotificationBoundedAndRecoveredByReload", suiteLostNotificationBoundedAndRecoveredByReload},
		{"RefreshRejectsMissingOrCrossOriginOrigin", suiteRefreshRejectsMissingOrCrossOriginOrigin},
		{"DisabledUserSessionsRevokedImmediately", suiteDisabledUserSessionsRevokedImmediately},
		{"IDORRevokeOwnAndCrossTenant", suiteIDOR},
		{"MalformedTokensRejectedWithoutPanic", suiteMalformedTokensRejectedWithoutPanic},
		{"StartRules", suiteStartRules},
		{"RevokeIsIdempotentAndBroadcastsOnce", suiteRevokeIsIdempotentAndBroadcastsOnce},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.fn(t, newPGWorld(t, dsn)) })
	}
}
