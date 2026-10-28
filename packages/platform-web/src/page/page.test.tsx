import { describe, expect, test } from "bun:test";
import { act, render, screen } from "@testing-library/react";
import fc from "fast-check";
import { type ReactNode, useState } from "react";
import {
  isPageActionNode,
  type PageAction,
  type PageActionEntry,
  PageActions,
  PageChromeProvider,
  usePageActions,
  usePageChrome,
  usePageLayout,
} from "./index";

let chromeRenders = 0;

function Chrome() {
  const { actions, scroll } = usePageChrome();
  chromeRenders += 1;
  return (
    <div>
      <span data-testid="ids">{actions.map((a) => a.id).join(",")}</span>
      <span data-testid="labels">
        {actions.map((a) => (isPageActionNode(a) ? "node" : a.label)).join(",")}
      </span>
      <span data-testid="scroll">{String(scroll)}</span>
      {actions.map((a) =>
        isPageActionNode(a) ? (
          <span key={a.id}>{a.node}</span>
        ) : (
          <button key={a.id} type="button" onClick={a.onSelect}>
            {`run-${a.id}`}
          </button>
        ),
      )}
    </div>
  );
}

function shell(page: ReactNode) {
  return (
    <PageChromeProvider>
      <Chrome />
      {page}
    </PageChromeProvider>
  );
}

const act1 = (id: string, extra: Partial<PageAction> = {}): PageAction => ({
  id,
  label: `Aksi ${id}`,
  onSelect: () => {},
  ...extra,
});

function Page({ actions, scroll }: { actions: readonly PageActionEntry[]; scroll?: boolean }) {
  usePageActions(actions);
  usePageLayout({ scroll });
  return null;
}

describe("usePageActions", () => {
  test("terdaftar sinkron saat mount (tanpa menunggu), dan nilai awal kosong", () => {
    const { rerender } = render(shell(null));
    expect(screen.getByTestId("ids").textContent).toBe("");
    expect(screen.getByTestId("scroll").textContent).toBe("true");
    rerender(shell(<Page actions={[act1("new")]} />));
    expect(screen.getByTestId("ids").textContent).toBe("new");
  });

  test("menggantikan saat daftar berubah", () => {
    const { rerender } = render(shell(<Page actions={[act1("a")]} />));
    rerender(shell(<Page actions={[act1("a", { label: "Baru" }), act1("b")]} />));
    expect(screen.getByTestId("ids").textContent).toBe("a,b");
    expect(screen.getByTestId("labels").textContent).toBe("Baru,Aksi b");
  });

  test("membersihkan saat unmount (P13)", () => {
    const { rerender } = render(shell(<Page actions={[act1("a")]} scroll={false} />));
    expect(screen.getByTestId("scroll").textContent).toBe("false");
    rerender(shell(null));
    expect(screen.getByTestId("ids").textContent).toBe("");
    expect(screen.getByTestId("scroll").textContent).toBe("true");
  });

  test("onSelect memakai closure terbaru tanpa render ulang konsumen", () => {
    let seen = -1;
    function Counter() {
      const [n, setN] = useState(0);
      usePageActions([
        {
          id: "x",
          label: "Aksi",
          onSelect: () => {
            seen = n;
          },
        },
      ]);
      return (
        <button type="button" onClick={() => setN((v) => v + 1)}>
          tambah
        </button>
      );
    }
    render(shell(<Counter />));
    const before = chromeRenders;
    act(() => screen.getByText("tambah").click());
    act(() => screen.getByText("tambah").click());
    act(() => screen.getByText("tambah").click());
    expect(chromeRenders).toBe(before);
    act(() => screen.getByText("run-x").click());
    expect(seen).toBe(3);
  });

  test("perubahan label/disabled/loading memperbarui konsumen (bukan hanya closure)", () => {
    const { rerender } = render(shell(<Page actions={[act1("a")]} />));
    const before = chromeRenders;
    rerender(shell(<Page actions={[act1("a", { disabled: true })]} />));
    expect(chromeRenders).toBeGreaterThan(before);
  });

  test("beberapa pemanggil digabung berurutan menurut urutan mount; unmount satu tidak menghapus lainnya", () => {
    function Two({ showFirst }: { showFirst: boolean }) {
      return (
        <>
          {showFirst ? <Page actions={[act1("a")]} /> : null}
          <Page actions={[act1("b"), act1("c")]} />
        </>
      );
    }
    const { rerender } = render(shell(<Two showFirst />));
    expect(screen.getByTestId("ids").textContent).toBe("a,b,c");
    rerender(shell(<Two showFirst={false} />));
    expect(screen.getByTestId("ids").textContent).toBe("b,c");
  });

  test("simpul kustom dirender dan diperbarui saat referensi node berubah", () => {
    const { rerender } = render(shell(<Page actions={[{ id: "n", node: <i>satu</i> }]} />));
    expect(screen.getByText("satu")).toBeInTheDocument();
    rerender(shell(<Page actions={[{ id: "n", node: <i>dua</i> }]} />));
    expect(screen.getByText("dua")).toBeInTheDocument();
    expect(screen.queryByText("satu")).toBeNull();
  });

  test("PageActions (komponen) setara dengan hook", () => {
    const { rerender } = render(shell(<PageActions actions={[act1("seg")]} />));
    expect(screen.getByTestId("ids").textContent).toBe("seg");
    rerender(shell(null));
    expect(screen.getByTestId("ids").textContent).toBe("");
  });

  test("tanpa PageChromeProvider (halaman diuji sendiri) tidak melempar", () => {
    expect(() => render(<Page actions={[act1("a")]} />)).not.toThrow();
  });
});

