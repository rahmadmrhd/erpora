# module-sdk

Paket TypeScript untuk modul frontend. Saat ini berisi tipe `ModuleManifest` hasil generate dari `contracts/module.schema.json`
(`src/manifest.gen.ts`, JANGAN DIEDIT; perbarui lewat `erpora contracts:generate`). `src/manifest.check.ts` memastikan tipe itu
menerima manifest `example` dan menolak nilai yang melanggar schema. `defineModule()` dan `OfflinePolicy` menyusul pada PRD-000
FR-09 (frontend module registry dan SDK TS).
