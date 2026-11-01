import { Component, type ReactNode } from "react";

// Galat yang ditangkap batas lokal shell ditandai di `getDerivedStateFromError` (fase render, SEBELUM React memanggil
// `onCaughtError` root), sehingga `handleCaughtError` tidak mencetak ulang objek galat beserta stack-nya.
const handled = new WeakSet<object>();
const logged = new WeakSet<object>();

/** Apakah galat ini sudah ditangani dan dicatat oleh batas lokal shell (Navbar, pengumuman rute). */
export function isHandledByShellBoundary(error: unknown): boolean {
  return typeof error === "object" && error !== null && handled.has(error);
}

interface Props {
  /** Dipakai di catatan konsol (mis. "navbar"). */
  label: string;
  /** Tampilan saat galat; fungsi menerima `retry` (menghapus keadaan galat dan memanggil `onRetry`). */
  fallback: ReactNode | ((retry: () => void) => ReactNode);
  /** Dipanggil sebelum anak dirender ulang oleh `retry` (mis. mereset query yang galat). */
  onRetry?: () => void;
  /** Berubahnya nilai ini (mis. URL) mengembalikan anak setelah galat. */
  resetKey: string;
  children: ReactNode;
}

/**
 * Batas galat lokal untuk bagian shell (Navbar, pengumuman rute): menampilkan `fallback`, mencatat SATU baris konsol per
 * objek galat (hanya nama galat; tanpa pesan, stack, atau PII), dan menandai galat agar tidak dicetak ulang root.
 */
export class LocalBoundary extends Component<Props, { failed: boolean }> {
  override state = { failed: false };

  static getDerivedStateFromError(error: unknown): { failed: boolean } {
    if (typeof error === "object" && error !== null) handled.add(error);
    return { failed: true };
  }

  override componentDidCatch(error: unknown): void {
    if (typeof error === "object" && error !== null) {
      if (logged.has(error)) return;
      logged.add(error);
    }
    const name = error instanceof Error ? error.name : "Error";
    console.error(`[erpora] ${this.props.label} gagal dirender: ${name}`);
  }

  override componentDidUpdate(prev: Props): void {
    if (this.state.failed && prev.resetKey !== this.props.resetKey) {
      // Berpindah halaman = percobaan ulang otomatis (mis. query galat dan chunk malas dimuat ulang).
      this.props.onRetry?.();
      this.setState({ failed: false });
    }
  }

  retry = (): void => {
    this.props.onRetry?.();
    this.setState({ failed: false });
  };

  override render(): ReactNode {
    if (!this.state.failed) return this.props.children;
    const { fallback } = this.props;
    return typeof fallback === "function" ? fallback(this.retry) : fallback;
  }
}
