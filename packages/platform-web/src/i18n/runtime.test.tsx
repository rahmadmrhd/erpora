import { afterEach, beforeEach, describe, expect, spyOn, test } from "bun:test";
import { createClient, type PlatformPaths } from "@erpora/platform-contracts";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import { Component, type ReactNode, Suspense } from "react";
import { StaticBootstrapProvider } from "../bootstrap";
import { makeBootstrap } from "../bootstrap/testing";
import type { Bootstrap } from "../bootstrap/types";
import { Message, msg } from "../msg";
import { PREF_KEYS } from "../prefs";
import { getActiveLocale, setActiveLocale } from "./locale";
import { I18nProvider, useTranslation } from "./provider";
import { resetTranslateWarnings } from "./translate";
import type { RichTag } from "./types";

// Katalog uji (augmentasi lokal; tidak memengaruhi paket lain).
declare module "./types" {
  interface I18nCatalogs {
    example: {
      "note.title": Record<never, never>;
      "note.count": { count: number };
      "note.broken": Record<never, never>;
      "note.rich": { name: string; b: RichTag };
    };
  }
}

const TENANT = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b";

type Catalog = { revision: string; messages: Record<string, string> };

/** Server katalog tiruan: lacak permintaan, dukung ETag/304, katalog dapat diubah di tengah test. */
function catalogServer(data: Record<string, Record<string, Catalog>>) {
  const calls: { locale: string; ns: string; ifNoneMatch: string | null; lang: string | null }[] =
    [];
  const fetchFn = (async (url: string | URL | Request, init?: RequestInit) => {
    const m = /\/api\/platform\/i18n\/([^/]+)\/([^/]+)$/.exec(String(url));
    const headers = new Headers(init?.headers);
    const locale = decodeURIComponent(m?.[1] ?? "");
    const ns = decodeURIComponent(m?.[2] ?? "");
    calls.push({
      locale,
      ns,
      ifNoneMatch: headers.get("If-None-Match"),
      lang: headers.get("Accept-Language"),
    });
    const cat = data[locale]?.[ns];
    if (!cat) {
      return new Response(JSON.stringify({ error: { code: "not_found", message: "x" } }), {
        status: 404,
      });
    }
    const etag = `"${cat.revision}"`;
    if (headers.get("If-None-Match") === etag) return new Response(null, { status: 304 });
    return new Response(
      JSON.stringify({
        data: { locale, namespace: ns, revision: cat.revision, messages: cat.messages },
      }),
      { status: 200, headers: { "Content-Type": "application/json", ETag: etag } },
    );
  }) as unknown as typeof fetch;
  return { calls, data, fetchFn };
}

const baseCatalogs = () => ({
  id: {
    example: {
      revision: "r1",
      messages: {
        "note.title": "Judul Catatan",
        "note.count": "{count, plural, one {# catatan} other {# catatan}}",
        "note.broken": "{broken",
        "note.rich": "Halo <b>{name}</b>",
      },
    },
  },
  en: {
    example: {
      revision: "e1",
      messages: {
        "note.title": "Note Title",
        "note.count": "{count, plural, one {# note} other {# notes}}",
        "note.rich": "Hello <b>{name}</b>",
      },
    },
  },
});

function bootstrapWith(
  revisions: Record<string, string> = { example: "r1" },
  over: Partial<Bootstrap> = {},
) {
  return makeBootstrap({
    tenantId: TENANT,
    i18n: {
      defaultLocale: "id",
      locales: [
        { code: "id", name: "Indonesia" },
        { code: "en", name: "English" },
      ],
      revisions,
    },
    ...over,
  });
}

function mount(
  ui: ReactNode,
  server: ReturnType<typeof catalogServer>,
  bootstrap: Bootstrap = bootstrapWith(),
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  const client = createClient<PlatformPaths>({
    fetch: server.fetchFn,
    getLocale: () => getActiveLocale(),
  });
  const tree = (b: Bootstrap) => (
    <QueryClientProvider client={queryClient}>
      <StaticBootstrapProvider value={b}>
        <I18nProvider client={client}>
          <Suspense fallback={<span>memuat</span>}>{ui}</Suspense>
        </I18nProvider>
      </StaticBootstrapProvider>
    </QueryClientProvider>
  );
  const result = render(tree(bootstrap));
  return { ...result, queryClient, rerenderWith: (b: Bootstrap) => result.rerender(tree(b)) };
}

