package lifecycle

import (
	"context"

	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/tenancy"
)

// EnsureRequired meng-install dan meng-enable setiap modul required beserta dependensinya, urut topologis
// (keputusan 17). Idempoten: modul yang sudah enabled menghasilkan hasil noop. Berhenti pada error atau
// hasil failed; pemanggil wajib memeriksa Result.Status.
func (s *Service) EnsureRequired(ctx context.Context, t tenancy.TenantID, actor string) ([]Result, error) {
	var out []Result
	for _, key := range s.d.Registry.Graph().Order() {
		e, ok := s.d.Registry.Entry(key)
		if !ok || key == registry.PlatformKey || !e.Manifest.Required {
			continue
		}
		res, err := s.Execute(ctx, Request{Tenant: t, Op: OpInstall, Key: key, WithDeps: true, Enable: true, Actor: actor})
		out = append(out, res...)
		if err != nil {
			return out, err
		}
		for _, r := range res {
			if r.Status == "failed" {
				return out, nil
			}
		}
	}
	return out, nil
}
