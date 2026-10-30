import {
  createContext,
  type ReactNode,
  useContext,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { createPageChromeStore, EMPTY_CHROME, type PageChromeStore } from "./store";
import { isPageActionNode, type PageAction, type PageActionEntry, type PageChrome } from "./types";

// Tanpa provider (halaman diuji sendiri) store kosong yang tidak pernah berubah dipakai: tidak ada navbar, tidak melempar.
const inertStore = createPageChromeStore();
const StoreContext = createContext<PageChromeStore>(inertStore);

/** Dipasang shell: menyimpan aksi dan opsi tata letak yang dideklarasikan halaman aktif. */
export function PageChromeProvider({ children }: { children: ReactNode }) {
  const [store] = useState(createPageChromeStore);
  return <StoreContext.Provider value={store}>{children}</StoreContext.Provider>;
}

/** Aksi dan tata letak halaman aktif; dibaca Navbar dan area konten. */
export function usePageChrome(): PageChrome {
  const store = useContext(StoreContext);
  return useSyncExternalStore(store.subscribe, store.getSnapshot, () => EMPTY_CHROME);
}

// Bagian aksi yang memengaruhi tampilan (tanpa fungsi): perubahan closure `onSelect` saja tidak memicu pembaruan.
function signatureOf(entries: readonly PageActionEntry[]): string {
  return JSON.stringify(
    entries.map((e) =>
      isPageActionNode(e)
        ? [e.id, "node"]
        : [e.id, e.label, e.icon, e.variant, e.disabled, e.loading, e.pinned],
    ),
  );
}

function sameNodes(a: readonly PageActionEntry[], b: readonly PageActionEntry[]): boolean {
  return a.every((e, i) => {
    const o = b[i];
    return isPageActionNode(e) ? o !== undefined && isPageActionNode(o) && e.node === o.node : true;
  });
}

/**
 * Mendaftarkan aksi halaman selama komponen ter-mount (sinkron lewat `useLayoutEffect`) dan membersihkannya saat
 * unmount (P13). `onSelect` terbaru selalu dipakai tanpa memicu render ulang konsumen; beberapa pemanggil digabung
 * menurut urutan mount.
 */
export function usePageActions(actions: readonly PageActionEntry[]): void {
  const store = useContext(StoreContext);
  const latest = useRef(actions);
  latest.current = actions;
  const slotRef = useRef<{
    update(v: { signature: string; entries: readonly PageActionEntry[] }): void;
    dispose(): void;
  } | null>(null);
  const applied = useRef<{ signature: string; entries: readonly PageActionEntry[] } | null>(null);

  const build = (src: readonly PageActionEntry[]): readonly PageActionEntry[] =>
    src.map((e) =>
      isPageActionNode(e)
        ? e
        : ({
            ...e,
            // Pembungkus stabil: selalu memanggil `onSelect` dari render terbaru halaman.
            onSelect: () => {
              const current = latest.current.find((x) => x.id === e.id);
              if (current && !isPageActionNode(current)) (current as PageAction).onSelect();
            },
          } satisfies PageAction),
    );

  useLayoutEffect(() => {
    const first = { signature: signatureOf(latest.current), entries: build(latest.current) };
    applied.current = first;
    slotRef.current = store.registerActions(first);
    return () => {
      slotRef.current?.dispose();
      slotRef.current = null;
      applied.current = null;
    };
  }, [store]);

  // Setiap render: perbarui hanya bila bagian tampilan (atau simpul kustom) berubah.
  useLayoutEffect(() => {
    const slot = slotRef.current;
    const prev = applied.current;
    if (!slot || !prev) return;
    const signature = signatureOf(actions);
    if (signature === prev.signature && sameNodes(actions, prev.entries)) return;
    const next = { signature, entries: build(actions) };
    applied.current = next;
    slot.update(next);
  });
}

/** Padanan komponen dari `usePageActions`. */
export function PageActions({ actions }: { actions: readonly PageActionEntry[] }): null {
  usePageActions(actions);
  return null;
}

/** `scroll: false` mematikan gulir area konten selama halaman ter-mount (layout list-detail); bawaan `true`. */
export function usePageLayout(opts: { scroll?: boolean }): void {
  const store = useContext(StoreContext);
  const scroll = opts.scroll !== false;
  const slotRef = useRef<{ update(v: boolean): void; dispose(): void } | null>(null);
  const scrollRef = useRef(scroll);
  scrollRef.current = scroll;

  useLayoutEffect(() => {
    slotRef.current = store.registerLayout(scrollRef.current);
    return () => {
      slotRef.current?.dispose();
      slotRef.current = null;
    };
  }, [store]);

  useLayoutEffect(() => {
    slotRef.current?.update(scroll);
  }, [scroll]);
}
