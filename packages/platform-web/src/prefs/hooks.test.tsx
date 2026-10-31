import { afterEach, describe, expect, test } from "bun:test";
import { act, render, screen } from "@testing-library/react";
import { useDockMode, useTimezone } from "./hooks";
import { writeDockMode, writeTimezone } from "./index";

afterEach(() => {
  localStorage.clear();
});

function Probe() {
  const dock = useDockMode();
  const tz = useTimezone();
  return (
    <div>
      <span data-testid="dock">{dock}</span>
      <span data-testid="tz">{tz ?? "auto"}</span>
    </div>
  );
}

describe("hook preferensi", () => {
  test("useDockMode dan useTimezone memuat nilai awal dan berubah seketika saat ditulis", () => {
    render(<Probe />);
    expect(screen.getByTestId("dock")).toHaveTextContent("floating");
    expect(screen.getByTestId("tz")).toHaveTextContent("auto");
    act(() => writeDockMode("pinned"));
    expect(screen.getByTestId("dock")).toHaveTextContent("pinned");
    act(() => writeTimezone("Asia/Jakarta"));
    expect(screen.getByTestId("tz")).toHaveTextContent("Asia/Jakarta");
    act(() => writeTimezone(null));
    expect(screen.getByTestId("tz")).toHaveTextContent("auto");
  });

  test("perubahan dari tab lain (event storage) ikut terbaca", () => {
    render(<Probe />);
    act(() => {
      localStorage.setItem("erpora.dock", "pinned");
      window.dispatchEvent(new StorageEvent("storage", { key: "erpora.dock" }));
    });
    expect(screen.getByTestId("dock")).toHaveTextContent("pinned");
  });
});
