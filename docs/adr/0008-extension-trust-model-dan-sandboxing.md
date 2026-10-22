# ADR-008: Extension trust model dan sandboxing

- **Status:** Proposed

## Konteks

Ekstensi pihak ketiga (paket, signing, sandbox) dijadwalkan di fase 06 (PRD-000 §4 dan §8). Ekstensi yang tidak tepercaya membutuhkan model trust dan isolasi eksekusi. Kandidat teknis yang sudah muncul: menjalankan ekstensi di dalam runtime WebAssembly (wazero), dan komunikasi ke host lewat gRPC. Keduanya belum dievaluasi.

Rujukan: `docs/architecture/extensions-and-marketplace.md`.

## Keputusan

Belum diputuskan. Kandidat wazero dan gRPC akan dievaluasi bersama model trust.
