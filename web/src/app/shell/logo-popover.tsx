import type { Desk } from "@erpora/platform-web";
import {
  Message,
  useBootstrap,
  useCurrentDesk,
  useDesks,
  useMessage,
  useTranslation,
} from "@erpora/platform-web";
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
  resolveIcon,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@erpora/ui";
import { useId } from "react";
import { useDeskNavigation } from "./desk-navigation";
import { DisabledHint } from "./disabled-hint";
import { DOCK_BUTTON_CLASS } from "./dock-sizing";
import { useShellActions } from "./shell-context";

function Icon({ name }: { name: string }) {
  const C = resolveIcon(name);
  return C ? <C aria-hidden="true" /> : null;
}

function DeskRadioItem({ desk }: { desk: Desk }) {
  const text = useMessage(desk.label);
  return <DropdownMenuRadioItem value={desk.id}>{text}</DropdownMenuRadioItem>;
}

/** Penanda logo ERPora (huruf "E" pada kotak berwarna primer sidebar). */
function Mark() {
  return (
    <span
      aria-hidden="true"
      className="flex size-8 shrink-0 items-center justify-center rounded-md bg-sidebar-primary text-sm font-semibold text-sidebar-primary-foreground"
    >
      E
    </span>
  );
}

/**
 * Logo dan nama proyek yang membuka popover tingkat aplikasi (R6): Pindah Desk, Semua Desk, Edit sidebar
 * (nonaktif sampai M3b), Bantuan dan pintasan, Tentang (versi dari bootstrap). `dock`: hanya logo di atas Dock
 * Pinned; `panel`: logo dan nama sebagai kepala panel (Floating dan Sheet); `navbar`: logo dan nama di kiri navbar
 * halaman Desks (K1, tanpa Dock/panel) tanpa "Pindah Desk" dan "Semua Desk" (K18, R6.6: pengguna sudah di halaman Desks).
 * Selain itu popover sama pada semua varian.
 */
export function LogoPopover({ variant }: { variant: "dock" | "panel" | "navbar" }) {
  const { t } = useTranslation("platform");
  const { appVersion } = useBootstrap();
  const { desks } = useDesks();
  const { deskId } = useCurrentDesk();
  const { switchDesk, goToDesks } = useDeskNavigation();
  const actions = useShellActions();
  const hintId = useId();
  const hint = t("logoMenu.editSidebarHint");

  const content = (
    <DropdownMenuContent
      side={variant === "dock" ? "right" : "bottom"}
      align="start"
      className="w-60"
    >
      {variant === "navbar" ? null : (
        <>
          <DropdownMenuSub>
            <DropdownMenuSubTrigger>
              <Icon name="layers" />
              {t("logoMenu.switchDesk")}
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <DropdownMenuRadioGroup
                value={deskId ?? ""}
                onValueChange={(id) => {
                  const desk = desks.find((d) => d.id === id);
                  if (desk) switchDesk(desk);
                }}
              >
                {desks.map((d) => (
                  <DeskRadioItem key={d.id} desk={d} />
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
          <DropdownMenuItem onSelect={goToDesks}>
            <Icon name="layout-dashboard" />
            {t("logoMenu.allDesks")}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
        </>
      )}
      <DisabledHint hint={hint}>
        <DropdownMenuItem disabled aria-describedby={hintId}>
          <Icon name="pencil" />
          {t("logoMenu.editSidebar")}
        </DropdownMenuItem>
      </DisabledHint>
      <span id={hintId} className="sr-only">
        {hint}
      </span>
      <DropdownMenuSeparator />
      <DropdownMenuItem onSelect={() => actions.openSettings("shortcuts")}>
        <Icon name="circle-question-mark" />
        {t("logoMenu.help")}
      </DropdownMenuItem>
      <DropdownMenuSub>
        <DropdownMenuSubTrigger>
          <Icon name="info" />
          {t("logoMenu.about")}
        </DropdownMenuSubTrigger>
        <DropdownMenuSubContent>
          <DropdownMenuLabel className="font-normal">
            <span className="block font-medium">
              <Message message={{ ns: "platform", key: "app.name" }} />
            </span>
            <span className="block text-xs text-muted-foreground">
              {t("logoMenu.version", { version: appVersion })}
            </span>
          </DropdownMenuLabel>
        </DropdownMenuSubContent>
      </DropdownMenuSub>
    </DropdownMenuContent>
  );

  if (variant === "dock") {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" className={DOCK_BUTTON_CLASS}>
            <span className="sr-only">{t("logoMenu.label")}</span>
            <Mark />
          </Button>
        </DropdownMenuTrigger>
        {content}
      </DropdownMenu>
    );
  }

  if (variant === "navbar") {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" className="h-10 gap-2 px-1.5">
            <span className="sr-only">{t("logoMenu.label")}</span>
            <Mark />
            <span aria-hidden="true" className="hidden font-semibold sm:inline">
              <Message message={{ ns: "platform", key: "app.name" }} />
            </span>
          </Button>
        </DropdownMenuTrigger>
        {content}
      </DropdownMenu>
    );
  }

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton size="lg" tooltip={t("app.name")}>
              <span className="sr-only">{t("logoMenu.label")}</span>
              <Mark />
              <span className="truncate font-semibold group-data-[collapsible=icon]:hidden">
                <Message message={{ ns: "platform", key: "app.name" }} />
              </span>
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          {content}
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
