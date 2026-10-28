# platform-web

Layanan frontend platform (`@erpora/platform-web`) yang dipakai bersama app shell dan modul. Saat ini berisi runtime i18n
(`src/i18n`): format pesan ICU MessageFormat di atas `intl-messageformat` (versi dipin), dibuktikan sama dengan sisi Go lewat
fixture konformansi `sdk/i18n/testdata/conformance.json` (`bun test`). Provider dan hook React (`I18nProvider`,
`useTranslation`) menyusul setelah toolchain React terpasang (M3a). Spesifikasi: `.kiro/specs/i18n-platform/`.
