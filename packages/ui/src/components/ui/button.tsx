import { cn } from "@erpora/ui/lib/utils";
import { cva, type VariantProps } from "class-variance-authority";
import { Slot } from "radix-ui";
import type * as React from "react";
import { LoadingIcon } from "../loading-icon";

// Hasil `shadcn add button`, diadaptasi ke ERP Design System (kartu DS Button; M3a K-24): varian primary/secondary/outline/
// ghost/destructive, ukuran sm/md/lg/icon dari token kontrol (28/34/40 px), `loading`, tanpa `outline-none` agar garis fokus
// global 2 px (token ring, offset 3 px) berlaku.
const buttonVariants = cva(
  "inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-sm font-medium whitespace-nowrap transition-colors disabled:pointer-events-none disabled:opacity-60 aria-busy:cursor-progress [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        primary: "bg-primary text-primary-foreground shadow-xs hover:bg-primary/90",
        secondary: "bg-secondary text-secondary-foreground shadow-xs hover:bg-secondary/80",
        outline:
          "border border-input bg-background shadow-xs hover:bg-accent hover:text-accent-foreground data-[state=open]:bg-accent",
        ghost: "hover:bg-accent hover:text-accent-foreground data-[state=open]:bg-accent",
        destructive: "bg-destructive text-destructive-foreground shadow-xs hover:bg-destructive/90",
      },
      size: {
        sm: "h-control-sm gap-1.5 px-3 text-xs [&_svg:not([class*='size-'])]:size-3.5",
        md: "h-control-md px-4",
        lg: "h-control-lg px-6",
        icon: "size-control-md p-0",
      },
    },
    defaultVariants: { variant: "primary", size: "md" },
  },
);

type ButtonProps = React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean;
    /** Menampilkan LoadingIcon, menandai `aria-busy`, dan menonaktifkan klik. */
    loading?: boolean;
  };

function Button({
  className,
  variant = "primary",
  size = "md",
  asChild = false,
  loading = false,
  disabled,
  type,
  children,
  ...props
}: ButtonProps) {
  if (
    size === "icon" &&
    !props["aria-label"] &&
    !props["aria-labelledby"] &&
    process.env.NODE_ENV !== "production"
  ) {
    console.warn('@erpora/ui Button size="icon" wajib memakai aria-label atau aria-labelledby');
  }
  const Comp = asChild ? Slot.Root : "button";
  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      type={asChild ? undefined : (type ?? "button")}
      aria-busy={loading || undefined}
      disabled={disabled || loading}
      className={cn(buttonVariants({ variant, size }), className)}
      {...props}
    >
      {asChild ? (
        children
      ) : (
        <>
          {loading ? <LoadingIcon /> : null}
          {children}
        </>
      )}
    </Comp>
  );
}

export { Button, buttonVariants };
