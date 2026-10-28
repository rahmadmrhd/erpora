import { cn } from "@erpora/ui/lib/utils";
import { Avatar as AvatarPrimitive } from "radix-ui";
import type * as React from "react";

// Hasil `shadcn add avatar`, diadaptasi ke kartu DS Avatar: lingkaran 40 px bawaan (sm 32 px footer sidebar, lg 80 px profil),
// gambar `object-contain`, fallback inisial (lihat initialsOf) di atas latar `muted`. AvatarBadge/AvatarGroup shadcn tidak dipakai (dibuang).
function Avatar({
  className,
  size = "default",
  ...props
}: React.ComponentProps<typeof AvatarPrimitive.Root> & { size?: "sm" | "default" | "lg" }) {
  return (
    <AvatarPrimitive.Root
      data-slot="avatar"
      data-size={size}
      className={cn(
        "group/avatar relative flex size-10 shrink-0 overflow-hidden rounded-full select-none data-[size=lg]:size-20 data-[size=sm]:size-8",
        className,
      )}
      {...props}
    />
  );
}

function AvatarImage({ className, ...props }: React.ComponentProps<typeof AvatarPrimitive.Image>) {
  return (
    <AvatarPrimitive.Image
      data-slot="avatar-image"
      className={cn("aspect-square size-full object-contain", className)}
      {...props}
    />
  );
}

function AvatarFallback({
  className,
  ...props
}: React.ComponentProps<typeof AvatarPrimitive.Fallback>) {
  return (
    <AvatarPrimitive.Fallback
      data-slot="avatar-fallback"
      className={cn(
        "flex size-full items-center justify-center rounded-full bg-muted text-xl font-semibold text-muted-foreground group-data-[size=lg]/avatar:text-4xl group-data-[size=sm]/avatar:text-xs",
        className,
      )}
      {...props}
    />
  );
}

export { Avatar, AvatarFallback, AvatarImage };
