// Tema dan token (M3a R2; K-13, K-14; P15): paritas blok gelap, token wajib, kontras dari nilai theme.css,
// dan larangan nilai palet langsung di komponen.
import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(join(here, "theme.css"), "utf8");

// ---- Pembaca CSS minimal ----

/** Isi blok `{...}` yang kurung buka-nya berada di `open` (menghitung kurung bersarang). */
function bodyAt(source: string, open: number): string {
  let depth = 0;
  for (let i = open; i < source.length; i++) {
    if (source[i] === "{") depth++;
    else if (source[i] === "}" && --depth === 0) return source.slice(open + 1, i);
  }
  throw new Error("kurung kurawal tidak seimbang");
}

/** Isi blok pertama yang selektornya cocok persis `selector` (di mana pun, termasuk di dalam @media). */
function block(source: string, selector: string): string {
  const re = new RegExp(`(^|[}\\s])${selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\s*\\{`, "m");
  const m = re.exec(source);
  if (!m) throw new Error(`blok ${selector} tidak ditemukan`);
  return bodyAt(source, m.index + m[0].length - 1);
}

function declarations(body: string): Map<string, string> {
  const out = new Map<string, string>();
  for (const m of body.matchAll(/--([\w-]+)\s*:\s*([^;]+);/g)) {
    out.set(m[1] as string, (m[2] as string).trim());
  }
  return out;
}

/** Bagian CSS di luar blok @media (prefers-color-scheme: dark) dan @theme/@layer: hanya :root dan .dark. */
const mediaStart = css.search(/@media \(prefers-color-scheme: dark\)\s*\{/);
const mediaBody = bodyAt(css, css.indexOf("{", mediaStart));
const cssWithoutMedia = css.slice(0, mediaStart) + css.slice(mediaStart + mediaBody.length + 2);

const light = declarations(block(cssWithoutMedia, ":root"));
const dark = declarations(block(cssWithoutMedia, ".dark"));
const systemDark = declarations(block(mediaBody, ":root:not(.light):not(.dark)"));

// ---- Warna ----

type Rgb = [number, number, number];

function oklchToLinear(L: number, C: number, hDeg: number): Rgb {
  const a = C * Math.cos((hDeg * Math.PI) / 180);
  const b = C * Math.sin((hDeg * Math.PI) / 180);
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  const clamp = (x: number): number => Math.max(0, Math.min(1, x));
  return [
    clamp(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s),
    clamp(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s),
    clamp(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s),
  ];
}

function srgbToLinear(c: number): number {
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

/** Mengurai oklch(L C h) (L boleh persen), #rgb/#rrggbb, atau rgb()/rgba() menjadi RGB linear. */
function parseColor(value: string): Rgb {
  const v = value.trim();
  const ok = /^oklch\(\s*([\d.]+)(%?)\s+([\d.]+)\s+([\d.]+)\s*\)$/.exec(v);
  if (ok) {
    const L = Number(ok[1]) / (ok[2] === "%" ? 100 : 1);
    return oklchToLinear(L, Number(ok[3]), Number(ok[4]));
  }
  const hex = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(v);
  if (hex) {
    let h = hex[1] as string;
    if (h.length === 3) h = [...h].map((c) => c + c).join("");
    const n = [0, 2, 4].map((i) => srgbToLinear(Number.parseInt(h.slice(i, i + 2), 16) / 255));
    return n as Rgb;
  }
  const rgb = /^rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)/.exec(v);
  if (rgb) {
    return [1, 2, 3].map((i) => srgbToLinear(Number(rgb[i]) / 255)) as Rgb;
  }
  throw new Error(`warna tidak dikenali: ${value}`);
}

function luminance([r, g, b]: Rgb): number {
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(fg: string, bg: string): number {
  const a = luminance(parseColor(fg));
  const b = luminance(parseColor(bg));
  const [hi, lo] = a >= b ? [a, b] : [b, a];
  return (hi + 0.05) / (lo + 0.05);
}

// ---- Pemindai palet langsung ----

const palette =
  "zinc|gray|slate|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose";
const colorUtility =
  "bg|text|border|ring|ring-offset|fill|stroke|from|via|to|outline|divide|decoration|shadow|accent|caret|placeholder";
const paletteClass = new RegExp(
  `(?:^|[\\s"'\`:\\[(])(?:${colorUtility})-(?:(?:${palette})-\\d{2,3}|white|black)(?:/\\d+)?(?=$|[\\s"'\`\\])])`,
);
const rawColor = /#[0-9a-fA-F]{3,8}\b|\b(?:rgb|rgba|hsl|hsla|oklch|oklab|color-mix)\(/;

/** Pelanggaran nilai warna langsung (kelas palet Tailwind, #hex, rgb()/oklch()) pada kode sumber komponen. */
export function paletteViolations(source: string): string[] {
  const found: string[] = [];
  for (const [i, line] of source.split("\n").entries()) {
    const p = paletteClass.exec(line);
    if (p) found.push(`baris ${i + 1}: kelas palet ${p[0].trim()}`);
    const r = rawColor.exec(line);
    if (r) found.push(`baris ${i + 1}: nilai warna ${r[0]}`);
  }
  return found;
}

function tsxFiles(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) out.push(...tsxFiles(full));
    else if (name.endsWith(".tsx") && !name.endsWith(".test.tsx")) out.push(full);
  }
  return out;
}

