import { describe, expect, test } from "bun:test";
import { act, render, screen } from "@testing-library/react";
import { Message, msg } from "../msg";
import { useTranslation } from "./provider";
import { createTestI18n } from "./testing";
import type { RichTag } from "./types";

declare module "./types" {
  interface I18nCatalogs {
    testing: {
      "note.title": Record<never, never>;
      "note.hello": { name: string };
      "note.rich": { b: RichTag };
      "note.absent": Record<never, never>;
    };
  }
}

function Probe() {
  const { t, rich, locale, setLocale } = useTranslation("testing");
  return (
    <div>
      <span data-testid="title">{t("note.title")}</span>
      <span data-testid="hello">{t("note.hello", { name: "Rahmad" })}</span>
      <span data-testid="rich">{rich("note.rich", { b: (c) => <strong>{c}</strong> })}</span>
      <span data-testid="absent">{t("note.absent")}</span>
      <span data-testid="locale">{locale}</span>
      <button type="button" onClick={() => setLocale("en")}>
        ke-en
      </button>
    </div>
  );
}

describe("createTestI18n", () => {
  const { Provider } = createTestI18n({
    testing: {
      "note.title": "Judul",
      "note.hello": "Halo, {name}",
      "note.rich": "Teks <b>tebal</b>",
    },
  });

  test("menyediakan provider tanpa jaringan, QueryClient, atau bootstrap", () => {
    render(
      <Provider>
        <Probe />
      </Provider>,
    );
    expect(screen.getByTestId("title")).toHaveTextContent("Judul");
    expect(screen.getByTestId("hello")).toHaveTextContent("Halo, Rahmad");
    expect(screen.getByText("tebal").tagName).toBe("STRONG");
  });

  test("kunci tidak ada mengembalikan ns::kunci; locale bawaan id; setLocale mengganti locale", () => {
    render(
      <Provider>
        <Probe />
      </Provider>,
    );
    expect(screen.getByTestId("absent")).toHaveTextContent("testing::note.absent");
    expect(screen.getByTestId("locale")).toHaveTextContent("id");
    act(() => screen.getByText("ke-en").click());
    expect(screen.getByTestId("locale")).toHaveTextContent("en");
  });

  test("opsi locale; namespace yang tidak diberikan menghasilkan ns::kunci", () => {
    const en = createTestI18n({ testing: { "note.title": "Title" } }, { locale: "en" });
    render(
      <en.Provider>
        <Probe />
      </en.Provider>,
    );
    expect(screen.getByTestId("locale")).toHaveTextContent("en");
    expect(screen.getByTestId("hello")).toHaveTextContent("testing::note.hello");
  });

  test("<Message> memakai katalog uji", () => {
    render(
      <Provider>
        <Message message={msg("testing", "note.title")} />
      </Provider>,
    );
    expect(screen.getByText("Judul")).toBeInTheDocument();
  });
});
