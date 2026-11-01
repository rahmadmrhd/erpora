import { afterEach, describe, expect, mock, test } from "bun:test";
import { fireEvent, render } from "@testing-library/react";
import { useGlobalShortcuts } from "./global-shortcuts";

function Probe(props: { toggleTheme: () => void; showShortcuts: () => void }) {
  useGlobalShortcuts(props);
  return (
    <>
      <input aria-label="isian" />
      <textarea aria-label="area" />
      {/* biome-ignore lint/a11y/useSemanticElements: elemen contenteditable memang diuji (editor Tiptap kelak) */}
      <div
        contentEditable
        role="textbox"
        tabIndex={0}
        aria-label="editor"
        suppressContentEditableWarning
      />
    </>
  );
}

afterEach(() => localStorage.clear());

function setup() {
  const toggleTheme = mock(() => {});
  const showShortcuts = mock(() => {});
  const view = render(<Probe toggleTheme={toggleTheme} showShortcuts={showShortcuts} />);
  return { toggleTheme, showShortcuts, ...view };
}

const slash = { key: "?", code: "Slash", shiftKey: true };

describe("pintasan global shell", () => {
  test("Ctrl+Shift+L mengganti tema, juga saat fokus di kolom isian", () => {
    const { toggleTheme, getByLabelText } = setup();
    fireEvent.keyDown(window, { key: "L", code: "KeyL", ctrlKey: true, shiftKey: true });
    fireEvent.keyDown(getByLabelText("isian"), {
      key: "L",
      code: "KeyL",
      ctrlKey: true,
      shiftKey: true,
    });
    expect(toggleTheme).toHaveBeenCalledTimes(2);
  });

  test("Shift+/ membuka daftar pintasan di luar kolom isian", () => {
    const { showShortcuts } = setup();
    fireEvent.keyDown(document.body, slash);
    expect(showShortcuts).toHaveBeenCalledTimes(1);
  });

  test("Shift+/ diabaikan saat fokus di input, textarea, atau contenteditable", () => {
    const { showShortcuts, getByLabelText } = setup();
    for (const label of ["isian", "area", "editor"]) {
      fireEvent.keyDown(getByLabelText(label), slash);
    }
    expect(showShortcuts).not.toHaveBeenCalled();
  });

  test("Ctrl+Shift+/ (Dock) dan kombinasi lain tidak memicu Shift+/", () => {
    const { showShortcuts, toggleTheme } = setup();
    fireEvent.keyDown(window, { ...slash, ctrlKey: true });
    fireEvent.keyDown(window, { key: "L", code: "KeyL", shiftKey: true });
    fireEvent.keyDown(window, { key: "l", code: "KeyL", ctrlKey: true });
    expect(showShortcuts).not.toHaveBeenCalled();
    expect(toggleTheme).not.toHaveBeenCalled();
  });
});
