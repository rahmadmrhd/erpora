import { Message, useModuleRegistry } from "@erpora/platform-web";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarHeader,
  SidebarMenu,
} from "@erpora/ui";
import { useRouterState } from "@tanstack/react-router";
import { NavMenu } from "./nav-menu";
import { NotificationsMenuItem } from "./notifications";
import { UserMenu } from "./user-menu";

/**
 * Sidebar (K-15/mockup): atas ke bawah logo dan nama aplikasi, item Notifikasi, menu dari registry, dan avatar akun
 * di dasar. Tiga keadaan (diperluas, terlipat ke ikon, Sheet di layar kecil) dari komponen `Sidebar` `@erpora/ui`.
 */
export function AppSidebar() {
  const { navigation } = useModuleRegistry();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  return (
    <Sidebar collapsible="icon" side="left">
      <SidebarHeader>
        <div className="flex h-8 items-center gap-2 px-1">
          <div
            aria-hidden="true"
            className="flex size-7 shrink-0 items-center justify-center rounded-md bg-sidebar-primary text-sm font-semibold text-sidebar-primary-foreground"
          >
            E
          </div>
          <span className="truncate font-semibold group-data-[collapsible=icon]:hidden">
            <Message message={{ ns: "platform", key: "app.name" }} />
          </span>
        </div>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarMenu>
            <NotificationsMenuItem />
          </SidebarMenu>
        </SidebarGroup>
        <NavMenu moduleNodes={navigation} pathname={pathname} />
      </SidebarContent>
      <SidebarFooter>
        <UserMenu />
      </SidebarFooter>
    </Sidebar>
  );
}
