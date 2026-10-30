import { cn } from "@erpora/ui/lib/utils";
import { XIcon } from "lucide-react";
import { Dialog as DialogPrimitive } from "radix-ui";
import type * as React from "react";
import { useUiLabel } from "../../labels";
import { Button } from "./button";

// Hasil `shadcn add dialog`, diadaptasi ke kartu DS Dialog (K-24): overlay token `overlay`, permukaan `background`, lebar 512 px
// (`size="default"`) atau 896 px (`size="wide"`), `align` (top/center/bottom) dan `forceAsDialog` (bawaan: di bawah breakpoint sm
// setinggi layar), `hideX`. Tombol tutup berlabel `actions.close` dari UiLabels (bukan teks Inggris tertanam).
function Dialog({ ...props }: React.ComponentProps<typeof DialogPrimitive.Root>) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />;
}

function DialogTrigger({ ...props }: React.ComponentProps<typeof DialogPrimitive.Trigger>) {
  return <DialogPrimitive.Trigger data-slot="dialog-trigger" {...props} />;
}

function DialogPortal({ ...props }: React.ComponentProps<typeof DialogPrimitive.Portal>) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />;
}

function DialogClose({ ...props }: React.ComponentProps<typeof DialogPrimitive.Close>) {
  return <DialogPrimitive.Close data-slot="dialog-close" {...props} />;
}

function DialogOverlay({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      data-slot="dialog-overlay"
      className={cn(
        "fixed inset-0 z-50 bg-overlay data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0 motion-reduce:animate-none",
        className,
      )}
      {...props}
    />
  );
}

type DialogContentProps = React.ComponentProps<typeof DialogPrimitive.Content> & {
  /** Sembunyikan tombol tutup (x). */
  hideX?: boolean;
  /** `default` 512 px, `wide` 896 px. */
  size?: "default" | "wide";
  /** Posisi vertikal di layar >= sm. */
  align?: "top" | "center" | "bottom";
  /** false (bawaan): di bawah sm dialog setinggi layar; true: tinggi mengikuti isi di semua ukuran. */
  forceAsDialog?: boolean;
};

function DialogContent({
  className,
  children,
  hideX = false,
  size = "default",
  align = "top",
  forceAsDialog = false,
  ...props
}: DialogContentProps) {
  const t = useUiLabel();
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        data-slot="dialog-content"
        data-align={align}
        className={cn(
          "fixed left-1/2 z-50 grid w-full max-w-[calc(100%-2rem)] -translate-x-1/2 gap-4 border bg-background p-6 shadow-lg duration-200 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 motion-reduce:animate-none sm:rounded-lg",
          size === "wide" ? "sm:max-w-4xl" : "sm:max-w-lg",
          align === "top" && "top-[8%]",
          align === "center" && "top-1/2 -translate-y-1/2",
          align === "bottom" && "bottom-4 top-auto",
          !forceAsDialog &&
            "max-sm:top-0 max-sm:h-dvh max-sm:max-w-none max-sm:translate-y-0 max-sm:content-start max-sm:overflow-y-auto",
          className,
        )}
        {...props}
      >
        {children}
        {hideX ? null : (
          <DialogPrimitive.Close
            data-slot="dialog-close"
            className="absolute top-4 right-4 rounded-xs opacity-70 transition-opacity hover:opacity-100 disabled:pointer-events-none data-[state=open]:bg-accent data-[state=open]:text-muted-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4"
          >
            <XIcon />
            <span className="sr-only">{t("actions.close")}</span>
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Content>
    </DialogPortal>
  );
}

function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="dialog-header"
      className={cn("flex flex-col gap-2 text-center sm:text-left", className)}
      {...props}
    />
  );
}

function DialogFooter({
  className,
  showCloseButton = false,
  children,
  ...props
}: React.ComponentProps<"div"> & { showCloseButton?: boolean }) {
  const t = useUiLabel();
  return (
    <div
      data-slot="dialog-footer"
      className={cn("flex flex-col-reverse gap-2 sm:flex-row sm:justify-end", className)}
      {...props}
    >
      {children}
      {showCloseButton ? (
        <DialogPrimitive.Close asChild>
          <Button variant="outline">{t("actions.close")}</Button>
        </DialogPrimitive.Close>
      ) : null}
    </div>
  );
}

function DialogTitle({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      data-slot="dialog-title"
      className={cn("text-lg leading-none font-semibold", className)}
      {...props}
    />
  );
}

function DialogDescription({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Description>) {
  return (
    <DialogPrimitive.Description
      data-slot="dialog-description"
      className={cn("text-sm text-muted-foreground", className)}
      {...props}
    />
  );
}

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
};
