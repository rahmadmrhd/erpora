// Pembantu uji tata letak navbar (bukan bagian bundel): happy-dom tidak punya tata letak, jadi lebar elemen dan
// `ResizeObserver` ditiru. Lebar header diatur lewat `setNavWidth`; teks probe = panjang teks x lebar karakter;
// tombol aksi = panjang label x lebar karakter + padding; blok leading 36 px.

export interface LayoutMockOptions {
  navWidth: number;
  /** Lebar tiap karakter teks (px). */
  char?: number;
  /** Lebar padding tombol aksi di luar label (px). */
  actionPadding?: number;
}

export interface LayoutMock {
  setNavWidth(width: number): void;
  /** Memanggil semua pengamat ukuran (meniru perubahan ukuran elemen). */
  fireResize(): void;
  restore(): void;
}

type ObserverCallback = () => void;

export function installLayoutMock(options: LayoutMockOptions): LayoutMock {
  const char = options.char ?? 8;
  const actionPadding = options.actionPadding ?? 40;
  const state = { navWidth: options.navWidth };

  const widthOf = (el: Element): number => {
    if (el.matches('[data-slot="navbar"]')) return state.navWidth;
    if (el.hasAttribute("data-fit-probe")) return (el.textContent?.length ?? 0) * char;
    if (el.matches('[data-slot="navbar-leading"]')) return 36;
    const key = el.getAttribute("data-action-key");
    if (key !== null) {
      if (key === "__more") return 36;
      if (key.startsWith("node:")) return 60;
      const label = key.slice(key.indexOf("|") + 1);
      return label.length * char + actionPadding;
    }
    return 0;
  };

  const proto = Element.prototype;
  const originalRect = proto.getBoundingClientRect;
  proto.getBoundingClientRect = function (this: Element): DOMRect {
    const width = widthOf(this);
    return {
      width,
      height: 0,
      top: 0,
      left: 0,
      right: width,
      bottom: 0,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect;
  };

  const callbacks = new Set<ObserverCallback>();
  const originalObserver = globalThis.ResizeObserver;
  class MockResizeObserver {
    private readonly callback: ObserverCallback;
    constructor(callback: ResizeObserverCallback) {
      this.callback = () => callback([], this as unknown as ResizeObserver);
      callbacks.add(this.callback);
    }
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {
      callbacks.delete(this.callback);
    }
  }
  globalThis.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver;

  return {
    setNavWidth(width) {
      state.navWidth = width;
    },
    fireResize() {
      for (const cb of [...callbacks]) cb();
    },
    restore() {
      proto.getBoundingClientRect = originalRect;
      globalThis.ResizeObserver = originalObserver;
      callbacks.clear();
    },
  };
}
