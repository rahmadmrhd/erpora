import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { Checkbox } from "./checkbox";
import { Label } from "./label";

function Controlled({ onChange }: { onChange?: (v: boolean | "indeterminate") => void }) {
  const [v, setV] = useState<boolean | "indeterminate">(false);
  return (
    <>
      <Checkbox
        id="c"
        checked={v}
        onCheckedChange={(next) => {
          setV(next);
          onChange?.(next);
        }}
      />
      <Label htmlFor="c">Sertakan dependensi</Label>
    </>
  );
}

describe("Checkbox", () => {
  test("berperan checkbox, bernama dari label, aria-checked mengikuti keadaan", async () => {
    const user = userEvent.setup();
    render(<Controlled />);
    const box = screen.getByRole("checkbox", { name: "Sertakan dependensi" });
    expect(box).toHaveAttribute("aria-checked", "false");
    await user.click(box);
    expect(box).toHaveAttribute("aria-checked", "true");
  });

  test("Spasi mengubah keadaan dan memanggil onCheckedChange", async () => {
    const user = userEvent.setup();
    const seen: Array<boolean | "indeterminate"> = [];
    render(<Controlled onChange={(v) => seen.push(v)} />);
    screen.getByRole("checkbox").focus();
    await user.keyboard(" ");
    await user.keyboard(" ");
    expect(seen).toEqual([true, false]);
  });

  test("klik pada label mengubah keadaan", async () => {
    const user = userEvent.setup();
    render(<Controlled />);
    await user.click(screen.getByText("Sertakan dependensi"));
    expect(screen.getByRole("checkbox")).toHaveAttribute("aria-checked", "true");
  });

  test("indeterminate dilaporkan sebagai aria-checked=mixed", () => {
    render(<Checkbox checked="indeterminate" aria-label="Semua" />);
    expect(screen.getByRole("checkbox", { name: "Semua" })).toHaveAttribute(
      "aria-checked",
      "mixed",
    );
  });

  test("disabled tidak berubah", async () => {
    const user = userEvent.setup();
    render(<Checkbox disabled aria-label="x" />);
    await user.click(screen.getByRole("checkbox"));
    expect(screen.getByRole("checkbox")).toHaveAttribute("aria-checked", "false");
    expect(screen.getByRole("checkbox")).toBeDisabled();
  });

  test("kotak 16 px bertoken primary saat dicentang", () => {
    render(<Checkbox checked aria-label="x" />);
    const cls = screen.getByRole("checkbox").className;
    expect(cls).toContain("size-4");
    expect(cls).toContain("data-[state=checked]:bg-primary");
  });
});
