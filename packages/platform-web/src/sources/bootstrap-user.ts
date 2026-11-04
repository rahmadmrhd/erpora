import type { Bootstrap } from "../bootstrap/types";
import { safeInternalHref } from "./href";
import type { CurrentUserSource } from "./types";

export interface BootstrapUserSourceOptions {
  /** `bootstrap.user` (FR-106); `undefined` = tidak ada pengguna (anonim). */
  user: Bootstrap["user"];
  signOut(): Promise<void>;
  /** Path internal halaman akun (mis. ubah sandi); nilai yang bukan path internal aman diabaikan. */
  manageAccountPath?: string | null;
}

/**
 * Sumber pengguna sungguhan dari `bootstrap.user` (menggantikan `anonymousUserSource` sejak C1.14). Nama dan email
 * adalah PII: hanya hidup di cache memori Query. Avatar belum ada di kontrak (`null`).
 */
export function createBootstrapUserSource(opts: BootstrapUserSourceOptions): CurrentUserSource {
  const path = safeInternalHref(opts.manageAccountPath);
  const { user } = opts;
  return {
    capabilities: { manageAccount: true, signOut: true },
    defaultDesk: null,
    get: () =>
      Promise.resolve(
        user
          ? { id: user.id, displayName: user.displayName, email: user.email, avatarUrl: null }
          : null,
      ),
    manageAccountPath: () => path,
    signOut: opts.signOut,
  };
}
