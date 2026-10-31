// Pembantu uji (bukan bagian bundel): `window.matchMedia` palsu yang menilai `(max-width: Npx)` terhadap lebar yang
// dapat diubah dan memancarkan event `change` ke pendengar yang terdaftar.

type Listener = () => void;
const original = typeof window === "undefined" ? undefined : window.matchMedia;

export function installMatchMedia(initialWidth: number) {
  let width = initialWidth;
  Object.defineProperty(window, "innerWidth", { value: width, configurable: true, writable: true });
  const entries: { query: string; listeners: Set<Listener> }[] = [];
  const matches = (query: string) => {
    const m = /\(max-width:\s*(\d+)px\)/.exec(query);
    return m ? width <= Number(m[1]) : false;
  };
  window.matchMedia = ((query: string) => {
    const entry = { query, listeners: new Set<Listener>() };
    entries.push(entry);
    return {
      get matches() {
        return matches(query);
      },
      media: query,
      addEventListener: (_: string, l: Listener) => entry.listeners.add(l),
      removeEventListener: (_: string, l: Listener) => entry.listeners.delete(l),
      addListener: (l: Listener) => entry.listeners.add(l),
      removeListener: (l: Listener) => entry.listeners.delete(l),
      onchange: null,
      dispatchEvent: () => true,
    };
  }) as unknown as typeof window.matchMedia;
  return {
    setWidth(next: number) {
      width = next;
      Object.defineProperty(window, "innerWidth", {
        value: next,
        configurable: true,
        writable: true,
      });
      for (const e of entries) for (const l of [...e.listeners]) l();
    },
  };
}

export function restoreMatchMedia() {
  if (original) window.matchMedia = original;
  Object.defineProperty(window, "innerWidth", { value: 1024, configurable: true, writable: true });
}
