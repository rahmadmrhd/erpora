import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "./card";

describe("Card", () => {
  test("komposisi header, judul, deskripsi, konten, dan footer", () => {
    render(
      <Card>
        <CardHeader>
          <CardTitle>Penjualan</CardTitle>
          <CardDescription>Ringkasan bulan ini</CardDescription>
        </CardHeader>
        <CardContent>Isi</CardContent>
        <CardFooter>Kaki</CardFooter>
      </Card>,
    );
    expect(screen.getByText("Penjualan")).toHaveAttribute("data-slot", "card-title");
    expect(screen.getByText("Ringkasan bulan ini")).toHaveAttribute(
      "data-slot",
      "card-description",
    );
    expect(screen.getByText("Isi")).toHaveAttribute("data-slot", "card-content");
    expect(screen.getByText("Kaki")).toHaveAttribute("data-slot", "card-footer");
  });

  test("className pemanggil digabung pada akar", () => {
    const { container } = render(<Card className="w-80">x</Card>);
    expect(container.firstElementChild).toHaveAttribute("data-slot", "card");
    expect(container.firstElementChild?.className).toContain("w-80");
  });
});
