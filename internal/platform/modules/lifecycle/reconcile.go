package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"

	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/tenancy"
)

// ReconcileActor adalah actor reconcile saat startup.
const ReconcileActor = "system:reconcile"

// ReconcileReport merangkum perubahan hasil Reconcile.
type ReconcileReport struct {
	UpgradePending, Downgraded, Restored, Missing []string
	Interrupted                                   []uuid.UUID
	LockSkipped                                   bool
}

// compareVersions membandingkan versi binary dengan versi ter-install; ok false bila salah satunya bukan SemVer.
func compareVersions(binary, installed string) (cmp int, ok bool) {
	a, err := semver.StrictNewVersion(binary)
	if err != nil {
		return 0, false
	}
	b, err := semver.StrictNewVersion(installed)
	if err != nil {
		return 0, false
	}
	return a.Compare(b), true
}

// Reconcile menyelaraskan status tersimpan dengan binary (FR-19.6, keputusan 11 dan 13): menandai operasi
// terputus, upgrade_pending untuk versi binary lebih baru, downgrade ditolak, dan memulihkan status saat
// binary kembali ke versi ter-install. Tidak pernah memigrasi, meng-install, atau meng-enable. Bila lock
// tidak diperoleh dalam batas waktu, hanya memuat snapshot (LockSkipped). Selalu diakhiri Refresh.
func (s *Service) Reconcile(ctx context.Context, t tenancy.TenantID) (ReconcileReport, error) {
	var rep ReconcileReport
	var rev int64
	bumped := false
	err := s.d.Store.WithTenantLock(ctx, t, s.lockTimeout(), func(ctx context.Context) error {
		ids, err := s.markInterrupted(ctx, t)
		if err != nil {
			return err
		}
		rep.Interrupted = ids
		list, err := s.d.Store.Installations(ctx, t)
		if err != nil {
			return err
		}
		for _, row := range list {
			bump, r, err := s.reconcileRow(ctx, t, row, &rep)
			if err != nil {
				return err
			}
			if bump {
				bumped, rev = true, r
			}
		}
		return nil
	})
	if errors.Is(err, store.ErrLockTimeout) {
		rep.LockSkipped = true
		s.log.WarnContext(ctx, "reconcile dilewati: lock lifecycle tidak diperoleh; hanya memuat snapshot")
		err = nil
	}
	if err != nil {
		return rep, err
	}
	if _, err := s.Refresh(ctx, t); err != nil {
		return rep, err
	}
	if bumped && s.d.Notifier != nil {
		s.d.Notifier.Publish(ctx, t, rev)
	}
	return rep, nil
}

