import { initialsOf, resolveIcon } from "@erpora/ui";

/**
 * Ikon menu dari registri `@erpora/ui` (nama kebab-case). Nama tak dikenal atau kosong menjadi lingkaran berinisial
 * dari label (perilaku DS, M3a R9.8).
 */
export function NavIcon({ name, label }: { name?: string; label: string }) {
  const Icon = resolveIcon(name);
  if (Icon) return <Icon aria-hidden="true" />;
  return (
    <span
      aria-hidden="true"
      data-slot="nav-icon-fallback"
      className="flex size-4 shrink-0 items-center justify-center rounded-full bg-muted text-[9px] font-semibold text-muted-foreground"
    >
      {Array.from(initialsOf(label))[0]}
    </span>
  );
}
