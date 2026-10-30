import { useNavigate } from "@erpora/module-sdk";
import {
  safeInternalHref,
  type ThemePreference,
  useBootstrap,
  useCurrentUser,
  useCurrentUserSource,
  useMessage,
  useTheme,
  useTranslation,
} from "@erpora/platform-web";
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
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
  initialsOf,
  resolveIcon,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@erpora/ui";

function Icon({ name }: { name: string }) {
  const C = resolveIcon(name);
  return C ? <C aria-hidden="true" /> : null;
}

/** Item yang dinonaktifkan dengan penjelasan (tooltip dan deskripsi untuk pembaca layar). */
function DisabledHint({ hint, children }: { hint: string; children: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="block">{children}</span>
      </TooltipTrigger>
      <TooltipContent side="right">{hint}</TooltipContent>
    </Tooltip>
  );
}

/**
 * Avatar, nama, dan email di dasar sidebar (terlipat: hanya avatar) dengan menu akun: Kelola Akun, Bahasa, Tema, Keluar.
 * Data pengguna dari `CurrentUserSource` (PII; hanya memori). Aksi yang belum tersedia dinonaktifkan dengan penjelasan.
 */
export function UserMenu() {
  const { t, locale, setLocale } = useTranslation("platform");
  const { user } = useCurrentUser();
  const source = useCurrentUserSource();
  const { preference, setPreference } = useTheme();
  const locales = useBootstrap().i18n?.locales ?? [];
  const navigate = useNavigate();
  const anonymous = useMessage({ ns: "platform", key: "user.anonymous" });

  const name = user?.displayName ?? anonymous;
  const email = user?.email ?? null;
  const avatarSrc = safeInternalHref(user?.avatarUrl ?? undefined) ?? undefined;
  const manageAccountPath = source.manageAccountPath?.() ?? null;
  const canManage = source.capabilities.manageAccount && manageAccountPath !== null;
  const canSignOut = source.capabilities.signOut;
  const hint = t("user.unavailable");

  const identity = (
    <>
      <Avatar size="sm">
        {avatarSrc ? <AvatarImage src={avatarSrc} alt="" /> : null}
        <AvatarFallback>{initialsOf(name)}</AvatarFallback>
      </Avatar>
      <span className="grid min-w-0 flex-1 text-left leading-tight group-data-[collapsible=icon]:hidden">
        <span className="truncate text-sm font-medium">{name}</span>
        {email ? <span className="truncate text-xs text-muted-foreground">{email}</span> : null}
      </span>
    </>
  );

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
          <DropdownMenuContent side="right" align="end" className="w-64">
            <DropdownMenuLabel className="flex items-center gap-2 font-normal">
              {identity}
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            {canManage ? (
              <DropdownMenuItem onSelect={() => void navigate({ to: manageAccountPath })}>
                <Icon name="user" />
                {t("user.manageAccount")}
              </DropdownMenuItem>
            ) : (
              <DisabledHint hint={hint}>
                <DropdownMenuItem disabled aria-describedby="user-menu-unavailable">
                  <Icon name="user" />
                  {t("user.manageAccount")}
                </DropdownMenuItem>
              </DisabledHint>
            )}
            <DropdownMenuSub>
              <DropdownMenuSubTrigger>
                <Icon name="languages" />
                {t("shell.language.label")}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent>
                <DropdownMenuRadioGroup value={locale} onValueChange={setLocale}>
                  {locales.map((l) => (
                    <DropdownMenuRadioItem key={l.code} value={l.code}>
                      {l.name}
                    </DropdownMenuRadioItem>
                  ))}
                </DropdownMenuRadioGroup>
              </DropdownMenuSubContent>
            </DropdownMenuSub>
            <DropdownMenuSub>
              <DropdownMenuSubTrigger>
                <Icon name="sun" />
                {t("theme.label")}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent>
                <DropdownMenuRadioGroup
                  value={preference}
                  onValueChange={(v) => setPreference(v as ThemePreference)}
                >
                  <DropdownMenuRadioItem value="light">{t("theme.light")}</DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="dark">{t("theme.dark")}</DropdownMenuRadioItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuRadioItem value="system">{t("theme.system")}</DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
              </DropdownMenuSubContent>
            </DropdownMenuSub>
            <DropdownMenuSeparator />
            {canSignOut ? (
              <DropdownMenuItem variant="destructive" onSelect={() => void source.signOut?.()}>
                <Icon name="log-out" />
                {t("user.signOut")}
              </DropdownMenuItem>
            ) : (
              <DisabledHint hint={hint}>
                <DropdownMenuItem
                  variant="destructive"
                  disabled
                  aria-describedby="user-menu-unavailable"
                >
                  <Icon name="log-out" />
                  {t("user.signOut")}
                </DropdownMenuItem>
              </DisabledHint>
            )}
            <span id="user-menu-unavailable" className="sr-only">
              {hint}
            </span>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
