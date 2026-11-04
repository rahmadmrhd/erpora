import { useTranslation } from "@erpora/platform-web";
import {
  Alert,
  AlertDescription,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  Input,
  Label,
  resolveIcon,
} from "@erpora/ui";
import { type ComponentProps, type ReactNode, useEffect, useId, useState } from "react";
import "../i18n/keys.gen";
import type { AuthMessage } from "./errors";

/** Fungsi terjemah `core` yang longgar untuk kunci dinamis (kode galat); kunci literal tetap diperiksa di katalog. */
export type CoreT = (key: string, args?: Record<string, string | number>) => string;

export function useCoreT(): CoreT {
  const { t } = useTranslation("core");
  return t as unknown as CoreT;
}

export function translateMessage(t: CoreT, m: AuthMessage): string {
  return t(m.key, m.args);
}

/** Kartu di tengah layar dengan nama aplikasi, judul, dan deskripsi (meniru halaman login ERPNext). */
export function AuthLayout({
  title,
  description,
  children,
  footer,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  footer?: ReactNode;
}) {
  const t = useCoreT();
  const brand = t("auth.brand");
  // Gerbang dirender tanpa shell, sehingga tak ada yang menyinkronkan judul tab: tanpa ini judul halaman shell
  // sebelumnya ("Dashboard - ERPora") terbawa ke layar login/lupa/reset. Format sama dengan shell: "<judul> - <aplikasi>".
  useEffect(() => {
    document.title = `${title} - ${brand}`;
  }, [title, brand]);
  return (
    <main className="flex min-h-svh flex-col items-center justify-center gap-6 bg-muted p-4">
      <p className="text-2xl font-semibold tracking-tight">{t("auth.brand")}</p>
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>
            <h1 className="text-xl leading-none font-semibold">{title}</h1>
          </CardTitle>
          {description ? <CardDescription>{description}</CardDescription> : null}
        </CardHeader>
        <CardContent className="flex flex-col gap-4">{children}</CardContent>
        {footer ? <CardFooter className="flex-col gap-2">{footer}</CardFooter> : null}
      </Card>
    </main>
  );
}

/** Daftar galat (satu `Alert` destruktif; role="alert" dari shadcn). */
export function ErrorAlert({ messages }: { messages: readonly AuthMessage[] }) {
  const t = useCoreT();
  if (messages.length === 0) return null;
  return (
    <Alert variant="destructive">
      <AlertDescription>
        {messages.length === 1 ? (
          <p>{translateMessage(t, messages[0] as AuthMessage)}</p>
        ) : (
          <ul className="list-disc pl-4">
            {messages.map((m) => (
              <li key={m.key}>{translateMessage(t, m)}</li>
            ))}
          </ul>
        )}
      </AlertDescription>
    </Alert>
  );
}

/** Label + kontrol + pesan galat per field, dengan `aria-describedby` dan `aria-invalid`. */
export function Field({
  label,
  error,
  hint,
  children,
}: {
  label: string;
  error?: string | undefined;
  hint?: string | undefined;
  children: (props: {
    id: string;
    "aria-invalid": true | undefined;
    "aria-describedby": string | undefined;
  }) => ReactNode;
}) {
  const id = useId();
  const noteId = `${id}-note`;
  const note = error ?? hint;
  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={id}>{label}</Label>
      {children({
        id,
        "aria-invalid": error ? true : undefined,
        "aria-describedby": note ? noteId : undefined,
      })}
      {note ? (
        <p
          id={noteId}
          className={error ? "text-sm text-destructive" : "text-sm text-muted-foreground"}
        >
          {note}
        </p>
      ) : null}
    </div>
  );
}

/** Input sandi dengan tombol tampilkan/sembunyikan (aria-pressed); nilai tidak pernah disimpan di luar state. */
export function PasswordInput({
  autoComplete,
  ...props
}: Omit<ComponentProps<typeof Input>, "type"> & {
  autoComplete: "current-password" | "new-password";
}) {
  const t = useCoreT();
  const [shown, setShown] = useState(false);
  const Icon = resolveIcon(shown ? "eye-off" : "eye");
  return (
    <div className="relative">
      <Input
        {...props}
        type={shown ? "text" : "password"}
        autoComplete={autoComplete}
        autoCapitalize="none"
        spellCheck={false}
        className="pr-10"
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="absolute top-0 right-0 size-9"
        aria-pressed={shown}
        aria-label={t(shown ? "auth.common.hidePassword" : "auth.common.showPassword")}
        onClick={() => setShown((v) => !v)}
      >
        {Icon ? <Icon aria-hidden="true" /> : null}
      </Button>
    </div>
  );
}

/** Pemeriksaan email sederhana sisi klien (server tetap otoritas). */
export function looksLikeEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+$/.test(value.trim());
}
