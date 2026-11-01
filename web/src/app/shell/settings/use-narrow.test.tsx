import { afterEach, describe, expect, test } from "bun:test";
import { act, render, screen } from "@testing-library/react";
import { installMatchMedia, restoreMatchMedia } from "./match-media-test-support";
import { useIsNarrow } from "./use-narrow";

afterEach(() => {
  restoreMatchMedia();
});

function Probe({ seen }: { seen: boolean[] }) {
  const narrow = useIsNarrow();
  seen.push(narrow);
  return <span data-testid="v">{String(narrow)}</span>;
}

describe("useIsNarrow", () => {
  test("render pertama sudah benar di ponsel (nilai awal sinkron, tanpa menunggu efek)", () => {
    installMatchMedia(375);
    const seen: boolean[] = [];
    render(<Probe seen={seen} />);
    expect(seen[0]).toBe(true);
    expect(seen.every((v) => v)).toBe(true);
    expect(screen.getByTestId("v")).toHaveTextContent("true");
  });

  test("render pertama di desktop bernilai false", () => {
    installMatchMedia(1280);
    const seen: boolean[] = [];
    render(<Probe seen={seen} />);
    expect(seen.every((v) => !v)).toBe(true);
  });

  test("ambang 768 px: 767 sempit, 768 lebar", () => {
    installMatchMedia(767);
    const a: boolean[] = [];
    const first = render(<Probe seen={a} />);
    expect(a[0]).toBe(true);
    first.unmount();
    installMatchMedia(768);
    const b: boolean[] = [];
    render(<Probe seen={b} />);
    expect(b[0]).toBe(false);
  });

  test("perubahan lebar (event change) tetap bereaksi", () => {
    const media = installMatchMedia(1280);
    render(<Probe seen={[]} />);
    expect(screen.getByTestId("v")).toHaveTextContent("false");
    act(() => media.setWidth(375));
    expect(screen.getByTestId("v")).toHaveTextContent("true");
    act(() => media.setWidth(1024));
    expect(screen.getByTestId("v")).toHaveTextContent("false");
  });

  test("tanpa matchMedia (lingkungan tanpa dukungan) dianggap lebar tanpa melempar", () => {
    const original = window.matchMedia;
    // biome-ignore lint/suspicious/noExplicitAny: menghapus API untuk menguji ketiadaannya
    (window as any).matchMedia = undefined;
    try {
      const seen: boolean[] = [];
      render(<Probe seen={seen} />);
      expect(seen[0]).toBe(false);
    } finally {
      window.matchMedia = original;
    }
  });
});
