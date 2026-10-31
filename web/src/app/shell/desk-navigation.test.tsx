import { beforeEach, describe, expect, test } from "bun:test";
import type { Desk } from "@erpora/platform-web";
import { makeDashboardItem, makeDesk, makeDeskItem } from "@erpora/platform-web/testing";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useDeskNavigation } from "./desk-navigation";
import { renderShell } from "./shell-test-support";

const desks: Desk[] = [
  makeDesk("example", [
    makeDeskItem("example", "notes", "/example/notes"),
    makeDashboardItem("example", "example"),
  ]),
  makeDesk("billing"),
];

function Probe() {
  const { openDesk } = useDeskNavigation();
  return (
    <>
      {desks.map((d) => (
        <button key={d.id} type="button" onClick={() => openDesk(d)}>
          {`buka ${d.id}`}
        </button>
      ))}
    </>
  );
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("useDeskNavigation.openDesk", () => {
  test("membuka Dashboard terkunci walau bukan item pertama, dan menetapkan Desk saat ini", async () => {
    const user = userEvent.setup();
    const { router } = renderShell(<Probe />, { desks });
    await user.click(await screen.findByRole("button", { name: "buka example" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/example"));
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("example");
  });

  test("Desk tanpa menu: cadangan /<id>", async () => {
    const user = userEvent.setup();
    const { router } = renderShell(<Probe />, { desks });
    await user.click(await screen.findByRole("button", { name: "buka billing" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/billing"));
  });
});
