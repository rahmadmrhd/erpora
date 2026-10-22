interface BootstrapResponse {
  data: { appVersion: string; tenantId: string };
}

async function main(): Promise<void> {
  const el = document.getElementById("version");
  if (!el) return;
  try {
    const res = await fetch("/api/bootstrap");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const body = (await res.json()) as BootstrapResponse;
    el.textContent = `Versi aplikasi: ${body.data.appVersion}`;
  } catch (err) {
    el.textContent = `Gagal memuat bootstrap: ${String(err)}`;
  }
}

void main();
