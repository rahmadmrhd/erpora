// Pemeriksaan tipe saja (dijalankan lewat `typecheck`): manifest example harus diterima ModuleManifest, dan
// nilai yang melanggar schema harus ditolak. Tidak diekspor dan tidak dijalankan.
import type { ModuleManifest } from "./manifest.gen";

const example: ModuleManifest = {
  schema_version: "1",
  key: "acme.example",
  slug: "example",
  name: "Example",
  version: "1.0.0",
  publisher: { id: "acme", name: "ACME" },
  trust: "builtin",
  compatibility: { platform: "^0.1", sdk: "^0.1" },
  dependencies: [{ module: "platform.core", version: "^0.1", optional: false }],
  capabilities: [],
  resources: [
    {
      key: "example.note",
      label: "resource.note",
      actions: ["read"],
      field_groups: [
        { key: "general", label: "fieldGroup.general", classification: "internal", default: true },
      ],
    },
  ],
  migrations: { strategy: "versioned" },
  offline: { mode: "online-only" },
};

// @ts-expect-error trust hanya builtin, verified, atau untrusted
const badTrust: ModuleManifest = { ...example, trust: "community" };

// @ts-expect-error slug harus string
const badSlug: ModuleManifest = { ...example, slug: 1 };

const badResource: ModuleManifest = {
  ...example,
  // @ts-expect-error resource wajib punya field_groups
  resources: [{ key: "example.note", label: "resource.note", actions: ["read"] }],
};

export type Checked = [typeof example, typeof badTrust, typeof badSlug, typeof badResource];