describe("usePageLayout", () => {
  test("scroll=false mematikan gulir halaman; kembali ke true saat diubah atau unmount", () => {
    const { rerender } = render(shell(<Page actions={[]} scroll={false} />));
    expect(screen.getByTestId("scroll").textContent).toBe("false");
    rerender(shell(<Page actions={[]} scroll />));
    expect(screen.getByTestId("scroll").textContent).toBe("true");
    rerender(shell(<Page actions={[]} scroll={false} />));
    rerender(shell(null));
    expect(screen.getByTestId("scroll").textContent).toBe("true");
  });

  test("tanpa opsi scroll: bawaan true", () => {
    render(shell(<Page actions={[]} />));
    expect(screen.getByTestId("scroll").textContent).toBe("true");
  });

  test("beberapa pemanggil: false bila salah satunya false", () => {
    render(
      shell(
        <>
          <Page actions={[]} />
          <Page actions={[]} scroll={false} />
        </>,
      ),
    );
    expect(screen.getByTestId("scroll").textContent).toBe("false");
  });
});

describe("P13 (properti)", () => {
  test("setelah semua halaman unmount kembali { actions: [], scroll: true }; sisanya sesuai urutan", () => {
    fc.assert(
      fc.property(
        fc.array(
          fc.record({
            ids: fc.uniqueArray(fc.constantFrom("a", "b", "c", "d"), { maxLength: 3 }),
            scroll: fc.boolean(),
            keep: fc.boolean(),
          }),
          { maxLength: 5 },
        ),
        (pages) => {
          // ID dibuat unik antar halaman agar urutan dapat diverifikasi.
          const mk = (keepOnly: boolean) =>
            pages
              .map((p, i) => ({ ...p, i }))
              .filter((p) => !keepOnly || p.keep)
              .map((p) => (
                <Page
                  key={p.i}
                  scroll={p.scroll}
                  actions={p.ids.map((id) => act1(`${p.i}${id}`))}
                />
              ));
          const { rerender, unmount } = render(shell(mk(false)));
          const all = pages.flatMap((p, i) => p.ids.map((id) => `${i}${id}`));
          expect(screen.getByTestId("ids").textContent).toBe(all.join(","));
          expect(screen.getByTestId("scroll").textContent).toBe(
            String(pages.every((p) => p.scroll)),
          );
          rerender(shell(mk(true)));
          const kept = pages.flatMap((p, i) => (p.keep ? p.ids.map((id) => `${i}${id}`) : []));
          expect(screen.getByTestId("ids").textContent).toBe(kept.join(","));
          rerender(shell(null));
          expect(screen.getByTestId("ids").textContent).toBe("");
          expect(screen.getByTestId("scroll").textContent).toBe("true");
          unmount();
        },
      ),
      { numRuns: 60 },
    );
  });
});
