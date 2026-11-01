import { describe, expect, test } from "bun:test";
import { field, invoiceMeta } from "./fixtures";
import { checkClientMeta, createResourceMetaLoader, MetadataLoadError } from "./loader";
import type { ResourceMeta } from "./meta";

const withField = (name: string, extra: Record<string, unknown>): ResourceMeta => ({
  ...invoiceMeta,
  fields: [...invoiceMeta.fields, { ...field(name, "string"), ...extra }],
});

describe("pemuat malas per resource", () => {
  test("memuat sekali, men-cache, dan peek sebelum dan sesudah muat", async () => {
    let calls = 0;
    const loader = createResourceMetaLoader();
    loader.register("sales.invoice", async () => {
      calls++;
      return { resourceMeta: invoiceMeta };
    });
    expect(loader.peek("sales.invoice")).toBeUndefined();
    expect(calls).toBe(0); // tidak ada muat sampai diminta
    const [a, b] = await Promise.all([loader.load("sales.invoice"), loader.load("sales.invoice")]);
    expect(a).toBe(invoiceMeta);
    expect(b).toBe(invoiceMeta);
    expect(calls).toBe(1); // permintaan bersamaan dipadukan
    await loader.load("sales.invoice");
    expect(calls).toBe(1);
    expect(loader.peek("sales.invoice")).toBe(invoiceMeta);
  });

  test("tiap resource punya importer sendiri (chunk terpisah)", async () => {
    const loader = createResourceMetaLoader();
    const called: string[] = [];
    loader.register("sales.invoice", async () => {
      called.push("invoice");
      return { resourceMeta: invoiceMeta };
    });
    loader.register("sales.order", async () => {
      called.push("order");
      return { resourceMeta: { ...invoiceMeta, key: "sales.order" } };
    });
    await loader.load("sales.order");
    expect(called).toEqual(["order"]);
    expect(loader.keys().sort()).toEqual(["sales.invoice", "sales.order"]);
  });

  test("kunci tidak terdaftar: MetadataLoadError not_registered", async () => {
    const loader = createResourceMetaLoader();
    await expect(loader.load("sales.ghost")).rejects.toMatchObject({ code: "not_registered" });
    await expect(loader.load("__proto__")).rejects.toBeInstanceOf(MetadataLoadError);
    await expect(loader.load("toString")).rejects.toBeInstanceOf(MetadataLoadError);
  });

  test("kegagalan impor tidak di-cache (dapat dicoba lagi)", async () => {
    let n = 0;
    const loader = createResourceMetaLoader();
    loader.register("sales.invoice", async () => {
      if (++n === 1) throw new Error("jaringan");
      return { resourceMeta: invoiceMeta };
    });
    await expect(loader.load("sales.invoice")).rejects.toMatchObject({ code: "import_failed" });
    expect(await loader.load("sales.invoice")).toBe(invoiceMeta);
  });

  test("chunk dengan kunci berbeda atau tanpa ekspor ditolak", async () => {
    const loader = createResourceMetaLoader();
    loader.register("sales.invoice", async () => ({
      resourceMeta: { ...invoiceMeta, key: "sales.other" },
    }));
    await expect(loader.load("sales.invoice")).rejects.toMatchObject({ code: "key_mismatch" });
    loader.register("sales.empty", async () => ({}) as never);
    await expect(loader.load("sales.empty")).rejects.toMatchObject({ code: "invalid_module" });
  });

  test("registrasi ganda ditolak", () => {
    const loader = createResourceMetaLoader();
    loader.register("sales.invoice", async () => ({ resourceMeta: invoiceMeta }));
    expect(() =>
      loader.register("sales.invoice", async () => ({ resourceMeta: invoiceMeta })),
    ).toThrow();
  });

  test("chunk yang bocor (clientVisible:false atau aturan scope server) ditolak, bukan dipakai", async () => {
    const loader = createResourceMetaLoader();
    loader.register("sales.invoice", async () => ({
      resourceMeta: withField("secretNote", { clientVisible: false }),
    }));
    await expect(loader.load("sales.invoice")).rejects.toMatchObject({ code: "unsafe_for_client" });
  });
});

describe("metadata klien tidak memuat apa yang harus tetap di server (D-8, D-21, Requirement 18.3 dan 18.8)", () => {
  test("metadata sah: tanpa pelanggaran", () => {
    expect(checkClientMeta(invoiceMeta)).toEqual([]);
  });

  test("field clientVisible: false (atau alasannya) tidak boleh ada", () => {
    expect(checkClientMeta(withField("a", { clientVisible: false })).join()).toContain(
      "clientVisible",
    );
    expect(checkClientMeta(withField("a", { clientVisibleReason: "rahasia" })).join()).toContain(
      "clientVisibleReason",
    );
  });

  test("aturan scope: server tidak boleh ada", () => {
    const m = {
      ...invoiceMeta,
      rules: [
        ...invoiceMeta.rules,
        { id: "s", assert: { field: "qty", op: "gt", value: 0 }, message: "e.x", scope: "server" },
      ],
    } as unknown as ResourceMeta;
    expect(checkClientMeta(m).join()).toContain("scope");
  });

  test("kondisi yang merujuk field confidential ke atas atau grup non-default tidak boleh ada di klien", () => {
    const m: ResourceMeta = {
      ...invoiceMeta,
      fields: [
        ...invoiceMeta.fields,
        field("costPrice", "decimal", { classification: "confidential" }),
        field("margin", "decimal", { fieldGroup: "finance" }),
      ],
      rules: [
        { id: "a", assert: { field: "costPrice", op: "gt", value: "0" }, message: "e.a" },
        { id: "b", assert: { field: "qty", op: "gt", ref: "margin" }, message: "e.b" },
      ],
    };
    const v = checkClientMeta(m).join("\n");
    expect(v).toContain("costPrice");
    expect(v).toContain("margin");
  });

  test("rujukan ke field yang tidak ada di metadata (mis. disembunyikan) dilaporkan", () => {
    const m: ResourceMeta = {
      ...invoiceMeta,
      rules: [{ id: "a", assert: { field: "hiddenField", op: "eq", value: "x" }, message: "e.a" }],
    };
    expect(checkClientMeta(m).join()).toContain("hiddenField");
  });

  test("kondisi rusak dilaporkan", () => {
    const m = withField("bad", { visibleWhen: { field: "qty" } });
    expect(checkClientMeta(m).join()).toContain("bad");
  });

  test("nilai field tidak pernah ikut: tipe metadata tidak punya tempat untuk data pengguna", () => {
    const json = JSON.stringify(invoiceMeta);
    expect(json).not.toContain("clientVisible");
    expect(json).not.toContain('"scope"');
  });
});
