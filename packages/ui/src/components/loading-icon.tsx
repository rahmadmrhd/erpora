import { cn } from "@erpora/ui/lib/utils";
import type * as React from "react";

const DOTS = 8;
const RADIUS = 10;

/**
 * Penanda memuat: delapan titik yang membesar bergiliran melingkar (kartu DS LoadingIcon). Animasi memakai CSS keyframes
 * (`erpora-loading-dot` di theme.css), bukan SMIL, sehingga diam saat `prefers-reduced-motion`. Warna mengikuti `currentColor`.
 * Dengan `aria-label` berperan `img`; tanpa label disembunyikan dari pembaca layar (pemanggil menyertakan teks "Memuat ...").
 */
export function LoadingIcon({
  className,
  "aria-label": ariaLabel,
  ...props
}: Omit<React.ComponentProps<"svg">, "children">) {
  return (
    // biome-ignore lint/a11y/noSvgWithoutTitle: nama aksesibel lewat aria-label (role=img) atau disembunyikan dari pembaca layar
    <svg
      viewBox="0 0 24 24"
      xmlns="http://www.w3.org/2000/svg"
      data-slot="loading-icon"
      className={cn("size-4", className)}
      {...(ariaLabel ? { role: "img", "aria-label": ariaLabel } : { "aria-hidden": true })}
      {...props}
    >
      {Array.from({ length: DOTS }, (_, i) => {
        const angle = (i * 2 * Math.PI) / DOTS;
        return (
          <circle
            key={i}
            cx={12 + RADIUS * Math.sin(angle)}
            cy={12 - RADIUS * Math.cos(angle)}
            r="2"
            fill="currentColor"
            className="erpora-loading-dot"
            style={{ animationDelay: `${(i * 0.125).toFixed(3)}s` }}
          />
        );
      })}
    </svg>
  );
}
