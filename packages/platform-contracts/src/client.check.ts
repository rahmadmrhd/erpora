// Pemeriksaan tipe saja (dijalankan lewat `typecheck`): client bertipe menurunkan tipe `data` dari kontrak dan menolak
// path yang tidak ada. Tidak diekspor dan tidak dijalankan.
import { createClient } from "./client";
import type { paths } from "./platform.gen";

export async function checkClient(): Promise<string> {
  const client = createClient<paths>({ baseUrl: "http://localhost" });
  const res = await client.get("/api/platform/modules", { params: { query: { page: 1 } } });
  if (!res.ok) return res.error.code;
  // data adalah daftar modul bertipe dari kontrak.
  const first = res.data[0];
  const key: string | undefined = first?.key;

  // @ts-expect-error path tidak ada di kontrak platform
  await client.get("/api/platform/tidak-ada");

  const one = await client.get("/api/platform/modules/{key}", {
    params: { path: { key: "acme.example" } },
  });
  if (one.ok) {
    const status: string = one.data.status;
    return `${key ?? ""}${status}`;
  }
  return "";
}