function Probe() {
  const { t, locale, setLocale } = useTranslation("example");
  return (
    <div>
      <span data-testid="title">{t("note.title")}</span>
      <span data-testid="count">{t("note.count", { count: 1 })}</span>
      <span data-testid="count2">{t("note.count", { count: 3 })}</span>
      <span data-testid="broken">{t("note.broken")}</span>
      <span data-testid="locale">{locale}</span>
      <button type="button" onClick={() => setLocale("en")}>
        ke-en
      </button>
      <button type="button" onClick={() => setLocale("fr")}>
        ke-fr
      </button>
    </div>
  );
}

const setLanguages = (langs: string[]) =>
  Object.defineProperty(navigator, "languages", { value: langs, configurable: true });

beforeEach(() => {
  resetTranslateWarnings();
  localStorage.clear();
  setLanguages([]);
  document.documentElement.lang = "";
});

afterEach(() => {
  setActiveLocale(undefined);
  localStorage.clear();
});

describe("useTranslation: memuat dan menerjemahkan", () => {
  test("Suspense: kunci mentah tidak pernah tampil selama memuat, lalu teks terjemahan", async () => {
    const s = catalogServer(baseCatalogs());
    mount(<Probe />, s);
    expect(screen.getByText("memuat")).toBeInTheDocument();
    expect(screen.queryByText("note.title")).toBeNull();
    await waitFor(() => expect(screen.getByTestId("title")).toHaveTextContent("Judul Catatan"));
    expect(screen.queryByText("note.title")).toBeNull();
    expect(s.calls).toHaveLength(1);
    expect(s.calls[0]).toMatchObject({
      locale: "id",
      ns: "example",
      ifNoneMatch: null,
      lang: "id",
    });
  });

  test("plural ICU lewat t(key, args)", async () => {
    mount(<Probe />, catalogServer(baseCatalogs()));
    await waitFor(() => expect(screen.getByTestId("count")).toHaveTextContent("1 catatan"));
    expect(screen.getByTestId("count2")).toHaveTextContent("3 catatan");
  });

  test("pesan yang gagal dikompilasi (I18nFormatError) mengembalikan ns::kunci", async () => {
    mount(<Probe />, catalogServer(baseCatalogs()));
    await waitFor(() =>
      expect(screen.getByTestId("broken")).toHaveTextContent("example::note.broken"),
    );
  });

  test("kunci yang tidak ada di katalog mengembalikan ns::kunci (tidak melempar)", async () => {
    function Missing() {
      const { t } = useTranslation("example");
      return <span data-testid="m">{t("note.title")}</span>;
    }
    const data = baseCatalogs();
    data.id.example.messages = {} as never;
    mount(<Missing />, catalogServer(data));
    await waitFor(() => expect(screen.getByTestId("m")).toHaveTextContent("example::note.title"));
  });

  test("rich merender tag sebagai elemen React (anak tetap teks)", async () => {
    function Rich() {
      const { rich } = useTranslation("example");
      return (
        <p data-testid="r">
          {rich("note.rich", { name: "Rahmad", b: (chunks) => <strong>{chunks}</strong> })}
        </p>
      );
    }
    mount(<Rich />, catalogServer(baseCatalogs()));
    await waitFor(() => expect(screen.getByTestId("r")).toHaveTextContent("Halo Rahmad"));
    expect(screen.getByText("Rahmad").tagName).toBe("STRONG");
  });

  test("tag rich dilucuti pada t() (teks polos)", async () => {
    function Plain() {
      const { t } = useTranslation("example");
      return <span data-testid="p">{t("note.rich", { name: "Rahmad" })}</span>;
    }
    mount(<Plain />, catalogServer(baseCatalogs()));
    await waitFor(() => expect(screen.getByTestId("p")).toHaveTextContent("Halo Rahmad"));
  });

  test("teks terjemahan dirender sebagai teks, bukan HTML", async () => {
    const data = baseCatalogs();
    data.id.example.messages["note.title"] = "Kode '<'img src=x onerror=alert(1)'>' aman";
    const { container } = mount(<Probe />, catalogServer(data));
    await waitFor(() =>
      expect(screen.getByTestId("title")).toHaveTextContent("<img src=x onerror=alert(1)>"),
    );
    expect(container.querySelector("img")).toBeNull();
  });

  test("key query berawalan tenant: [tenantId, i18n, locale, ns], katalog tidak kedaluwarsa (staleTime Infinity)", async () => {
    const s = catalogServer(baseCatalogs());
    const { queryClient } = mount(<Probe />, s);
    await waitFor(() => expect(screen.getByTestId("title")).toHaveTextContent("Judul Catatan"));
    const q = queryClient.getQueryCache().find({ queryKey: [TENANT, "i18n", "id", "example"] });
    expect(q).toBeDefined();
    expect(q?.isStale()).toBe(false);
    for (const query of queryClient.getQueryCache().getAll())
      expect(query.queryKey[0]).toBe(TENANT);
  });

  test("namespace modul yang katalognya 404 (modul nonaktif) non-fatal: ns::kunci, tanpa melempar, peringatan dev", async () => {
    const warn = spyOn(console, "warn").mockImplementation(() => {});
    try {
      const data = baseCatalogs();
      // katalog `example` tidak tersaji (modul nonaktif) sedangkan revisi bootstrap tidak memuatnya
      mount(<Probe />, catalogServer({ id: {}, en: data.en }), bootstrapWith({}));
      await waitFor(() =>
        expect(screen.getByTestId("title")).toHaveTextContent("example::note.title"),
      );
      expect(screen.queryByText("memuat")).toBeNull();
      expect(warn.mock.calls.some((c) => String(c[0]).includes("example::note.title"))).toBe(true);
    } finally {
      warn.mockRestore();
    }
  });

  test("namespace modul 404 lalu diaktifkan (revisi muncul di bootstrap): katalog dimuat ulang", async () => {
    const data = baseCatalogs();
    const server = catalogServer({ id: {}, en: data.en });
    const view = mount(<Probe />, server, bootstrapWith({}));
    await waitFor(() =>
      expect(screen.getByTestId("title")).toHaveTextContent("example::note.title"),
    );
    server.data.id = data.id;
    view.rerenderWith(bootstrapWith({ example: "r1" }));
    await waitFor(() => expect(screen.getByTestId("title")).toHaveTextContent("Judul Catatan"));
  });

  test("katalog `platform`/`ui` yang 404 tetap fatal (ke error boundary)", async () => {
    class Boundary extends Component<{ children: ReactNode }, { failed: boolean }> {
      state = { failed: false };
      static getDerivedStateFromError() {
        return { failed: true };
      }
      render() {
        return this.state.failed ? <span>gagal-muat</span> : this.props.children;
      }
    }
    function Core() {
      const { t } = useTranslation("platform" as never);
      return <span>{(t as unknown as (k: string) => string)("x")}</span>;
    }
    const err = spyOn(console, "error").mockImplementation(() => {});
    try {
      mount(
        <Boundary>
          <Core />
        </Boundary>,
        catalogServer({ id: {}, en: {} }),
      );
      await waitFor(() => expect(screen.getByText("gagal-muat")).toBeInTheDocument());
    } finally {
      err.mockRestore();
    }
  });

  test("useTranslation di luar I18nProvider melempar galat yang jelas", () => {
    const err = spyOn(console, "error").mockImplementation(() => {});
    try {
      expect(() => render(<Probe />)).toThrow(/I18nProvider/);
    } finally {
      err.mockRestore();
    }
  });
});

