package lifecycle

import (
	"context"

	"github.com/google/uuid"

	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/tenancy"
)

// Retry mengulang operasi failed berID opID (keputusan 12): hanya untuk operasi yang menjadi
// last_operation_id modulnya dan bukan reconcile. Langkah yang sudah succeeded dilewati kecuali migrate.
func (s *Service) Retry(ctx context.Context, t tenancy.TenantID, opID uuid.UUID, actor string) (Result, error) {
	return s.retry(ctx, t, actor, "", func(ctx context.Context) (store.Operation, error) {
		return s.Operation(ctx, t, opID)
	})
}

// RetryModule me-retry operasi terakhir modul key.
func (s *Service) RetryModule(ctx context.Context, t tenancy.TenantID, key, actor string) (Result, error) {
	return s.retry(ctx, t, actor, key, func(ctx context.Context) (store.Operation, error) {
		if _, err := s.knownKey(ctx, t, key); err != nil {
			return store.Operation{}, err
		}
		ops, err := s.d.Store.Operations(ctx, t, key, 1)
		if err != nil {
			return store.Operation{}, err
		}
		if len(ops) == 0 {
			return store.Operation{}, &Error{Code: CodeOperationNotFound, Module: key, Message: "modul belum punya operasi"}
		}
		return ops[0], nil
	})
}

func (s *Service) retry(ctx context.Context, t tenancy.TenantID, actor, key string, find func(context.Context) (store.Operation, error)) (Result, error) {
	var res Result
	err := s.d.Store.WithTenantLock(ctx, t, s.lockTimeout(), func(ctx context.Context) error {
		// Operasi terputus menjadi failed lebih dulu agar dapat di-retry.
		if _, err := s.markInterrupted(ctx, t); err != nil {
			return err
		}
		op, err := find(ctx)
		if err != nil {
			return err
		}
		if op.Status != "failed" {
			return &Error{Code: CodeInvalidTransition, Module: op.ModuleKey, Message: "hanya operasi failed yang dapat di-retry"}
		}
		if op.Operation == "reconcile" {
			return &Error{Code: CodeInvalidTransition, Module: op.ModuleKey, Message: "operasi reconcile tidak dapat di-retry"}
		}
		if _, ok := s.d.Registry.Entry(op.ModuleKey); !ok || op.ModuleKey == registry.PlatformKey {
			return &Error{Code: CodeModuleNotFound, Module: op.ModuleKey, Message: "modul tidak ada di katalog"}
		}
		rows, err := s.loadRows(ctx, t)
		if err != nil {
			return err
		}
		if rows[op.ModuleKey].LastOperationID != op.ID {
			return &Error{Code: CodeInvalidTransition, Module: op.ModuleKey, Message: "hanya operasi terakhir modul yang dapat di-retry"}
		}
		skip := map[string]bool{}
		for _, st := range op.Steps {
			if st.Status == "succeeded" {
				skip[st.Name] = true
			}
		}
		r, err := s.runOne(ctx, Request{Tenant: t, Op: Op(op.Operation), Key: op.ModuleKey, Actor: actor},
			op.ModuleKey, Op(op.Operation), &resume{skip: skip, retryOf: op.ID})
		res = r
		return err
	})
	err = mapLockErr(err, key)
	if err != nil {
		s.logRejected(ctx, Request{Op: OpRetry, Key: key, Actor: actor}, err)
		return Result{}, err
	}
	return res, nil
}
