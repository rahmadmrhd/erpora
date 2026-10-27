package lifecycle

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/tenancy"
)

// markInterrupted menandai setiap operasi running milik tenant sebagai failed/interrupted (keputusan 11).
// Wajib dipanggil di dalam advisory lock tenant: selama lock dipegang tidak ada operasi lain yang hidup,
// sehingga baris running pasti sisa proses yang mati. Modul ikut failed hanya untuk install/upgrade.
func (s *Service) markInterrupted(ctx context.Context, t tenancy.TenantID) ([]uuid.UUID, error) {
	ops, err := s.d.Store.RunningOperations(ctx, t)
	if err != nil || len(ops) == 0 {
		return nil, err
	}
	rows, err := s.loadRows(ctx, t)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	var rev int64
	bumped := false
	for _, op := range ops {
		steps := slices.Clone(op.Steps)
		last := ""
		for i := range steps {
			if steps[i].Status == "running" {
				steps[i].Status, steps[i].Error = "failed", CodeInterrupted
				last = steps[i].Name
			}
		}
		if last == "" && len(steps) > 0 {
			last = steps[len(steps)-1].Name
		}
		if len(steps) > 0 {
			if err := s.d.Store.SaveSteps(ctx, t, op.ID, steps); err != nil {
				s.log.WarnContext(ctx, "simpan langkah operasi terputus gagal", "operation_id", op.ID.String(), "error", err)
			}
		}
		c := store.Commit{
			BumpRevision: true, // lastOperation berubah: ETag (= revision) harus berubah (Q32 B-4)
			Finish: &store.Finish{OperationID: op.ID, Status: "failed", Error: &store.OpError{
				Code: CodeInterrupted, Message: "operasi terputus sebelum selesai", Step: last,
			}},
			Events: []store.Event{{
				ModuleKey: op.ModuleKey, Event: "interrupted", Actor: op.Actor, OperationID: op.ID,
				Details: map[string]any{"operation": op.Operation, "step": last},
			}},
		}
		if op.Operation == string(OpInstall) || op.Operation == string(OpUpgrade) {
			cur, hasRow := rows[op.ModuleKey]
			in := cur
			in.TenantID, in.ModuleKey, in.LastOperationID = t, op.ModuleKey, op.ID
			in.Status = string(runtime.StatusFailed)
			if op.Operation == string(OpInstall) {
				origin := string(runtime.StatusAvailable)
				if hasRow {
					origin = cur.Status
					if cur.Status == string(runtime.StatusFailed) {
						origin = cur.PreviousStatus
					}
				}
				in.PreviousStatus = origin
			}
			c.Installation = &in
			c.BumpRevision = true
			rows[op.ModuleKey] = in
		}
		r, err := s.d.Store.Commit(ctx, t, c)
		if err != nil {
			return ids, fmt.Errorf("tandai operasi %s interrupted: %w", op.ID, err)
		}
		if c.BumpRevision {
			bumped, rev = true, r
		}
		ids = append(ids, op.ID)
		s.log.WarnContext(ctx, "operasi lifecycle terputus ditandai failed", "operation_id", op.ID.String(),
			"erp.module.key", op.ModuleKey, "operation", op.Operation, "step", last)
	}
	if bumped {
		if _, err := s.Refresh(ctx, t); err != nil {
			s.log.WarnContext(ctx, "refresh snapshot gagal setelah interrupted", "error", err)
		}
		if s.d.Notifier != nil {
			s.d.Notifier.Publish(ctx, t, rev)
		}
	}
	return ids, nil
}
