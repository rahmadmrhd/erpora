import { Tooltip, TooltipContent, TooltipTrigger } from "@erpora/ui";
import type { ReactNode } from "react";

/** Item yang dinonaktifkan dengan penjelasan: tooltip untuk penglihat; deskripsi lewat `aria-describedby` di pemanggil. */
export function DisabledHint({
  hint,
  side = "right",
  children,
}: {
  hint: string;
  side?: "top" | "right" | "bottom" | "left";
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="block">{children}</span>
      </TooltipTrigger>
      <TooltipContent side={side}>{hint}</TooltipContent>
    </Tooltip>
  );
}
