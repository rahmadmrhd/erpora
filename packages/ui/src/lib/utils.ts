import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

/** Menggabungkan kelas kondisional dan menyelesaikan konflik utilitas Tailwind (yang terakhir menang). */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
