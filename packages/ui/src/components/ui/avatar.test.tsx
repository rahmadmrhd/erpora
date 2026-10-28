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

  test("lingkaran 40 px bawaan; fallback berlatar muted", () => {
    const { container } = render(
      <Avatar>
        <AvatarFallback>AB</AvatarFallback>
      </Avatar>,
    );
    const root = container.firstElementChild as HTMLElement;
    expect(root.className).toContain("size-10");
    expect(root.className).toContain("rounded-full");
    expect(screen.getByText("AB").className).toContain("bg-muted");
  });

  test("ukuran dapat diganti lewat className (mis. 32 px di footer sidebar)", () => {
    const { container } = render(
      <Avatar className="size-8">
        <AvatarFallback>AB</AvatarFallback>
      </Avatar>,
    );
    const cls = (container.firstElementChild as HTMLElement).className;
    expect(cls).toContain("size-8");
    expect(cls).not.toContain("size-10");
  });
});
