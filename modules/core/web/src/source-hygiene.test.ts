// Kebersihan sumber layar auth (PRD-001 Q33; CLAUDE.md): tanpa dangerouslySetInnerHTML dan tanpa teks UI tertanam
// (semua teks lewat i18n). Pemeriksaan membaca AST TypeScript, bukan regex pada teks mentah.
import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const srcDir = dirname(fileURLToPath(import.meta.url));

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) return name === "i18n" ? [] : sources(full);
    const isCode = /\.(ts|tsx)$/.test(name) && !/\.(test|gen)\.tsx?$/.test(name);
    return isCode && name !== "testing.tsx" ? [full] : [];
  });
}

const files = sources(srcDir).map((f) => ({
  rel: relative(srcDir, f),
  ast: ts.createSourceFile(
    f,
    readFileSync(f, "utf8"),
    ts.ScriptTarget.ES2023,
    true,
    ts.ScriptKind.TSX,
  ),
}));

// Atribut JSX yang tampil atau dibacakan ke pengguna.
const TEXT_ATTRS = new Set(["aria-label", "title", "placeholder", "alt", "label"]);

function walk(node: ts.Node, visit: (n: ts.Node) => void) {
  visit(node);
  ts.forEachChild(node, (c) => walk(c, visit));
}

describe("sumber layar auth modul core", () => {
  test("ada berkas yang diperiksa", () => {
    expect(files.length).toBeGreaterThan(8);
  });

  test("tidak memakai dangerouslySetInnerHTML, innerHTML, atau outerHTML", () => {
    for (const { rel, ast } of files) {
      walk(ast, (n) => {
        const text = ts.isIdentifier(n) || ts.isJsxAttribute(n) ? n.getText() : "";
        expect(text.includes("dangerouslySetInnerHTML"), rel).toBe(false);
        if (ts.isPropertyAccessExpression(n)) {
          expect(["innerHTML", "outerHTML"].includes(n.name.text), rel).toBe(false);
        }
      });
    }
  });

  test("tidak ada teks JSX tertanam: semua teks tampil lewat t()/katalog", () => {
    const offenders: string[] = [];
    for (const { rel, ast } of files) {
      walk(ast, (n) => {
        if (ts.isJsxText(n) && /[\p{L}]/u.test(n.getText())) {
          offenders.push(`${rel}: teks "${n.getText().trim()}"`);
        }
        if (
          ts.isJsxAttribute(n) &&
          TEXT_ATTRS.has(n.name.getText()) &&
          n.initializer &&
          ts.isStringLiteral(n.initializer) &&
          /[\p{L}]/u.test(n.initializer.text)
        ) {
          offenders.push(`${rel}: atribut ${n.name.getText()}="${n.initializer.text}"`);
        }
      });
    }
    expect(offenders).toEqual([]);
  });
});