// ---- Test ----

const requiredTokens = [
  "background",
  "foreground",
  "card",
  "card-foreground",
  "popover",
  "popover-foreground",
  "primary",
  "primary-foreground",
  "secondary",
  "secondary-foreground",
  "muted",
  "muted-foreground",
  "accent",
  "accent-foreground",
  "destructive",
  "destructive-foreground",
  "border",
  "input",
  "ring",
  "sidebar",
  "sidebar-foreground",
  "sidebar-primary",
  "sidebar-primary-foreground",
  "sidebar-accent",
  "sidebar-accent-foreground",
  "sidebar-border",
  "sidebar-ring",
  "success",
  "success-foreground",
  "info",
  "info-foreground",
  "warning",
  "warning-foreground",
  "invert",
  "invert-foreground",
  "status-primary-bg",
  "status-primary-fg",
  "status-success-bg",
  "status-success-fg",
  "status-error-bg",
  "status-error-fg",
  "status-warning-bg",
  "status-warning-fg",
  "status-secondary-bg",
  "status-secondary-fg",
  "overlay",
  "notification-badge-bg",
  "notification-dot",
  "logout-fg",
];

describe("paritas tema (P15)", () => {
  test("blok sistem-gelap (@media) sama persis dengan blok .dark", () => {
    expect([...systemDark.entries()].sort()).toEqual([...dark.entries()].sort());
    expect(dark.size).toBeGreaterThan(30);
  });

  test("blok sistem-gelap tidak memengaruhi pilihan eksplisit (.light/.dark)", () => {
    expect(mediaBody).toContain(":root:not(.light):not(.dark)");
  });
});

describe("token wajib", () => {
  test.each(requiredTokens)("token --%s ada di :root dan .dark", (name) => {
    expect(light.has(name)).toBe(true);
    expect(dark.has(name)).toBe(true);
  });

  test("setiap token wajib terpetakan di @theme inline (--color-<token>)", () => {
    const theme = declarations(block(css, "@theme inline"));
    for (const name of requiredTokens) {
      expect(theme.get(`color-${name}`)).toBe(`var(--${name})`);
    }
  });

  test("token ukuran shell dan kontrol (K-15, design §2)", () => {
    const theme = declarations(block(css, "@theme inline"));
    expect(theme.get("spacing-control-sm")).toBe("1.75rem");
    expect(theme.get("spacing-control-md")).toBe("2.125rem");
    expect(theme.get("spacing-control-lg")).toBe("2.5rem");
    const root = declarations(block(cssWithoutMedia, ":root"));
    expect(root.get("sidebar-width")).toBe("14.25rem");
    expect(root.get("sidebar-width-icon")).toBe("3.25rem");
    expect(root.get("navbar-height")).toBe("3rem");
  });
});

