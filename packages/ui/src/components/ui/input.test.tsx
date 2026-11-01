import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Input } from "./input";

describe("Input", () => {
  test("berperan textbox dan menerima ketikan", async () => {
    const user = userEvent.setup();
    render(<Input aria-label="Nama" />);
    const i = screen.getByRole("textbox", { name: "Nama" });
    await user.type(i, "Budi");
    expect(i).toHaveValue("Budi");
    expect(i).toHaveAttribute("data-slot", "input");
  });

  test("type dan disabled diteruskan", () => {
    render(<Input type="password" aria-label="Sandi" disabled />);
    const i = screen.getByLabelText("Sandi");
    expect(i).toHaveAttribute("type", "password");
    expect(i).toBeDisabled();
  });

  test("aria-invalid diteruskan untuk gaya galat", () => {
    render(<Input aria-label="x" aria-invalid />);
    expect(screen.getByRole("textbox")).toHaveAttribute("aria-invalid", "true");
  });
});
