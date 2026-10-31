import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { initialsOf } from "../../lib/initials";
import { Avatar, AvatarFallback, AvatarImage } from "./avatar";

describe("Avatar", () => {
  test("gambar yang tidak termuat/gagal menampilkan inisial fallback", () => {
    render(
      <Avatar>
        <AvatarImage src="https://invalid.example/x.png" alt="Rahmad Maulana" />
        <AvatarFallback>{initialsOf("Rahmad Maulana")}</AvatarFallback>
      </Avatar>,
    );
    expect(screen.getByText("RM")).toBeInTheDocument();
    expect(screen.queryByRole("img")).toBeNull();
  });

  test("fallback tampil dan ukuran ditandai data-size (default, sm, lg)", () => {
    const { container, rerender } = render(
      <Avatar>
        <AvatarFallback>AB</AvatarFallback>
      </Avatar>,
    );
    const root = () => container.firstElementChild as HTMLElement;
    expect(screen.getByText("AB")).toBeInTheDocument();
    expect(root()).toHaveAttribute("data-slot", "avatar");
    expect(root()).toHaveAttribute("data-size", "default");
    rerender(
      <Avatar size="lg">
        <AvatarFallback>AB</AvatarFallback>
      </Avatar>,
    );
    expect(root()).toHaveAttribute("data-size", "lg");
  });
});
