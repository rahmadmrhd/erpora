/** Satu notifikasi untuk panel notifikasi. Isi dapat memuat data bisnis/PII: hanya di cache memori, tidak dicatat. */
export interface AppNotification {
  id: string;
  title: string;
  message: string;
  /** ISO 8601 UTC. */
  createdAt: string;
  read: boolean;
  /** Path internal opsional; hanya dipakai bila lolos `safeInternalHref`. */
  href?: string;
}

/**
 * Sumber notifikasi, terpisah dari komponen UI sehingga backend (PRD Notifications) dapat dipasang tanpa mengubah
 * komponen (M3a R11.4, K-3).
 */
export interface NotificationSource {
  unreadCount(signal?: AbortSignal): Promise<number>;
  list(opts: { limit: number; signal?: AbortSignal }): Promise<readonly AppNotification[]>;
  markRead(id: string): Promise<void>;
  markAllRead(): Promise<void>;
  /** Opsional: dorongan perubahan dari sumber (mis. SSE di masa depan); mengembalikan fungsi berhenti. */
  subscribe?(onChange: () => void): () => void;
}

/** Pengguna saat ini. Nama dan email adalah PII (Rahasia): hanya cache memori, tidak di storage/log/URL. */
export interface CurrentUser {
  id: string;
  displayName: string;
  email: string | null;
  avatarUrl: string | null;
}

/** Sumber identitas, terpisah dari komponen; implementasi sungguhan datang bersama PRD-001 (M3a K-4). */
export interface CurrentUserSource {
  readonly capabilities: { readonly manageAccount: boolean; readonly signOut: boolean };
  /** `null` berarti anonim. */
  get(signal?: AbortSignal): Promise<CurrentUser | null>;
  /** Path internal halaman akun; `null` bila tidak tersedia. */
  manageAccountPath?(): string | null;
  signOut?(): Promise<void>;
}
