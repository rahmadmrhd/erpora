import { useNavigate } from "@erpora/module-sdk";
import {
  type Desk,
  type DeskMenuGroup,
  type DeskMenuItem,
  Message,
  type MessageRef,
  useCurrentDesk,
  useDesks,
  useMessage,
  useTranslation,
} from "@erpora/platform-web";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Kbd,
  KbdGroup,
  resolveIcon,
} from "@erpora/ui";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@erpora/ui/command";
import { Fragment, type ReactNode, useCallback, useEffect, useMemo, useState } from "react";
import { deskDashboardPath, useDeskNavigation } from "./desk-navigation";

/** Satu hasil palet: halaman menu sebuah Desk, atau Desk itu sendiri (membuka Dashboard-nya). */
interface Entry {
  value: string;
  kind: "page" | "desk";
  deskId: string;
  label: MessageRef;
  /** Label grup menu induk (halaman di dalam grup). */
  parent?: MessageRef;
  deskLabel: MessageRef;
  to: string;
  icon?: string;
}

function leaves(desk: Desk): { item: DeskMenuItem; group?: DeskMenuGroup }[] {
  return desk.menus.flatMap((m) =>
    m.type === "item" ? [{ item: m }] : m.children.map((item) => ({ item, group: m })),
  );
}

function pageEntries(desk: Desk): Entry[] {
  return leaves(desk).map(({ item, group }) => ({
    value: `page:${desk.id}:${item.id}`,
    kind: "page",
    deskId: desk.id,
    label: item.label,
    ...(group ? { parent: group.label } : {}),
    deskLabel: desk.label,
    to: item.to,
    ...(item.icon !== undefined ? { icon: item.icon } : {}),
  }));
}

const normalize = (s: string): string => s.normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();

function tokensOf(query: string): string[] {
  return normalize(query).split(/\s+/).filter(Boolean);
}

/**
 * Pencocokan lokal tanpa kueri backend: semua kata kueri harus muncul (urutan bebas, tanpa peka huruf besar-kecil dan
 * aksen) pada teks item (`keywords`: label, grup induk, nama Desk). Awalan label diutamakan.
 */
export function paletteFilter(_value: string, search: string, keywords?: string[]): number {
  const tokens = tokensOf(search);
  if (tokens.length === 0) return 1;
  const texts = (keywords ?? []).map(normalize);
  const haystack = texts.join(" ");
  if (!tokens.every((t) => haystack.includes(t))) return 0;
  return texts[0]?.startsWith(tokens[0] ?? "") ? 2 : 1;
}

/** Menyorot bagian teks yang cocok dengan kata kueri (`<mark>`, tanpa HTML mentah). */
function Highlight({ text, query }: { text: string; query: string }): ReactNode {
  const tokens = query.trim().split(/\s+/).filter(Boolean);
  if (tokens.length === 0) return text;
  const pattern = new RegExp(
    `(${tokens
      .map((t) => t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"))
      .sort((a, b) => b.length - a.length)
      .join("|")})`,
    "gi",
  );
  const parts = text.split(pattern);
  return parts.map((part, i) =>
    i % 2 === 1 ? (
      <mark key={i} className="rounded-xs bg-primary/15 text-foreground">
        {part}
      </mark>
    ) : (
      <Fragment key={i}>{part}</Fragment>
    ),
  );
}

function EntryItem({
  entry,
  query,
  onChoose,
}: {
  entry: Entry;
  query: string;
  onChoose: (entry: Entry) => void;
}) {
  const label = useMessage(entry.label);
  const parent = useMessage(entry.parent ?? entry.label);
  const deskLabel = useMessage(entry.deskLabel);
  const Icon = resolveIcon(entry.icon);
  const parentText = entry.parent === undefined ? "" : parent;
  return (
    <CommandItem
      value={entry.value}
      keywords={[label, parentText, deskLabel].filter(Boolean)}
      onSelect={() => onChoose(entry)}
    >
      {Icon ? <Icon aria-hidden="true" /> : null}
      <span className="truncate">
        <Highlight text={label} query={query} />
      </span>
      {parentText !== "" ? (
        <span className="truncate text-xs text-muted-foreground">
          <Highlight text={parentText} query={query} />
        </span>
      ) : null}
    </CommandItem>
  );
}

