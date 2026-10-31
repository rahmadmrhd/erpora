// Entry sekunder `@erpora/ui/command` (M3a revisi, R18.2): `Command` (shadcn + cmdk 1.1.1) dipisah dari barrel agar `cmdk`
// tidak masuk bundel awal. Hanya palet perintah (dimuat malas lewat `import()`) yang mengimpor entry ini. Berkas
// `components/ui/command.tsx` = hasil `shadcn add command` persis (lihat shadcn.lock.json); tidak diubah tangan.
export {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from "../components/ui/command";