func (s *Service) reconcileRow(ctx context.Context, t tenancy.TenantID, row store.Installation, rep *ReconcileReport) (bool, int64, error) {
	if row.ModuleKey == registry.PlatformKey {
		return false, 0, nil
	}
	entry, ok := s.d.Registry.Entry(row.ModuleKey)
	if !ok {
		rep.Missing = append(rep.Missing, row.ModuleKey)
		s.log.WarnContext(ctx, "modul tersimpan tidak ada di binary (status missing)", "erp.module.key", row.ModuleKey)
		return false, 0, nil
	}
	if row.InstalledVersion == "" {
		return false, 0, nil
	}
	cmp, ok := compareVersions(entry.Manifest.Version, row.InstalledVersion)
	if !ok {
		return false, 0, nil
	}
	status := runtime.Status(row.Status)
	in := row
	in.TenantID = t

	switch {
	case cmp > 0 && (status == runtime.StatusInstalled || status == runtime.StatusEnabled || status == runtime.StatusDisabled):
		in.Status, in.PreviousStatus = string(runtime.StatusUpgradePending), row.Status
		rev, err := s.d.Store.Commit(ctx, t, store.Commit{
			Installation: &in, BumpRevision: true,
			Events: []store.Event{{ModuleKey: row.ModuleKey, Event: "reconciled", Actor: ReconcileActor,
				Details: map[string]any{"reason": "upgrade_pending", "installedVersion": row.InstalledVersion, "binaryVersion": entry.Manifest.Version}}},
		})
		if err != nil {
			return false, 0, fmt.Errorf("reconcile %s: %w", row.ModuleKey, err)
		}
		rep.UpgradePending = append(rep.UpgradePending, row.ModuleKey)
		return true, rev, nil

	case cmp < 0 && status != runtime.StatusFailed && status != runtime.StatusUninstalled:
		opID, err := uuid.NewV7()
		if err != nil {
			return false, 0, err
		}
		if err := s.d.Store.StartOperation(ctx, store.Operation{
			ID: opID, TenantID: t, ModuleKey: row.ModuleKey, Operation: "reconcile",
			FromVersion: row.InstalledVersion, ToVersion: entry.Manifest.Version, Status: "running", Actor: ReconcileActor,
		}); err != nil {
			return false, 0, err
		}
		msg := fmt.Sprintf("versi binary %s lebih lama dari versi ter-install %s", entry.Manifest.Version, row.InstalledVersion)
		in.Status, in.PreviousStatus, in.LastOperationID = string(runtime.StatusFailed), row.Status, opID
		rev, err := s.d.Store.Commit(ctx, t, store.Commit{
			Installation: &in, BumpRevision: true,
			Finish: &store.Finish{OperationID: opID, Status: "failed", Error: &store.OpError{Code: CodeDowngradeNotSupported, Message: msg}},
			Events: []store.Event{{ModuleKey: row.ModuleKey, Event: "operation_failed", Actor: ReconcileActor, OperationID: opID,
				Details: map[string]any{"operation": "reconcile", "code": CodeDowngradeNotSupported, "message": msg}}},
		})
		if err != nil {
			return false, 0, fmt.Errorf("reconcile %s: %w", row.ModuleKey, err)
		}
		rep.Downgraded = append(rep.Downgraded, row.ModuleKey)
		s.log.WarnContext(ctx, "downgrade modul ditolak", "erp.module.key", row.ModuleKey, "message", msg)
		return true, rev, nil

	case cmp == 0 && status == runtime.StatusUpgradePending && row.PreviousStatus != "":
		// Binary kembali ke versi ter-install (mis. rollback binary): upgrade_pending tidak lagi berlaku (Q32 B-3).
		in.Status, in.PreviousStatus = row.PreviousStatus, ""
		rev, err := s.d.Store.Commit(ctx, t, store.Commit{
			Installation: &in, BumpRevision: true,
			Events: []store.Event{{ModuleKey: row.ModuleKey, Event: "reconciled", Actor: ReconcileActor,
				Details: map[string]any{"reason": "version_restored", "status": row.PreviousStatus, "from": string(runtime.StatusUpgradePending)}}},
		})
		if err != nil {
			return false, 0, fmt.Errorf("reconcile %s: %w", row.ModuleKey, err)
		}
		rep.Restored = append(rep.Restored, row.ModuleKey)
		return true, rev, nil

	case cmp == 0 && status == runtime.StatusFailed && row.LastOperationID != uuid.Nil && row.PreviousStatus != "":
		op, err := s.d.Store.Operation(ctx, t, row.LastOperationID)
		if errors.Is(err, store.ErrNotFound) {
			return false, 0, nil
		}
		if err != nil {
			return false, 0, err
		}
		if op.Operation != "reconcile" || op.Error == nil || op.Error.Code != CodeDowngradeNotSupported {
			return false, 0, nil
		}
		in.Status, in.PreviousStatus = row.PreviousStatus, ""
		rev, err := s.d.Store.Commit(ctx, t, store.Commit{
			Installation: &in, BumpRevision: true,
			Events: []store.Event{{ModuleKey: row.ModuleKey, Event: "reconciled", Actor: ReconcileActor,
				Details: map[string]any{"reason": "version_restored", "status": row.PreviousStatus}}},
		})
		if err != nil {
			return false, 0, fmt.Errorf("reconcile %s: %w", row.ModuleKey, err)
		}
		rep.Restored = append(rep.Restored, row.ModuleKey)
		return true, rev, nil
	}
	return false, 0, nil
}