describe("pemilihan bahasa", () => {
  test("tanpa pilihan dan tanpa navigator cocok: bahasa bawaan; <html lang> diperbarui", async () => {
    const s = catalogServer(baseCatalogs());
    mount(<Probe />, s);
    await waitFor(() => expect(screen.getByTestId("locale")).toHaveTextContent("id"));
    expect(document.documentElement.lang).toBe("id");
  });

  test("navigator.languages dipakai bila tidak ada pilihan tersimpan", async () => {
    setLanguages(["fr", "en-US"]);
    const s = catalogServer(baseCatalogs());
    mount(<Probe />, s);
    await waitFor(() => expect(screen.getByTestId("title")).toHaveTextContent("Note Title"));
    expect(document.documentElement.lang).toBe("en");
    expect(s.calls[0]?.locale).toBe("en");
  });

  test("pilihan tersimpan yang valid menang atas navigator", async () => {
    setLanguages(["en"]);
    localStorage.setItem(PREF_KEYS.locale, "id");
    mount(<Probe />, catalogServer(baseCatalogs()));
    await waitFor(() => expect(screen.getByTestId("title")).toHaveTextContent("Judul Catatan"));
  });

  test("pilihan tersimpan yang tidak terdaftar diabaikan", async () => {
    localStorage.setItem(PREF_KEYS.locale, "fr");
    setLanguages(["en"]);
    mount(<Probe />, catalogServer(baseCatalogs()));
    await waitFor(() => expect(screen.getByTestId("locale")).toHaveTextContent("en"));
  });

  test("localStorage melempar: tidak merusak, memakai navigator/bawaan", async () => {
    const get = spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    const set = spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    try {
      setLanguages(["en"]);
      mount(<Probe />, catalogServer(baseCatalogs()));
      await waitFor(() => expect(screen.getByTestId("locale")).toHaveTextContent("en"));
      act(() => screen.getByText("ke-en").click());
      expect(screen.getByTestId("locale")).toHaveTextContent("en");
    } finally {
      get.mockRestore();
      set.mockRestore();
    }
  });

  test("setLocale memuat katalog bahasa baru tanpa reload, menyimpan pilihan, memperbarui lang dan header", async () => {
    const s = catalogServer(baseCatalogs());
    mount(<Probe />, s);
    await waitFor(() => expect(screen.getByTestId("title")).toHaveTextContent("Judul Catatan"));
    act(() => screen.getByText("ke-en").click());
    await waitFor(() => expect(screen.getByTestId("title")).toHaveTextContent("Note Title"));
    expect(localStorage.getItem(PREF_KEYS.locale)).toBe("en");
    expect(document.documentElement.lang).toBe("en");
    expect(s.calls.at(-1)).toMatchObject({ locale: "en", ns: "example", lang: "en" });
    expect(getActiveLocale()).toBe("en");
  });

  test("setLocale ke bahasa tak terdaftar diabaikan (tanpa permintaan dan tanpa perubahan)", async () => {
    const s = catalogServer(baseCatalogs());
    mount(<Probe />, s);
    await waitFor(() => expect(screen.getByTestId("title")).toHaveTextContent("Judul Catatan"));
    const before = s.calls.length;
    act(() => screen.getByText("ke-fr").click());
    expect(screen.getByTestId("locale")).toHaveTextContent("id");
    expect(localStorage.getItem(PREF_KEYS.locale)).toBeNull();
    expect(s.calls).toHaveLength(before);
  });
});

