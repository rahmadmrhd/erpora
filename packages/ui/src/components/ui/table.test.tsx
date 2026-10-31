import { describe, expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./table";

function Sample() {
  return (
    <Table>
      <TableCaption>Daftar modul</TableCaption>
      <TableHeader>
        <TableRow>
          <TableHead>Modul</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow>
          <TableCell>example</TableCell>
          <TableCell>aktif</TableCell>
        </TableRow>
      </TableBody>
    </Table>
  );
}

describe("Table", () => {
  test("berperan table bernama dari caption", () => {
    render(<Sample />);
    expect(screen.getByRole("table", { name: "Daftar modul" })).toBeInTheDocument();
  });

  test("header berperan columnheader", () => {
    render(<Sample />);
    const headers = screen.getAllByRole("columnheader");
    expect(headers.map((h) => h.textContent)).toEqual(["Modul", "Status"]);
  });

  test("baris dan sel", () => {
    render(<Sample />);
    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(2);
    expect(
      within(rows[1] as HTMLElement)
        .getAllByRole("cell")
        .map((c) => c.textContent),
    ).toEqual(["example", "aktif"]);
  });
});
