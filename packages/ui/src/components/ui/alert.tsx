import { cn } from "@erpora/ui/lib/utils";
import { cva, type VariantProps } from "class-variance-authority";
import type * as React from "react";

// Hasil `shadcn add alert`, diadaptasi ke kartu DS Alert (M3a K-14): varian default/destructive/warning/info/success, hanya tampilan
// outline (solid DS gagal kontras). Teks memakai `foreground`; hue hanya pada border dan ikon. Peran ARIA: `alert` untuk
// destructive (menyela), `status` untuk selebihnya (sopan); dapat ditimpa lewat `role`.
const alertVariants = cva(
  "relative grid w-full grid-cols-[0_1fr] items-start gap-y-0.5 rounded-lg border bg-card px-4 py-3 text-sm text-foreground has-[>svg]:grid-cols-[calc(var(--spacing)*4)_1fr] has-[>svg]:gap-x-3 [&>svg]:size-4 [&>svg]:translate-y-0.5",
  {
    variants: {
      variant: {
        default: "border-border [&>svg]:text-muted-foreground",
        destructive: "border-destructive [&>svg]:text-destructive",
        warning: "border-warning [&>svg]:text-warning",
        info: "border-info [&>svg]:text-info",
        success: "border-success [&>svg]:text-success",
      },
    },
    defaultVariants: { variant: "default" },
  },
);

function Alert({
  className,
  variant = "default",
  role,
  ...props
}: React.ComponentProps<"div"> & VariantProps<typeof alertVariants>) {
  return (
    <div
      data-slot="alert"
      data-variant={variant}
      role={role ?? (variant === "destructive" ? "alert" : "status")}
      className={cn(alertVariants({ variant }), className)}
      {...props}
    />
  );
}

function AlertTitle({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-title"
      className={cn("col-start-2 line-clamp-1 min-h-4 font-medium tracking-tight", className)}
      {...props}
    />
  );
}

function AlertDescription({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-description"
      className={cn(
        "col-start-2 grid justify-items-start gap-1 text-sm text-muted-foreground [&_p]:leading-relaxed",
        className,
      )}
      {...props}
    />
  );
}

export { Alert, AlertDescription, AlertTitle };
