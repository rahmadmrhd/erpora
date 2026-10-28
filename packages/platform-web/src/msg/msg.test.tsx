import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { createMsgHelpers } from "./helpers";
import { Message, MessageTranslatorProvider, useMessage } from "./message";

type Catalogs = {
  example: { "nav.notes": string; "page.greeting": string };
  platform: { "app.name": string };
};
const { msg, crumb } = createMsgHelpers<Catalogs>();

describe("msg() dan crumb()", () => {
  test("msg menghasilkan MessageRef { ns, key }", () => {
    expect(msg("example", "nav.notes")).toEqual({ ns: "example", key: "nav.notes" });
  });

  test("crumb membungkus MessageRef sebagai { ref }", () => {
    expect(crumb("platform", "app.name")).toEqual({ ref: { ns: "platform", key: "app.name" } });
  });

  test("hasilnya beku", () => {
    expect(Object.isFrozen(msg("example", "nav.notes"))).toBe(true);
    expect(Object.isFrozen(crumb("example", "nav.notes"))).toBe(true);
  });
});

describe("Message dan useMessage", () => {
  const dictionary: Record<string, Record<string, string>> = {
    example: { "nav.notes": "Catatan", "page.greeting": "Halo, {name}" },
  };
  const translator = (ns: string) => (key: string, args?: Record<string, string | number>) => {
    const raw = dictionary[ns]?.[key] ?? `?${ns}:${key}`;
    return raw.replace(/\{(\w+)\}/g, (_, k: string) => String(args?.[k] ?? ""));
  };

  test("dengan penerjemah terpasang: teks dari katalog namespace rujukan", () => {
    render(
      <MessageTranslatorProvider useTranslate={translator}>
        <Message message={msg("example", "nav.notes")} />
      </MessageTranslatorProvider>,
    );
    expect(screen.getByText("Catatan")).toBeInTheDocument();
  });

  test("argumen diteruskan", () => {
    render(
      <MessageTranslatorProvider useTranslate={translator}>
        <Message message={msg("example", "page.greeting")} args={{ name: "Rahmad" }} />
      </MessageTranslatorProvider>,
    );
    expect(screen.getByText("Halo, Rahmad")).toBeInTheDocument();
  });

  test("tanpa penerjemah (runtime i18n belum dipasang): kunci apa adanya", () => {
    render(<Message message={msg("example", "nav.notes")} />);
    expect(screen.getByText("nav.notes")).toBeInTheDocument();
  });

  test("useMessage mengembalikan string untuk atribut seperti aria-label", () => {
    function Probe() {
      const text = useMessage(msg("example", "nav.notes"));
      return <button type="button" aria-label={text} />;
    }
    render(
      <MessageTranslatorProvider useTranslate={translator}>
        <Probe />
      </MessageTranslatorProvider>,
    );
    expect(screen.getByLabelText("Catatan")).toBeInTheDocument();
  });

  test("teks dirender sebagai teks (bukan HTML)", () => {
    const evil = (_ns: string) => () => "<img src=x onerror=alert(1)>";
    const { container } = render(
      <MessageTranslatorProvider useTranslate={evil}>
        <Message message={msg("example", "nav.notes")} />
      </MessageTranslatorProvider>,
    );
    expect(container.querySelector("img")).toBeNull();
    expect(container.textContent).toBe("<img src=x onerror=alert(1)>");
  });
});