export interface CommandPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Palet perintah (Awesomebar, R18): `Command` shadcn dalam `Dialog`, dimuat malas oleh pemanggil. Sumber lokal: menu
 * Desk saat ini dan semua Desk yang diizinkan; dengan kueri, halaman Desk lain ikut dicari (dikelompokkan menurut
 * Desk). Tanpa kueri backend (pencarian dokumen adalah M3c). Memilih hasil lintas Desk menetapkan Desk lebih dulu
 * (`setCurrentDesk`) lalu menavigasi (R18.4).
 */
export function CommandPalette({ open, onOpenChange }: CommandPaletteProps) {
  const { t } = useTranslation("platform");
  const { desks } = useDesks();
  const { deskId: currentDeskId, setCurrentDesk } = useCurrentDesk();
  const { openDesk } = useDeskNavigation();
  const navigate = useNavigate();
  const [query, setQuery] = useState("");

  useEffect(() => {
    if (!open) setQuery("");
  }, [open]);

  const currentDesk = desks.find((d) => d.id === currentDeskId) ?? null;
  const searching = query.trim() !== "";
  const otherDesks = useMemo(
    () => desks.filter((d) => d.id !== currentDeskId),
    [desks, currentDeskId],
  );

  const choose = useCallback(
    (entry: Entry) => {
      onOpenChange(false);
      const desk = desks.find((d) => d.id === entry.deskId);
      if (desk === undefined) return;
      if (entry.kind === "desk") {
        openDesk(desk);
        return;
      }
      if (entry.deskId !== currentDeskId) setCurrentDesk(entry.deskId);
      void navigate({ to: entry.to });
    },
    [desks, currentDeskId, setCurrentDesk, openDesk, navigate, onOpenChange],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="gap-0 overflow-hidden p-0 sm:max-w-xl">
        <DialogHeader className="sr-only">
          <DialogTitle>{t("palette.title")}</DialogTitle>
          <DialogDescription>{t("palette.description")}</DialogDescription>
        </DialogHeader>
        <Command
          loop
          label={t("palette.placeholder")}
          filter={paletteFilter}
          className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group]]:px-2 [&_[cmdk-input-wrapper]_svg]:h-5 [&_[cmdk-input-wrapper]_svg]:w-5 [&_[cmdk-input]]:h-12 [&_[cmdk-item]]:px-2 [&_[cmdk-item]]:py-2.5"
        >
          <CommandInput
            value={query}
            onValueChange={setQuery}
            placeholder={t("palette.placeholder")}
          />
          <CommandList>
            <CommandEmpty>{t("palette.empty")}</CommandEmpty>
            {currentDesk !== null ? (
              <CommandGroup heading={t("palette.groups.pages")}>
                {pageEntries(currentDesk).map((entry) => (
                  <EntryItem key={entry.value} entry={entry} query={query} onChoose={choose} />
                ))}
              </CommandGroup>
            ) : null}
            <CommandGroup heading={t("palette.groups.desks")}>
              {desks.map((desk) => (
                <EntryItem
                  key={desk.id}
                  query={query}
                  onChoose={choose}
                  entry={{
                    value: `desk:${desk.id}`,
                    kind: "desk",
                    deskId: desk.id,
                    label: desk.label,
                    deskLabel: desk.label,
                    to: deskDashboardPath(desk),
                    ...(desk.icon !== undefined ? { icon: desk.icon } : {}),
                  }}
                />
              ))}
            </CommandGroup>
            {searching
              ? otherDesks.map((desk) => (
                  <CommandGroup key={desk.id} heading={<Message message={desk.label} />}>
                    {pageEntries(desk).map((entry) => (
                      <EntryItem key={entry.value} entry={entry} query={query} onChoose={choose} />
                    ))}
                  </CommandGroup>
                ))
              : null}
          </CommandList>
        </Command>
        <div className="flex items-center gap-4 border-t px-3 py-2 text-xs text-muted-foreground">
          <span className="flex items-center gap-1.5">
            <KbdGroup>
              <Kbd>↑</Kbd>
              <Kbd>↓</Kbd>
            </KbdGroup>
            {t("palette.hints.navigate")}
          </span>
          <span className="flex items-center gap-1.5">
            <Kbd>↵</Kbd>
            {t("palette.hints.open")}
          </span>
          <span className="flex items-center gap-1.5">
            <Kbd>Esc</Kbd>
            {t("palette.hints.close")}
          </span>
        </div>
      </DialogContent>
    </Dialog>
  );
}
