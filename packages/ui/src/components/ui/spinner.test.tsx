import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { UiLabelsProvider } from "../../labels";
import { Spinner } from "./spinner";

describe("Spinner", () => {
  test("berperan status dengan nama dari UiLabels (status.loading), bukan teks Inggris", () => {
    render(
      <UiLabelsProvider t={(k) => (k === "status.loading" ? "Memuat…" : k)}>
        <Spinner />
      </UiLabelsProvider>,
    );
    expect(screen.getByRole("status", { name: "Memuat…" })).toBeInTheDocument();
  });

  test("tanpa provider nama aksesibel adalah kunci (tidak pernah 'Loading' tertanam)", () => {
    render(<Spinner />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "status.loading");
  });

  test("className pemanggil digabung", () => {
    render(<Spinner className="size-8" />);
    expect(screen.getByRole("status").getAttribute("class")).toContain("size-8");
  });
});
