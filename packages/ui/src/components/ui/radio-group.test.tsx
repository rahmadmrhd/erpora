import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RadioGroup, RadioGroupItem } from "./radio-group";

function setup(onValueChange?: (v: string) => void) {
  return render(
    <RadioGroup defaultValue="terang" aria-label="Tema" onValueChange={onValueChange}>
      <RadioGroupItem value="terang" aria-label="Terang" />
      <RadioGroupItem value="gelap" aria-label="Gelap" />
    </RadioGroup>,
  );
}

describe("RadioGroup", () => {
  test("peran radiogroup dan radio; nilai awal terpilih", () => {
    setup();
    expect(screen.getByRole("radiogroup", { name: "Tema" })).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "Terang" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "Gelap" })).not.toBeChecked();
  });

  test("klik memilih opsi dan memanggil onValueChange", async () => {
    const user = userEvent.setup();
    const seen: string[] = [];
    setup((v) => seen.push(v));
    await user.click(screen.getByRole("radio", { name: "Gelap" }));
    expect(screen.getByRole("radio", { name: "Gelap" })).toBeChecked();
    expect(seen).toEqual(["gelap"]);
  });

  test("keyboard: panah bawah memindahkan pilihan (Radix)", async () => {
    const user = userEvent.setup();
    setup();
    screen.getByRole("radio", { name: "Terang" }).focus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("radio", { name: "Gelap" })).toBeChecked();
  });
});
