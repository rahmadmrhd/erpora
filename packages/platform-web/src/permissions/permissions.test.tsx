import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { StaticBootstrapProvider } from "../bootstrap";
import { makeBootstrap } from "../bootstrap/testing";
import { usePermission } from "./index";

function Probe({ permission }: { permission: string }) {
  return <span data-testid="out">{String(usePermission(permission))}</span>;
}

describe("usePermission (UX saja; backend tetap otoritas)", () => {
  test("permissions kosong berarti belum ada informasi: optimistis true", () => {
    render(
      <StaticBootstrapProvider value={makeBootstrap({ permissions: [] })}>
        <Probe permission="platform.modules.manage" />
      </StaticBootstrapProvider>,
    );
    expect(screen.getByTestId("out").textContent).toBe("true");
  });

  test("permissions berisi: hanya yang tercantum yang true", () => {
    const value = makeBootstrap({ permissions: ["platform.modules.read"] });
    const { rerender } = render(
      <StaticBootstrapProvider value={value}>
        <Probe permission="platform.modules.read" />
      </StaticBootstrapProvider>,
    );
    expect(screen.getByTestId("out").textContent).toBe("true");
    rerender(
      <StaticBootstrapProvider value={value}>
        <Probe permission="platform.modules.manage" />
      </StaticBootstrapProvider>,
    );
    expect(screen.getByTestId("out").textContent).toBe("false");
  });

  test("tanpa kecocokan awalan atau wildcard (pencocokan persis)", () => {
    render(
      <StaticBootstrapProvider value={makeBootstrap({ permissions: ["platform.modules.read"] })}>
        <Probe permission="platform.modules" />
      </StaticBootstrapProvider>,
    );
    expect(screen.getByTestId("out").textContent).toBe("false");
  });
});
