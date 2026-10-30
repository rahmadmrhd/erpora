import { useTenantId } from "../bootstrap";

/**
 * Query key berawalan tenant: selalu `[tenantId, ...rest]` (PRD-000 FR-08/FR-21, M3a R7.1). Satu-satunya key tanpa
 * awalan tenant adalah `BOOTSTRAP_KEY` (sumber `tenantId`). `tenantId` kosong ditolak agar key tanpa scope tidak
 * terbentuk diam-diam.
 */
export function scopedKey(tenantId: string, ...rest: readonly unknown[]): readonly unknown[] {
  if (tenantId === "") throw new Error("scopedKey: tenantId tidak boleh kosong");
  return [tenantId, ...rest];
}

/** `scopedKey` dengan `tenantId` dari bootstrap; hanya di dalam `BootstrapProvider`. */
export function useScopedKey(...rest: readonly unknown[]): readonly unknown[] {
  return scopedKey(useTenantId(), ...rest);
}