describe("kontras token (K-14)", () => {
  const textPairs: Array<[string, string]> = [
    ["foreground", "background"],
    ["card-foreground", "card"],
    ["popover-foreground", "popover"],
    ["muted-foreground", "background"],
    ["muted-foreground", "muted"],
    ["muted-foreground", "secondary"],
    ["muted-foreground", "sidebar"],
    ["secondary-foreground", "secondary"],
    ["accent-foreground", "accent"],
    ["sidebar-foreground", "sidebar"],
    ["sidebar-accent-foreground", "sidebar-accent"],
    ["primary-foreground", "primary"],
    ["destructive-foreground", "destructive"],
    ["invert-foreground", "invert"],
    ["status-primary-fg", "status-primary-bg"],
    ["status-success-fg", "status-success-bg"],
    ["status-error-fg", "status-error-bg"],
    ["status-warning-fg", "status-warning-bg"],
    ["status-secondary-fg", "status-secondary-bg"],
  ];

  for (const [theme, tokens] of [
    ["terang", light],
    ["gelap", dark],
  ] as const) {
    test.each(textPairs)(`${theme}: --%s di atas --%s >= 4,5:1`, (fg, bg) => {
      const f = tokens.get(fg);
      const b = tokens.get(bg);
      expect(f).toBeDefined();
      expect(b).toBeDefined();
      expect(contrast(f as string, b as string)).toBeGreaterThanOrEqual(4.5);
    });

    test.each(["background", "muted"])(
      `${theme}: --ring di atas --%s >= 3:1 (komponen non-teks)`,
      (bg) => {
        expect(
          contrast(tokens.get("ring") as string, tokens.get(bg) as string),
        ).toBeGreaterThanOrEqual(3);
      },
    );
  }

  test("angka K-14 terpenuhi: muted-foreground gelap di secondary dan sidebar", () => {
    expect(
      contrast(dark.get("muted-foreground") as string, dark.get("secondary") as string),
    ).toBeGreaterThan(4.7);
    expect(
      contrast(dark.get("muted-foreground") as string, dark.get("sidebar") as string),
    ).toBeGreaterThan(5.5);
  });

  test("pengukur kontras sahih: hitam di putih 21:1, sama persis 1:1", () => {
    expect(contrast("#000", "#fff")).toBeCloseTo(21, 1);
    expect(contrast("#777", "#777")).toBeCloseTo(1, 5);
  });
});

describe("larangan nilai palet langsung di komponen (R2.2)", () => {
  test("pemindai menolak kelas palet, putih/hitam langsung, dan nilai warna mentah", () => {
    for (const bad of [
      `className="bg-zinc-100"`,
      `className="p-2 text-red-500"`,
      `cn("hover:bg-gray-50/50")`,
      `className="text-white"`,
      `className="border-black"`,
      `style={{ color: "#fff" }}`,
      `const c = "rgb(0 0 0)"`,
      `const c = "oklch(0.5 0 0)"`,
    ]) {
      expect(paletteViolations(bad).length).toBeGreaterThan(0);
    }
  });

  test("pemindai meloloskan kelas token semantik", () => {
    for (const ok of [
      `className="bg-background text-foreground border-border"`,
      `className="bg-status-success-bg text-status-success-fg"`,
      `className="ring-ring/50 bg-primary/90 text-muted-foreground"`,
      `className="size-4 text-sm shadow-xs"`,
    ]) {
      expect(paletteViolations(ok)).toEqual([]);
    }
  });

  test("tidak ada nilai palet langsung di src/components/**/*.tsx", () => {
    const dir = resolve(here, "..", "components");
    let files: string[] = [];
    try {
      files = tsxFiles(dir);
    } catch {
      files = [];
    }
    const problems = files.flatMap((f) =>
      paletteViolations(readFileSync(f, "utf8")).map((p) => `${f}: ${p}`),
    );
    expect(problems).toEqual([]);
  });
});
