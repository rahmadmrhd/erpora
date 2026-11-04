import { useMemo } from "react";
import { encode } from "uqr";

const QUIET_ZONE = 4;

/** Satu `<path>` dari matriks modul gelap: satu persegi panjang per deretan modul berurutan di tiap baris. */
export function qrPath(matrix: readonly (readonly boolean[])[]): string {
  const parts: string[] = [];
  matrix.forEach((row, y) => {
    let x = 0;
    while (x < row.length) {
      if (!row[x]) {
        x++;
        continue;
      }
      const start = x;
      while (x < row.length && row[x]) x++;
      parts.push(`M${start} ${y}h${x - start}v1h-${x - start}z`);
    }
  });
  return parts.join("");
}

/**
 * Kode QR sebagai elemen React `<svg>` (bukan string HTML; tanpa `dangerouslySetInnerHTML`). Dimuat malas hanya di
 * layar pendaftaran MFA. Warna sengaja hitam di atas putih pada semua tema: pemindai membutuhkan kontras gelap-di-atas-
 * terang (ISO/IEC 18004), jadi bukan token tema.
 */
export default function QrCode({ text, label }: { text: string; label: string }) {
  const { size, path } = useMemo(() => {
    const qr = encode(text, { ecc: "M" });
    return { size: qr.size, path: qrPath(qr.data) };
  }, [text]);
  const side = size + QUIET_ZONE * 2;
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${side} ${side}`}
      shapeRendering="crispEdges"
      className="mx-auto size-48 rounded-md"
    >
      <rect width={side} height={side} fill="white" />
      <path d={path} fill="black" transform={`translate(${QUIET_ZONE} ${QUIET_ZONE})`} />
    </svg>
  );
}
