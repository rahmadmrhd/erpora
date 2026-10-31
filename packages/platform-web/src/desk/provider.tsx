import type { PlatformPaths, TypedClient } from "@erpora/platform-contracts";
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react";
import { useDefaultDesk } from "../sources/current-user";
import { useDesksQuery } from "./api";
import { resolveDesk } from "./resolver";
import { isValidDeskId, readRememberedDesk, readTabDesk, writeDeskChoice } from "./storage";
import type { Desk, DeskList } from "./types";

/**
 * Penanda state navigasi (bukan URL, R10.5): `navigate({ to: "/", state: SHOW_DESKS_STATE })` membuat `/` menampilkan
 * halaman Desks dengan melewati penentuan Desk; muat ulang menghapusnya dari konteks efektif (penentuan biasa).
 */
export const SHOW_DESKS_STATE = Object.freeze({ showDesks: true });

/** `true` hanya untuk objek dengan `showDesks === true` (state riwayat tidak dipercaya). */
export function isShowDesksState(state: unknown): boolean {
  return (
    typeof state === "object" &&
    state !== null &&
    (state as { showDesks?: unknown }).showDesks === true
  );
}

function subscribePopState(onChange: () => void): () => void {
  window.addEventListener("popstate", onChange);
  return () => window.removeEventListener("popstate", onChange);
}

function readHistoryState(): unknown {
  try {
    return window.history.state;
  } catch {
    return null;
  }
}

export interface DeskContextValue {
  status: "loading" | "ready" | "error";
  desks: readonly Desk[];
  /** ETag/revisi daftar terakhir; `null` sebelum siap. */
  revision: string | null;
  /** Desk saat ini (id); `null` bila belum ada kandidat valid (halaman Desks) atau daftar belum siap. */
  currentDeskId: string | null;
  setCurrentDesk(deskId: string): void;
  /** `default_desk` pengguna (`null` sampai PRD-001). */
  defaultDesk: string | null;
  showDesks: boolean;
  refetch(): void;
}

const DeskContext = createContext<DeskContextValue | null>(null);
const NO_DESKS: readonly Desk[] = Object.freeze([]);

export interface DeskProviderProps {
  client: TypedClient<PlatformPaths>;
  /**
   * State riwayat lokasi saat ini (mis. `location.state` dari router) untuk penanda `showDesks`. Tanpa prop dibaca
   * dari `window.history.state` saat render dan pada `popstate`.
   */
  historyState?: unknown;
  children: ReactNode;
}

/**
 * Memuat daftar Desk dari server dan menyimpan Desk saat ini (state klien, bukan URL; tiap tab boleh berbeda,
 * R9.5). Desk saat ini = pilihan eksplisit (`setCurrentDesk`) bila masih diizinkan server; selain itu hasil
 * `resolveDesk` tahap 1-3 (memori tab, `localStorage`, `default_desk`). Dipasang di dalam `BootstrapProvider`,
 * `QueryClientProvider`, dan `CurrentUserSourceProvider`.
 */
export function DeskProvider({ client, historyState, children }: DeskProviderProps) {
  const query = useDesksQuery(client);
  const defaultDesk = useDefaultDesk();
  const desks = query.data?.desks ?? NO_DESKS;
  const status: DeskContextValue["status"] = query.data
    ? "ready"
    : query.isError
      ? "error"
      : "loading";

  const [chosen, setChosen] = useState<string | null>(null);

  const fallback = useMemo(
    () =>
      status === "ready"
        ? resolveDesk({
            desks,
            tabDesk: readTabDesk(),
            rememberedDesk: readRememberedDesk(),
            defaultDesk,
          }).deskId
        : null,
    [status, desks, defaultDesk],
  );
  const currentDeskId =
    status === "ready" && chosen !== null && desks.some((d) => d.id === chosen) ? chosen : fallback;

  const setCurrentDesk = useCallback(
    (deskId: string) => {
      if (!isValidDeskId(deskId) || !desks.some((d) => d.id === deskId)) return;
      writeDeskChoice(deskId);
      setChosen(deskId);
    },
    [desks],
  );

  const popState = useSyncExternalStore(subscribePopState, readHistoryState, () => null);
  const showDesks = isShowDesksState(historyState === undefined ? popState : historyState);
  const refetch = useCallback(() => {
    void query.refetch();
  }, [query.refetch]);

  const value = useMemo<DeskContextValue>(
    () => ({
      status,
      desks,
      revision: query.data?.revision ?? null,
      currentDeskId,
      setCurrentDesk,
      defaultDesk,
      showDesks,
      refetch,
    }),
    [
      status,
      desks,
      query.data?.revision,
      currentDeskId,
      setCurrentDesk,
      defaultDesk,
      showDesks,
      refetch,
    ],
  );
  return <DeskContext.Provider value={value}>{children}</DeskContext.Provider>;
}

/** Konteks Desk lengkap; hanya di dalam `DeskProvider`. */
export function useDeskContext(): DeskContextValue {
  const value = useContext(DeskContext);
  if (value === null) throw new Error("useDeskContext harus dipakai di dalam DeskProvider");
  return value;
}

/** Daftar Desk yang diizinkan server beserta statusnya. */
export function useDesks(): {
  status: DeskContextValue["status"];
  desks: readonly Desk[];
  revision: DeskList["revision"] | null;
  refetch(): void;
} {
  const { status, desks, revision, refetch } = useDeskContext();
  return { status, desks, revision, refetch };
}

/** Desk saat ini (`desk` = objeknya) dan penggantinya. */
export function useCurrentDesk(): {
  deskId: string | null;
  desk: Desk | null;
  setCurrentDesk(deskId: string): void;
} {
  const { currentDeskId, desks, setCurrentDesk } = useDeskContext();
  const desk = useMemo(
    () => desks.find((d) => d.id === currentDeskId) ?? null,
    [desks, currentDeskId],
  );
  return { deskId: currentDeskId, desk, setCurrentDesk };
}

/** Penanda "tampilkan halaman Desks" dari state navigasi (R10.5). */
export function useShowDesks(): boolean {
  return useDeskContext().showDesks;
}
