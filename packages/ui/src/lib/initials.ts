/** Inisial dari dua kata pertama nama (huruf besar); satu kata menjadi satu huruf; kosong menjadi "?". */
export function initialsOf(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean).slice(0, 2);
  const letters = words.map((w) => Array.from(w)[0] ?? "").join("");
  return letters === "" ? "?" : letters.toLocaleUpperCase();
}
