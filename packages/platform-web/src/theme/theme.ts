export type ThemePreference = "light" | "dark" | "system";
export type ResolvedTheme = "light" | "dark";

/** Kunci `localStorage`; harus sama dengan yang dibaca `web/src/theme-init.ts` (diuji). */
export const THEME_STORAGE_KEY = "erpora.theme";

/** Total: nilai apa pun yang bukan "light"/"dark" menjadi "system". */
export function parseThemePreference(raw: unknown): ThemePreference {
  return raw === "light" || raw === "dark" ? raw : "system";
}

export function resolveTheme(pref: ThemePreference, systemDark: boolean): ResolvedTheme {
  if (pref === "system") return systemDark ? "dark" : "light";
  return pref;
}

/** Memasang tepat satu kelas tema (`light` atau `dark`) dan `color-scheme` pada elemen akar. */
export function applyTheme(root: HTMLElement, resolved: ResolvedTheme): void {
  root.classList.remove(resolved === "dark" ? "light" : "dark");
  root.classList.add(resolved);
  root.style.colorScheme = resolved;
}