describe("revisi katalog dan ETag", () => {
  test("revisi di bootstrap berubah: katalog dimuat ulang dengan If-None-Match dan teks diperbarui", async () => {
    const s = catalogServer(baseCatalogs());
    const view = mount(<Probe />, s, bootstrapWith({ example: "r1" }));
    await waitFor(() => expect(screen.getByTestId("title")).toHaveTextContent("Judul Catatan"));
    expect(s.calls).toHaveLength(1);

    s.data.id = {
      example: {
        revision: "r2",
        messages: { ...baseCatalogs().id.example.messages, "note.title": "Judul Baru" },
      },
    };
    view.rerenderWith(bootstrapWith({ example: "r2" }));
    await waitFor(() => expect(screen.getByTestId("title")).toHaveTextContent("Judul Baru"));
    expect(s.calls).toHaveLength(2);
    expect(s.calls[1]?.ifNoneMatch).toBe('"r1"');
  });

  test("revisi bootstrap sama: tidak ada permintaan ulang", async () => {
    const s = catalogServer(baseCatalogs());
    const view = mount(<Probe />, s, bootstrapWith({ example: "r1" }));
    await waitFor(() => expect(screen.getByTestId("title")).toHaveTextContent("Judul Catatan"));
    view.rerenderWith(bootstrapWith({ example: "r1" }));
    view.rerenderWith({ ...bootstrapWith({ example: "r1" }), revision: 99 });
    expect(s.calls).toHaveLength(1);
  });

  test("server membalas 304 untuk revisi baru di bootstrap: sekali saja (tanpa loop), teks tetap", async () => {
    const s = catalogServer(baseCatalogs());
    const view = mount(<Probe />, s, bootstrapWith({ example: "r1" }));
    await waitFor(() => expect(screen.getByTestId("title")).toHaveTextContent("Judul Catatan"));
    view.rerenderWith(bootstrapWith({ example: "zzz" }));
    await waitFor(() => expect(s.calls).toHaveLength(2));
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });
    expect(s.calls).toHaveLength(2);
    expect(s.calls[1]?.ifNoneMatch).toBe('"r1"');
    expect(screen.getByTestId("title")).toHaveTextContent("Judul Catatan");
  });
});

describe("Message terhubung ke runtime (M3a 9.4b)", () => {
  test("<Message> di dalam I18nProvider menampilkan terjemahan namespace rujukan", async () => {
    mount(<Message message={msg("example", "note.title")} />, catalogServer(baseCatalogs()));
    await waitFor(() => expect(screen.getByText("Judul Catatan")).toBeInTheDocument());
  });
});
