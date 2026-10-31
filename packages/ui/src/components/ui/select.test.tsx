import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./select";

function setup(onValueChange?: (v: string) => void, defaultValue?: string) {
  return render(
    <Select defaultValue={defaultValue} onValueChange={onValueChange}>
      <SelectTrigger aria-label="Zona waktu">
        <SelectValue placeholder="Pilih zona" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="wib">WIB</SelectItem>
        <SelectItem value="wita">WITA</SelectItem>
      </SelectContent>
    </Select>,
  );
}

describe("Select", () => {
  test("trigger berperan combobox dan menampilkan placeholder", () => {
    setup();
    const t = screen.getByRole("combobox", { name: "Zona waktu" });
    expect(t).toHaveAttribute("data-slot", "select-trigger");
    expect(t).toHaveTextContent("Pilih zona");
  });

  test("nilai awal ditampilkan", () => {
    setup(undefined, "wita");
    expect(screen.getByRole("combobox")).toHaveTextContent("WITA");
  });

  test("keyboard: Enter membuka daftar, panah dan Enter memilih opsi", async () => {
    const user = userEvent.setup();
    const seen: string[] = [];
    setup((v) => seen.push(v));
    screen.getByRole("combobox").focus();
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("listbox")).toBeInTheDocument();
    await user.keyboard("{ArrowDown}{Enter}");
    expect(seen.length).toBe(1);
  });
});
