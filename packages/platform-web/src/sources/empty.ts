import type { CurrentUserSource, NotificationSource } from "./types";

/** Sumber notifikasi kosong (backend belum ada): 0 belum dibaca, daftar kosong, aksi no-op. */
export const emptyNotificationSource: NotificationSource = Object.freeze({
  unreadCount: () => Promise.resolve(0),
  list: () => Promise.resolve([]),
  markRead: () => Promise.resolve(),
  markAllRead: () => Promise.resolve(),
});

/** Pengguna anonim: Kelola Akun dan Keluar tidak tersedia (identitas datang di PRD-001). */
export const anonymousUserSource: CurrentUserSource = Object.freeze({
  capabilities: Object.freeze({ manageAccount: false, signOut: false }),
  get: () => Promise.resolve(null),
});

/** Interval polling jumlah belum dibaca: 60 detik, dan tidak sama sekali untuk sumber kosong. */
export function notificationPollInterval(source: NotificationSource): number | false {
  return source === emptyNotificationSource ? false : 60_000;
}
