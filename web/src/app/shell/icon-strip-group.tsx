import { Link } from "@erpora/module-sdk";
import { type DeskMenuGroup, Message } from "@erpora/platform-web";
import { Popover, PopoverContent, PopoverTrigger, SidebarMenuItem } from "@erpora/ui";
import type { ReactNode } from "react";

const trim = (p: string): string => (p.length > 1 && p.endsWith("/") ? p.slice(0, -1) : p);

/**
 * Grup menu pada strip ikon (Floating terciut): anak tampil di popover di samping ikon grup. Dimuat malas oleh
 * `DeskPanel` (Popover dan Popper hanya dibutuhkan setelah panel diciutkan) agar tidak membebani bundel awal.
 */
export function IconStripGroup({
  node,
  pathname,
  trigger,
}: {
  node: DeskMenuGroup;
  pathname: string;
  trigger: ReactNode;
}) {
  return (
    <SidebarMenuItem>
      <Popover>
        <PopoverTrigger asChild>{trigger}</PopoverTrigger>
        <PopoverContent side="right" align="start" className="w-48 p-1">
          <ul className="flex flex-col gap-0.5">
            {node.children.map((c) => (
              <li key={c.id}>
                <Link
                  to={c.to}
                  activeOptions={{ exact: true }}
                  aria-current={trim(pathname) === trim(c.to) ? "page" : undefined}
                  className="flex h-8 items-center rounded-md px-2 text-sm hover:bg-accent aria-[current=page]:bg-accent aria-[current=page]:font-medium"
                >
                  <Message message={c.label} />
                </Link>
              </li>
            ))}
          </ul>
        </PopoverContent>
      </Popover>
    </SidebarMenuItem>
  );
}
