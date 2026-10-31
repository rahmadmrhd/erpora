import { afterEach, describe, expect, test } from "bun:test";
import { act, render, screen } from "@testing-library/react";
import { writeTimezone } from "../prefs";
import { useDateTimeFormatter } from "./hooks";

afterEach(() => {
  localStorage.clear();
});

const LATE = "2026-10-15T23:30:00Z";

function Probe() {
  const format = useDateTimeFormatter();
  return <span data-testid="out">{format(LATE, "en")}</span>;
}

describe("useDateTimeFormatter", () => {
  test("memformat dengan zona preferensi dan berubah seketika saat zona diganti", () => {
    writeTimezone("America/New_York");
    render(<Probe />);
    expect(screen.getByTestId("out").textContent).toContain("Oct 15");
    act(() => writeTimezone("Asia/Jakarta"));
    expect(screen.getByTestId("out").textContent).toContain("Oct 16");
    act(() => writeTimezone(null));
    expect(screen.getByTestId("out").textContent?.length).toBeGreaterThan(0);
  });
});
