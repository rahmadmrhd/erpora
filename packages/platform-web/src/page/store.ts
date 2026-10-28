import type { PageActionEntry, PageChrome } from "./types";

/** Keadaan awal dan keadaan setelah semua halaman unmount (P13); satu referensi tetap. */
export const EMPTY_CHROME: PageChrome = Object.freeze({
  actions: Object.freeze([] as PageActionEntry[]),
  scroll: true,
});

interface ActionSlot {
  signature: string;
  entries: readonly PageActionEntry[];
}

export interface Disposable<T> {
  update(value: T): void;
  dispose(): void;
}

export interface PageChromeStore {
  subscribe(listener: () => void): () => void;
  getSnapshot(): PageChrome;
  /** Mendaftarkan satu pemanggil `usePageActions`; urutan pendaftaran menentukan urutan tampil. */
  registerActions(slot: ActionSlot): Disposable<ActionSlot>;
  /** Mendaftarkan satu pemanggil `usePageLayout`. */
  registerLayout(scroll: boolean): Disposable<boolean>;
}

export function createPageChromeStore(): PageChromeStore {
  const listeners = new Set<() => void>();
  const actionSlots = new Map<symbol, ActionSlot>();
  const layoutSlots = new Map<symbol, boolean>();
  let snapshot: PageChrome = EMPTY_CHROME;

  const recompute = () => {
    const actions = [...actionSlots.values()].flatMap((s) => s.entries);
    const scroll = [...layoutSlots.values()].every((v) => v);
    const next: PageChrome = actions.length === 0 && scroll ? EMPTY_CHROME : { actions, scroll };
    if (next !== snapshot) {
      snapshot = next;
      for (const l of [...listeners]) l();
    }
  };

  return {
    subscribe(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    getSnapshot: () => snapshot,
    registerActions(slot) {
      const id = Symbol("page-actions");
      actionSlots.set(id, slot);
      recompute();
      return {
        update(value) {
          if (!actionSlots.has(id)) return;
          actionSlots.set(id, value);
          recompute();
        },
        dispose() {
          if (actionSlots.delete(id)) recompute();
        },
      };
    },
    registerLayout(scroll) {
      const id = Symbol("page-layout");
      layoutSlots.set(id, scroll);
      recompute();
      return {
        update(value) {
          if (!layoutSlots.has(id)) return;
          layoutSlots.set(id, value);
          recompute();
        },
        dispose() {
          if (layoutSlots.delete(id)) recompute();
        },
      };
    },
  };
}
