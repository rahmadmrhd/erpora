import { cn } from "@erpora/ui/lib/utils";
import type * as React from "react";

// Hasil `shadcn add skeleton`, diadaptasi: latar `muted` (DS), denyut dimatikan pada prefers-reduced-motion (R14.1),
// dan disembunyikan dari pembaca layar.
function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="skeleton"
      aria-hidden="true"
      className={cn("animate-pulse rounded-md bg-muted motion-reduce:animate-none", className)}
      {...props}
    />
  );
}

export { Skeleton };
