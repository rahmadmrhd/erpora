import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Label } from "./label";

describe("Label", () => {
  test("terhubung ke kontrol lewat htmlFor: nama aksesibel dan fokus saat diklik", async () => {
    const user = userEvent.setup();
    render(
      <>
        <Label htmlFor="nama">Nama</Label>
        <input id="nama" />
      </>,
    );
    const input = screen.getByLabelText("Nama");
    await user.click(screen.getByText("Nama"));
    expect(input).toHaveFocus();
  });

  test("className digabung dan teks sm medium", () => {
    render(<Label className="extra">Teks</Label>);
    const l = screen.getByText("Teks");
    expect(l.className).toContain("extra");
    expect(l.className).toContain("font-medium");
  });
});
