import { Link } from "@erpora/module-sdk";
import {
  type Desk,
  Message,
  useCurrentDesk,
  useMessage,
  useTranslation,
} from "@erpora/platform-web";
import {
  Button,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@erpora/ui";
import { useRouterState } from "@tanstack/react-router";
import { type ReactElement, useEffect, useRef, useState } from "react";
import { DeskMenuTree } from "./desk-panel-menu";
import { NavIcon } from "./nav-icon";
import { ShellBoundary } from "./shell-boundary";

/** Pengganti isi popover bila katalog atau chunk menu gagal dimuat: pesan kecil dan tombol coba lagi. */
function DeskMenuError({ retry }: { retry: () => void }) {
  const { t } = useTranslation("platform");
  return (
    <div className="flex flex-col gap-2 px-2 py-1">
      <p role="alert" className="text-sm text-muted-foreground">
        {t("panel.loadError")}
      </p>
      <Button type="button" variant="outline" size="sm" onClick={retry}>
        {t("shell.retry")}
      </Button>
    </div>
  );
}

/**
 * Isi popover: label Desk dan pohon menunya bergaya item sidebar. Dirender hanya saat popover terbuka sehingga
 * langganan lokasi router tidak membuat ikon Dock merender ulang pada tiap navigasi.
 */
function DeskMenuContent({ desk, onSelect }: { desk: Desk; onSelect: () => void }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  return (
    <>
      <p className="px-2 pb-1 text-xs font-medium text-sidebar-foreground/70">
        <Message message={desk.label} />
      </p>
      <DeskMenuTree desk={desk} pathname={pathname} tooltip={false} onSelect={onSelect} />
    </>
  );
}

/**
 * Popover menu sebuah Desk di sisi kanan ikon Desk Dock Pinned (K8). Klik ikon (juga ikon Desk yang sedang aktif)
 * membuka popover, bukan langsung berpindah; memilih item menavigasi ke item itu dan menetapkan Desk ini sebagai
 * Desk saat ini. `children` adalah tombol ikon pemicunya. Dimuat malas oleh `Dock`.
 */
export function DeskMenuPopover({
  desk,
  defaultOpen = false,
  onOpenChange,
  onSelected,
  children,
}: {
  desk: Desk;
  /** Terbuka sejak dipasang (klik ikon yang terjadi saat chunk ini masih dimuat). */
  defaultOpen?: boolean;
  /** Dipanggil saat popover dibuka atau ditutup (pil Floating memakainya agar tetap terbuka). */
  onOpenChange?: (open: boolean) => void;
  /** Dipanggil setelah sebuah item dipilih (pil Floating menutup diri). */
  onSelected?: () => void;
  children: ReactElement;
}) {
  const [open, setOpenState] = useState(defaultOpen);
  const setOpen = (next: boolean) => {
    setOpenState(next);
    onOpenChange?.(next);
  };
  // Ditutup lewat interaksi di luar: fokus mengikuti elemen yang diklik (tidak dikembalikan ke ikon pemicu), sehingga
  // pil Floating yang fokusnya lalu berada di luar ikut menutup.
  const dismissedOutside = useRef(false);
  // Dipasang sudah terbuka (klik saat chunk dimuat): beri tahu pemanggil sekali.
  useEffect(() => {
    if (defaultOpen) onOpenChange?.(true);
  }, []);
  const { setCurrentDesk } = useCurrentDesk();
  const label = useMessage(desk.label);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent
        side="right"
        align="start"
        sideOffset={8}
        aria-label={label}
        onInteractOutside={() => {
          dismissedOutside.current = true;
        }}
        onCloseAutoFocus={(event) => {
          if (dismissedOutside.current) event.preventDefault();
          dismissedOutside.current = false;
        }}
        className="max-h-[calc(100svh-1rem)] w-60 overflow-y-auto border-sidebar-border bg-sidebar p-2 text-sidebar-foreground"
      >
        <ShellBoundary label="menu Desk" fallback={(retry) => <DeskMenuError retry={retry} />}>
          <DeskMenuContent
            desk={desk}
            onSelect={() => {
              setCurrentDesk(desk.id);
              setOpen(false);
              onSelected?.();
            }}
          />
        </ShellBoundary>
      </PopoverContent>
    </Popover>
  );
}

/**
 * Item menu "+N" Dock Pinned untuk Desk yang ikonnya tidak muat: submenu berisi menu Desk itu (Dashboard, item, dan
 * isi grup di bawah label grupnya). Memilih item menavigasi ke item itu dan menetapkan Desk itu sebagai Desk saat ini.
 */
export function OverflowDeskSub({
  desk,
  onSelected,
}: {
  desk: Desk;
  /** Dipanggil setelah sebuah item dipilih. */
  onSelected?: () => void;
}) {
  const text = useMessage(desk.label);
  const { setCurrentDesk } = useCurrentDesk();
  const choose = () => {
    setCurrentDesk(desk.id);
    onSelected?.();
  };
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger>
        <NavIcon name={desk.icon} label={text} />
        {text}
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent>
        {desk.menus.map((m) =>
          m.type === "item" ? (
            <OverflowLink key={m.id} to={m.to} label={m.label} onSelect={choose} />
          ) : (
            <div key={m.id}>
              <DropdownMenuLabel className="text-xs text-muted-foreground">
                <Message message={m.label} />
              </DropdownMenuLabel>
              {m.children.map((c) => (
                <OverflowLink key={c.id} to={c.to} label={c.label} onSelect={choose} />
              ))}
            </div>
          ),
        )}
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}

function OverflowLink({
  to,
  label,
  onSelect,
}: {
  to: string;
  label: Desk["label"];
  onSelect: () => void;
}) {
  return (
    <DropdownMenuItem asChild>
      <Link to={to} onClick={onSelect}>
        <Message message={label} />
      </Link>
    </DropdownMenuItem>
  );
}
