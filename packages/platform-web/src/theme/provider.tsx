import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useState,
} from "react";
import { readPref, writePref } from "../prefs";
import {
  applyTheme,
  parseThemePreference,
  type ResolvedTheme,
  resolveTheme,
  THEME_STORAGE_KEY,
  type ThemePreference,
} from "./theme";

const DARK_QUERY = "(prefers-color-scheme: dark)";

interface ThemeContextValue {
  preference: ThemePreference;
  resolved: ResolvedTheme;
  setPreference(p: ThemePreference): void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

function systemPrefersDark(): boolean {
  try {
    return window.matchMedia(DARK_QUERY).matches;
  } catch {
    return false;
  }
}

/**
 * Tema terang/gelap/sistem. Pilihan tersimpan di `localStorage` (`erpora.theme`); "sistem" mengikuti OS tanpa
 * reload; event `storage` menyinkronkan antar tab. Kelas dan `color-scheme` dipasang pada `<html>`.
 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<ThemePreference>(() =>
    parseThemePreference(readPref(THEME_STORAGE_KEY)),
  );
  const [systemDark, setSystemDark] = useState<boolean>(systemPrefersDark);

  useEffect(() => {
    let mql: MediaQueryList;
    try {
      mql = window.matchMedia(DARK_QUERY);
    } catch {
      return;
    }
    const onChange = (e: { matches: boolean }) => setSystemDark(e.matches);
    setSystemDark(mql.matches);
    mql.addEventListener("change", onChange);
    return () => mql.removeEventListener("change", onChange);
  }, []);

  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key !== null && e.key !== THEME_STORAGE_KEY) return;
      setPreferenceState(parseThemePreference(readPref(THEME_STORAGE_KEY)));
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, []);

  const resolved = resolveTheme(preference, systemDark);
  useLayoutEffect(() => {
    applyTheme(document.documentElement, resolved);
  }, [resolved]);

  const setPreference = useCallback((p: ThemePreference) => {
    const next = parseThemePreference(p);
    writePref(THEME_STORAGE_KEY, next);
    setPreferenceState(next);
  }, []);

  const value = useMemo(
    () => ({ preference, resolved, setPreference }),
    [preference, resolved, setPreference],
  );
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (ctx === null) throw new Error("useTheme harus dipakai di dalam ThemeProvider");
  return ctx;
}
