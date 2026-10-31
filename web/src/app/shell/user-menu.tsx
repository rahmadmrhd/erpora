import {
  safeInternalHref,
  useCurrentUser,
  useCurrentUserSource,
  useMessage,
  useTranslation,
} from "@erpora/platform-web";
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  initialsOf,
  resolveIcon,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@erpora/ui";
import { useId } from "react";
import { DisabledHint } from "./disabled-hint";
import { useShellActions } from "./shell-context";

function Icon({ name }: { name: string }) {
  const C = resolveIcon(name);
  return C ? <C aria-hidden="true" /> : null;
}

/**
 * Avatar (dan, pada varian `panel`, nama dan email) yang membuka menu akun: Pengaturan, Muat ulang, Keluar (R16).
 * `dock`: hanya avatar di dasar Dock Pinned; `panel`: identitas lengkap di dasar panel (Floating dan Sheet); `navbar`:
 * hanya avatar di kanan navbar halaman Desks (K1, menu terbuka ke bawah). Data
 * pengguna dari `CurrentUserSource` (PII; hanya memori). Keluar dinonaktifkan dengan penjelasan sampai PRD-001;
 * tidak ada "My Space" (tidak ada konsepnya di ERPora).
 */
export function UserMenu({ variant }: { variant: "dock" | "panel" | "navbar" }) {
  const { t } = useTranslation("platform");
  const { user } = useCurrentUser();
  const source = useCurrentUserSource();
  const actions = useShellActions();
  const anonymous = useMessage({ ns: "platform", key: "user.anonymous" });
  const hintId = useId();

  const name = user?.displayName ?? anonymous;
  const email = user?.email ?? null;
  const avatarSrc = safeInternalHref(user?.avatarUrl ?? undefined) ?? undefined;
  const canSignOut = source.capabilities.signOut;
  const hint = t("user.unavailable");

  const avatar = (
    <Avatar size="sm" aria-hidden={variant === "panel" ? undefined : "true"}>
      {avatarSrc ? <AvatarImage src={avatarSrc} alt="" /> : null}
      <AvatarFallback>{initialsOf(name)}</AvatarFallback>
    </Avatar>
  );
  const identity = (
    <>
      {avatar}
      <span className="grid min-w-0 flex-1 text-left leading-tight group-data-[collapsible=icon]:hidden">
        <span className="truncate text-sm font-medium">{name}</span>
        {email ? <span className="truncate text-xs text-muted-foreground">{email}</span> : null}
      </span>
    </>
  );

  const content = (
    <DropdownMenuContent
      side={variant === "navbar" ? "bottom" : "right"}
      align="end"
      className="w-64"
    >
      <DropdownMenuLabel className="flex items-center gap-2 font-normal">
        {avatar}
        <span className="grid min-w-0 flex-1 leading-tight">
          <span className="truncate text-sm font-medium">{name}</span>
          {email ? <span className="truncate text-xs text-muted-foreground">{email}</span> : null}
        </span>
      </DropdownMenuLabel>
      <DropdownMenuSeparator />
      <DropdownMenuItem onSelect={() => actions.openSettings()}>
        <Icon name="settings" />
        {t("user.settings")}
      </DropdownMenuItem>
      <DropdownMenuItem onSelect={() => actions.reload()}>
        <Icon name="refresh-cw" />
        {t("user.reload")}
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      {canSignOut ? (
        <DropdownMenuItem variant="destructive" onSelect={() => void source.signOut?.()}>
          <Icon name="log-out" />
          {t("user.signOut")}
        </DropdownMenuItem>
      ) : (
        <DisabledHint hint={hint}>
          <DropdownMenuItem variant="destructive" disabled aria-describedby={hintId}>
            <Icon name="log-out" />
            {t("user.signOut")}
          </DropdownMenuItem>
        </DisabledHint>
      )}
      <span id={hintId} className="sr-only">
        {hint}
      </span>
    </DropdownMenuContent>
  );

  if (variant !== "panel") {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" className="size-10 rounded-full">
            <span className="sr-only">{`${t("user.menu")} ${name}`}</span>
            {avatar}
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
            <SidebarMenuButton size="lg" tooltip={name}>
              {/* Nama aksesibel = teks terlihat (nama, email) berawalan "Menu akun": label tidak menimpa teks terlihat (WCAG 2.5.3). */}
              <span className="sr-only">{t("user.menu")}</span>
              {identity}
              <Icon name="chevrons-up-down" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          {content}
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
