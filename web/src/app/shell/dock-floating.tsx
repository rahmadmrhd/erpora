import { useTranslation } from "@erpora/platform-web";
import { useCallback, useEffect, useRef, useState } from "react";
import { DockDeskList } from "./dock";

/** Penutupan tertunda setelah pointer meninggalkan pil (memberi toleransi gerak tipis keluar-masuk). */
const LEAVE_DELAY_MS = 250;

function isDockShortcut(event: KeyboardEvent): boolean {
  return (
    event.ctrlKey &&
    event.shiftKey &&
    !event.altKey &&
    !event.metaKey &&
    (event.code === "Slash" || event.key === "/" || event.key === "?")
  );
}

/**
 * Dock Floating (R4.4, R4.5): pil vertikal yang diparkir di luar layar dan meluncur dari tepi kiri (transisi 280 ms
 * `cubic-bezier(.4,0,.2,1)`, dimatikan `prefers-reduced-motion`). Pemicu: zona tepi kiri 12 px (hover) dan
 * `Ctrl+Shift+/`. Menutup setelah memilih Desk, `Escape`, pintasan lagi, atau fokus/pointer keluar. Saat tertutup
 * pil `inert` dan `aria-hidden` (tidak dapat difokus dan tersembunyi dari pembaca layar). Komponen ini dimuat malas.
 */
export function DockFloating() {
  const { t } = useTranslation("platform");
  const [open, setOpen] = useState(false);
  const pillRef = useRef<HTMLDivElement>(null);
  const focusRef = useRef<(() => void) | null>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const leaveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const openedByKeyboard = useRef(false);
  // Menu "+N" terbuka: isinya di portal (di luar DOM pil) sehingga fokus dan pointer ke sana bukan "keluar" dari pil.
  const menuOpen = useRef(false);
  const hovered = useRef(false);
  const onMenuOpenChange = useCallback((isOpen: boolean) => {
    if (isOpen) {
      menuOpen.current = true;
      return;
    }
    // Penutupan ditunda satu tick agar Escape yang menutup menu tidak sekaligus menutup pil (penangan window
    // berjalan setelah Radix).
    setTimeout(() => {
      menuOpen.current = false;
      // Menu ditutup dari luar pil (klik di luar): bila pointer tidak di atas pil, tutup seperti meninggalkannya.
      if (!hovered.current) scheduleLeave();
    }, 0);
  }, []);

  const clearLeave = () => {
    if (leaveTimer.current !== null) {
      clearTimeout(leaveTimer.current);
      leaveTimer.current = null;
    }
  };

  // Penutupan karena "meninggalkan" pil dibatalkan bila saat jeda habis fokus ada di dalam pil atau menu +N terbuka
  // (pengguna keyboard: Escape pada menu +N mengembalikan fokus ke tombol +N di dalam pil).
  function scheduleLeave() {
    clearLeave();
    leaveTimer.current = setTimeout(() => {
      leaveTimer.current = null;
      if (menuOpen.current || pillRef.current?.contains(document.activeElement)) return;
      hide(false);
    }, LEAVE_DELAY_MS);
  }

  const show = useCallback(() => {
    clearLeave();
    setOpen((was) => {
      if (!was) {
        const active = document.activeElement;
        returnFocusRef.current = active instanceof HTMLElement ? active : null;
      }
      return true;
    });
  }, []);

  const hide = useCallback((restoreFocus: boolean) => {
    clearLeave();
    menuOpen.current = false;
    setOpen(false);
    const target = returnFocusRef.current;
    returnFocusRef.current = null;
    if (restoreFocus && target?.isConnected) target.focus();
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (isDockShortcut(event)) {
        event.preventDefault();
        if (open) hide(true);
        else {
          openedByKeyboard.current = true;
          show();
        }
        return;
      }
      if (open && event.key === "Escape" && !menuOpen.current) hide(true);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open, show, hide]);

  useEffect(() => () => clearLeave(), []);

  // Fokus ke Desk saat ini setelah pil terbuka lewat keyboard; pembukaan lewat pointer tidak mencuri fokus.
  useEffect(() => {
    if (open && openedByKeyboard.current) focusRef.current?.();
    if (!open) openedByKeyboard.current = false;
  }, [open]);

  return (
    <>
      <div
        aria-hidden="true"
        data-slot="dock-hot-edge"
        className="fixed inset-y-0 left-0 z-30 w-3"
        onMouseEnter={show}
      />
      <div
        data-slot="dock-floating"
        data-state={open ? "open" : "closed"}
        aria-hidden={open ? undefined : true}
        inert={!open}
        onMouseEnter={() => {
          hovered.current = true;
          clearLeave();
        }}
        onMouseLeave={() => {
          hovered.current = false;
          if (menuOpen.current) return;
          if (pillRef.current?.contains(document.activeElement)) return;
          scheduleLeave();
        }}
        onBlur={(event) => {
          if (menuOpen.current) return;
          const next = event.relatedTarget;
          if (next instanceof Node && event.currentTarget.contains(next)) return;
          // Fokus keluar pil (ke elemen lain atau jendela): tutup tanpa merebut fokus kembali.
          if (open) hide(false);
        }}
        className="fixed top-1/2 left-2 z-40 -translate-x-[150%] -translate-y-1/2 transition-transform duration-280 ease-[cubic-bezier(.4,0,.2,1)] motion-reduce:transition-none data-[state=open]:translate-x-0"
      >
        <nav
          ref={pillRef}
          aria-label={t("dock.label")}
          className="flex max-h-[calc(100svh-2rem)] w-14 flex-col items-center gap-2 rounded-2xl border border-sidebar-border bg-sidebar py-2 text-sidebar-foreground shadow-lg"
        >
          <DockDeskList
            onSelected={() => hide(false)}
            onMenuOpenChange={onMenuOpenChange}
            initialFocusRef={focusRef}
          />
        </nav>
      </div>
    </>
  );
}
