import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { Alert, AlertDescription, AlertTitle } from "./alert";

// Alert shadcn terbaru: varian default dan destructive; selalu role="alert". Varian DS lama (warning, info,
// success) dan role status dihapus bersama adaptasi; nada status memakai StatusBadge atau pemanggil.
describe("Alert", () => {
  test("destructive berperan alert dan memuat judul serta deskripsi", () => {
    render(
      <Alert variant="destructive">
        <AlertTitle>Gagal</AlertTitle>
        <AlertDescription>Periksa koneksi.</AlertDescription>
      </Alert>,
    );
    const a = screen.getByRole("alert");
    expect(a).toHaveTextContent("Gagal");
    expect(a).toHaveTextContent("Periksa koneksi.");
    expect(screen.getByText("Gagal")).toHaveAttribute("data-slot", "alert-title");
  });

  test("varian default berperan alert", () => {
    render(
      <Alert>
        <AlertTitle>Judul</AlertTitle>
      </Alert>,
    );
    expect(screen.getByRole("alert")).toHaveAttribute("data-slot", "alert");
  });

  test("role dapat ditimpa pemanggil", () => {
    render(
      <Alert role="status">
        <AlertTitle>t</AlertTitle>
      </Alert>,
    );
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  test("ikon (svg) anak dirender di dalam alert", () => {
    const { container } = render(
      <Alert>
        <svg />
        <AlertTitle>t</AlertTitle>
      </Alert>,
    );
    expect(container.querySelector("svg")).not.toBeNull();
  });
});
