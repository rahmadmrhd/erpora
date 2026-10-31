import {
  isPageActionNode,
  type PageAction,
  type PageActionEntry,
  type PageActionNode,
  useTranslation,
} from "@erpora/platform-web";
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  LoadingButton,
  resolveIcon,
} from "@erpora/ui";
import { useLayoutEffect, useRef, useState } from "react";

/** Kunci pengukuran tombol "⋯" (selain kunci aksi). */
export const MORE_ACTION_KEY = "__more";

/** Kunci stabil pengukuran: berubah bila id atau label (teks tampil) aksi berubah. */
export function actionKey(a: PageAction | PageActionNode): string {
  return "node" in a ? `node:${a.id}` : `${a.id}|${a.label}`;
}

export interface SplitActions {
  /** Aksi utama: yang `pinned`, atau bila tidak ada yang pinned, aksi pertama. Tidak pernah dilipat. */
  primary: PageAction[];
  /** Aksi sekunder berurutan deklarasi; yang terakhir masuk menu "⋯" lebih dulu. */
  secondary: PageAction[];
  /** Simpul kustom: hanya tampil di navbar lebar dan tidak dilipat. */
  nodes: PageActionNode[];
}

/** Membagi aksi halaman menjadi aksi utama, sekunder, dan simpul kustom (R8.2, R8.3). */
export function splitPageActions(actions: readonly PageActionEntry[]): SplitActions {
  const nodes = actions.filter(isPageActionNode);
  const plain = actions.filter((a): a is PageAction => !isPageActionNode(a));
  const primary = plain.some((a) => a.pinned) ? plain.filter((a) => a.pinned) : plain.slice(0, 1);
  return { primary, secondary: plain.filter((a) => !primary.includes(a)), nodes };
}

function ActionIcon({ name }: { name?: string }) {
  const C = resolveIcon(name);
  return C ? <C aria-hidden="true" /> : null;
}

/** Kontrak `PageAction.variant` memakai "primary" (aksi utama halaman); pada Button shadcn namanya "default". */
function buttonVariant(
  variant: PageAction["variant"],
): "default" | "outline" | "ghost" | "destructive" {
  return variant === undefined ? "outline" : variant === "primary" ? "default" : variant;
}

export interface PageActionsViewProps {
  split: SplitActions;
  /** Jumlah aksi sekunder (dari belakang) yang masuk menu "⋯". */
  collapsed: number;
  /** Navbar sempit (< 480 px): aksi utama berupa tombol ikon, simpul kustom disembunyikan. */
  narrow: boolean;
  /** Dilaporkan setiap tata letak: lebar alami (px) tiap tombol yang tampil, menurut `actionKey`. */
  onMeasure?: (widths: Record<string, number>) => void;
}

/**
 * Aksi halaman di kanan navbar (R8). Urutan tampil: simpul kustom, aksi sekunder yang masih muat, menu "⋯" (aksi
 * sekunder yang terlipat), lalu aksi utama di paling kanan. Aksi utama tidak pernah hilang; di navbar sempit
 * menjadi tombol ikon dengan nama aksesibel dan semua aksi sekunder masuk menu "⋯".
 */
export function PageActionsView({ split, collapsed, narrow, onMeasure }: PageActionsViewProps) {
  const { t } = useTranslation("platform");
  const [el, setEl] = useState<HTMLDivElement | null>(null);
  const folded = narrow ? split.secondary.length : collapsed;
  const inline = split.secondary.slice(0, split.secondary.length - folded);
  const menu = split.secondary.slice(split.secondary.length - folded);

  // Pengukuran lebar alami untuk perencana tata letak: setiap render dan setiap perubahan ukuran blok ini.
  const measureRef = useRef<() => void>(() => {});
  measureRef.current = () => {
    if (el === null || onMeasure === undefined) return;
    const widths: Record<string, number> = {};
    for (const child of el.querySelectorAll<HTMLElement>("[data-action-key]")) {
      const key = child.dataset.actionKey;
      if (key !== undefined) widths[key] = child.getBoundingClientRect().width;
    }
    onMeasure(widths);
  };
  useLayoutEffect(() => {
    measureRef.current();
  });
  useLayoutEffect(() => {
    if (el === null) return;
    const observer = new ResizeObserver(() => measureRef.current());
    observer.observe(el);
    return () => observer.disconnect();
  }, [el]);

  if (split.primary.length === 0 && split.secondary.length === 0 && split.nodes.length === 0) {
    return null;
  }

  return (
    <div
      ref={setEl}
      data-slot="navbar-actions"
      className="ml-auto flex shrink-0 items-center gap-2"
    >
      {narrow
        ? null
        : split.nodes.map((n) => (
            <div key={n.id} data-action-key={actionKey(n)}>
              {n.node}
            </div>
          ))}
      {inline.map((a) => (
        <LoadingButton
          key={a.id}
          type="button"
          data-action-key={actionKey(a)}
          variant={buttonVariant(a.variant)}
          loading={a.loading}
          disabled={a.disabled}
          onClick={a.onSelect}
        >
          <ActionIcon name={a.icon} />
          {a.label}
        </LoadingButton>
      ))}
      {menu.length > 0 ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              type="button"
              size="icon"
              variant="ghost"
              data-action-key={MORE_ACTION_KEY}
              aria-label={t("pageActions.more")}
            >
              <ActionIcon name="ellipsis" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {menu.map((a) => (
              <DropdownMenuItem
                key={a.id}
                disabled={a.disabled}
                variant={a.variant === "destructive" ? "destructive" : "default"}
                onSelect={a.onSelect}
              >
                <ActionIcon name={a.icon} />
                {a.label}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
      {split.primary.map((a) =>
        narrow ? (
          <LoadingButton
            key={a.id}
            type="button"
            size="icon"
            variant={buttonVariant(a.variant)}
            aria-label={a.label}
            loading={a.loading}
            disabled={a.disabled}
            onClick={a.onSelect}
          >
            <ActionIcon name={a.icon} />
          </LoadingButton>
        ) : (
          <LoadingButton
            key={a.id}
            type="button"
            data-action-key={actionKey(a)}
            variant={buttonVariant(a.variant)}
            loading={a.loading}
            disabled={a.disabled}
            onClick={a.onSelect}
          >
            <ActionIcon name={a.icon} />
            {a.label}
          </LoadingButton>
        ),
      )}
    </div>
  );
}
