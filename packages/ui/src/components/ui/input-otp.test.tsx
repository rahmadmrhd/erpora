import { describe, expect, mock, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { InputOTP, InputOTPGroup, InputOTPSlot } from "./input-otp";

function Six({ onComplete }: { onComplete?: (v: string) => void }) {
  return (
    <InputOTP maxLength={6} aria-label="Kode" onComplete={onComplete}>
      <InputOTPGroup>
        {[0, 1, 2, 3, 4, 5].map((i) => (
          <InputOTPSlot key={i} index={i} />
        ))}
      </InputOTPGroup>
    </InputOTP>
  );
}

describe("InputOTP", () => {
  test("menerima ketikan, menampilkan karakter per slot, dan memanggil onComplete saat penuh", async () => {
    const user = userEvent.setup();
    const onComplete = mock((_v: string) => {});
    render(<Six onComplete={onComplete} />);
    const input = screen.getByLabelText("Kode");
    expect(input).toHaveAttribute("data-slot", "input-otp");
    await user.type(input, "123456");
    expect(input).toHaveValue("123456");
    expect(onComplete).toHaveBeenCalledWith("123456");
    expect(document.querySelectorAll('[data-slot="input-otp-slot"]')).toHaveLength(6);
  });

  test("tidak menerima lebih dari maxLength", async () => {
    const user = userEvent.setup();
    render(<Six />);
    const input = screen.getByLabelText("Kode");
    await user.type(input, "12345678");
    expect(input).toHaveValue("123456");
  });
});
