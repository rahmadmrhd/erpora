import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useTranslation } from "./provider";
import { flattenCatalog, StaticI18nProvider } from "./static";

function Probe() {
  const { t, locale, setLocale } = useTranslation("core" as never) as unknown as {
    t: (k: string, a?: Record<string, unknown>) => string;
    locale: string;
    setLocale: (l: string) => void;
  };
  return (
    <div>
      <p>{t("hello", { name: "Ani" })}</p>
      <p>{t("tidak.ada")}</p>
      <span data-testid="locale">{locale}</span>
      <button type="button" onClick={() => setLocale("en")}>
        en
      </button>
      <button type="button" onClick={() => setLocale("zz")}>
        zz
      </button>
    </div>
  );
}

describe("StaticI18nProvider", () => {
  const catalogsByLocale = {
    id: { core: flattenCatalog({ hello: "Halo {name}" }) },
    en: { core: flattenCatalog({ hello: "Hello {name}" }) },
  };

  test("menerjemahkan dari katalog yang dibundel tanpa jaringan; kunci hilang menjadi ns::kunci", () => {
    render(
      <StaticI18nProvider locale="id" catalogsByLocale={catalogsByLocale}>
        <Probe />
      </StaticI18nProvider>,
    );
    expect(screen.getByText("Halo Ani")).toBeInTheDocument();
    expect(screen.getByText("core::tidak.ada")).toBeInTheDocument();
  });

  test("setLocale mengganti bahasa hanya ke bahasa yang dibundel", async () => {
    const user = userEvent.setup();
    render(
      <StaticI18nProvider locale="id" catalogsByLocale={catalogsByLocale}>
        <Probe />
      </StaticI18nProvider>,
    );
    await user.click(screen.getByRole("button", { name: "zz" }));
    expect(screen.getByTestId("locale").textContent).toBe("id");
    await user.click(screen.getByRole("button", { name: "en" }));
    expect(screen.getByText("Hello Ani")).toBeInTheDocument();
  });

  test("flattenCatalog meratakan objek bersarang dan mengabaikan nilai bukan string", () => {
    expect(flattenCatalog({ a: { b: "x", c: 1, d: { e: "y" } }, f: null })).toEqual({
      "a.b": "x",
      "a.d.e": "y",
    });
  });
});
