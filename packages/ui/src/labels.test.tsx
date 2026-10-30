import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { render, screen } from "@testing-library/react";
import { type UiKey, UiLabelsProvider, uiKeys, useUiLabel } from "./labels";

function Probe({ k }: { k: UiKey }) {
  const t = useUiLabel();
  return <p data-testid="out">{t(k, { name: "Budi" })}</p>;
}

describe("useUiLabel", () => {
  test("tanpa provider mengembalikan kunci apa adanya", () => {
    render(<Probe k="actions.close" />);
    expect(screen.getByTestId("out")).toHaveTextContent("actions.close");
  });

  test("dengan UiLabelsProvider memanggil fungsi t dengan kunci dan argumen", () => {
    const calls: Array<[string, unknown]> = [];
    render(
      <UiLabelsProvider
        t={(key, args) => {
          calls.push([key, args]);
          return `[${key}]`;
        }}
      >
        <Probe k="actions.close" />
      </UiLabelsProvider>,
    );
    expect(screen.getByTestId("out")).toHaveTextContent("[actions.close]");
    expect(calls).toEqual([["actions.close", { name: "Budi" }]]);
  });

  test("provider bersarang: yang terdalam menang", () => {
    render(
      <UiLabelsProvider t={() => "luar"}>
        <UiLabelsProvider t={() => "dalam"}>
          <Probe k="actions.retry" />
        </UiLabelsProvider>
      </UiLabelsProvider>,
    );
    expect(screen.getByTestId("out")).toHaveTextContent("dalam");
  });
});

// Sampai keys.gen.ts (i18n:generate) tersedia, UiKey ditulis tangan: setiap kunci wajib ada di katalog id dan en.
describe("UiKey terhadap katalog namespace ui", () => {
  const here = dirname(fileURLToPath(import.meta.url));

  function flatten(node: unknown, prefix = ""): string[] {
    if (typeof node === "string") return [prefix];
    if (node && typeof node === "object") {
      return Object.entries(node).flatMap(([k, v]) => flatten(v, prefix ? `${prefix}.${k}` : k));
    }
    return [];
  }

  test.each(["id", "en"])("setiap UiKey ada di i18n/%s.json", (locale) => {
    const catalog = JSON.parse(readFileSync(join(here, "..", "i18n", `${locale}.json`), "utf8"));
    const keys = new Set(flatten(catalog));
    const missing = uiKeys.filter((k) => !keys.has(k));
    expect(missing).toEqual([]);
  });

  test("tidak ada kunci ganda", () => {
    expect(new Set(uiKeys).size).toBe(uiKeys.length);
  });
});
