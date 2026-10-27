package lifecycle

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"

	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/resolver"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/modules/store"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/module"
)

// ModuleState adalah read model satu modul untuk CLI, API, dan bootstrap.
type ModuleState struct {
	Key, Slug, Name, BinaryVersion, InstalledVersion string
	Status                                           runtime.Status // status efektif
	Stored                                           string         // status tersimpan ("available" bila tidak ada baris)
	Required, Compatible                             bool
	Incompatible                                     []resolver.Issue
	Dependencies                                     []module.Dependency
	Dependents                                       []string
	LastOperation                                    *store.Operation
}

// Modules memuat semua modul terkompilasi (tanpa platform.core) berurutan topologis, ditambah baris
// missing di akhir, beserta revision tenant.
func (s *Service) Modules(ctx context.Context, t tenancy.TenantID) ([]ModuleState, int64, error) {
	rev, err := s.d.Store.Revision(ctx, t)
	if err != nil {
		return nil, 0, err
	}
	list, err := s.d.Store.Installations(ctx, t)
	if err != nil {
		return nil, 0, err
	}
	rows := make(map[string]store.Installation, len(list))
	for _, r := range list {
		rows[r.ModuleKey] = r
	}
	eff := Effective(s.d.Registry, list)

	var out []ModuleState
	for _, key := range s.d.Registry.Graph().Order() {
		if key == registry.PlatformKey {
			continue
		}
		st, err := s.stateOf(ctx, t, key, rows, eff)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, st)
	}
	var missing []string
	for k := range rows {
		if _, ok := s.d.Registry.Entry(k); !ok && k != registry.PlatformKey {
			missing = append(missing, k)
		}
	}
	slices.Sort(missing)
	for _, k := range missing {
		st, err := s.stateOf(ctx, t, k, rows, eff)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, st)
	}
	return out, rev, nil
}

func (s *Service) stateOf(ctx context.Context, t tenancy.TenantID, key string, rows map[string]store.Installation, eff map[string]runtime.Status) (ModuleState, error) {
	row, hasRow := rows[key]
	st := ModuleState{Key: key, Status: eff[key], Stored: string(runtime.StatusAvailable), InstalledVersion: row.InstalledVersion, Compatible: true}
	if hasRow {
		st.Stored = row.Status
	}
	if e, ok := s.d.Registry.Entry(key); ok {
		m := e.Manifest
		st.Slug, st.Name, st.BinaryVersion, st.Required = m.Slug, m.Name, m.Version, m.Required
		st.Dependencies = slices.Clone(m.Dependencies)
		st.Incompatible = incompatibleIssues(s.d.Registry.Graph(), key)
		st.Compatible = len(st.Incompatible) == 0
		for _, o := range s.d.Registry.Entries() {
			for _, d := range o.Manifest.Dependencies {
				if d.Module == key {
					st.Dependents = append(st.Dependents, o.Manifest.Key)
				}
			}
		}
		slices.Sort(st.Dependents)
	}
	if hasRow && row.LastOperationID != uuid.Nil {
		op, err := s.d.Store.Operation(ctx, t, row.LastOperationID)
		switch {
		case err == nil:
			st.LastOperation = &op
		case !errors.Is(err, store.ErrNotFound):
			return ModuleState{}, err
		}
	}
	return st, nil
}

// Module memuat satu modul; key yang tidak dikenal (bukan di katalog dan tanpa baris) -> *Error module_not_found.
func (s *Service) Module(ctx context.Context, t tenancy.TenantID, key string) (ModuleState, int64, error) {
	all, rev, err := s.Modules(ctx, t)
	if err != nil {
		return ModuleState{}, 0, err
	}
	for _, m := range all {
		if m.Key == key {
			return m, rev, nil
		}
	}
	return ModuleState{}, 0, &Error{Code: CodeModuleNotFound, Module: key, Message: "modul tidak dikenal"}
}

// History memuat event lifecycle modul (terbaru dulu).
func (s *Service) History(ctx context.Context, t tenancy.TenantID, key string, page, pageSize int) ([]store.Event, int64, error) {
	if _, err := s.knownKey(ctx, t, key); err != nil {
		return nil, 0, err
	}
	return s.d.Store.Events(ctx, t, key, page, pageSize)
}

func (s *Service) knownKey(ctx context.Context, t tenancy.TenantID, key string) (bool, error) {
	if _, ok := s.d.Registry.Entry(key); ok && key != registry.PlatformKey {
		return true, nil
	}
	list, err := s.d.Store.Installations(ctx, t)
	if err != nil {
		return false, err
	}
	for _, r := range list {
		if r.ModuleKey == key && key != registry.PlatformKey {
			return true, nil
		}
	}
	return false, &Error{Code: CodeModuleNotFound, Module: key, Message: "modul tidak dikenal"}
}

// Operation memuat satu operasi milik tenant; tidak ada atau milik tenant lain -> *Error operation_not_found.
func (s *Service) Operation(ctx context.Context, t tenancy.TenantID, id uuid.UUID) (store.Operation, error) {
	op, err := s.d.Store.Operation(ctx, t, id)
	if errors.Is(err, store.ErrNotFound) {
		return store.Operation{}, &Error{Code: CodeOperationNotFound, Message: "operasi tidak ditemukan"}
	}
	return op, err
}
