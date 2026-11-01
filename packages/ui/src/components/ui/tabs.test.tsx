import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./tabs";

function setup(onValueChange?: (v: string) => void) {
  return render(
    <Tabs defaultValue="semua" onValueChange={onValueChange}>
      <TabsList>
        <TabsTrigger value="semua">Semua</TabsTrigger>
        <TabsTrigger value="peristiwa">Peristiwa</TabsTrigger>
      </TabsList>
      <TabsContent value="semua">Panel semua</TabsContent>
      <TabsContent value="peristiwa">Panel peristiwa</TabsContent>
    </Tabs>,
  );
}

describe("Tabs", () => {
  test("peran tablist, tab, dan tabpanel; tab awal terpilih", () => {
    setup();
    expect(screen.getByRole("tablist")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Semua" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Peristiwa" })).toHaveAttribute(
      "aria-selected",
      "false",
    );
    expect(screen.getByRole("tabpanel")).toHaveTextContent("Panel semua");
  });

  test("klik tab mengganti panel dan memanggil onValueChange", async () => {
    const user = userEvent.setup();
    const seen: string[] = [];
    setup((v) => seen.push(v));
    await user.click(screen.getByRole("tab", { name: "Peristiwa" }));
    expect(screen.getByRole("tabpanel")).toHaveTextContent("Panel peristiwa");
    expect(seen).toEqual(["peristiwa"]);
  });

  test("keyboard: panah kanan memindahkan fokus dan pilihan (Radix)", async () => {
    const user = userEvent.setup();
    setup();
    screen.getByRole("tab", { name: "Semua" }).focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "Peristiwa" })).toHaveFocus();
  });
});
