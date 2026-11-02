// Dev API di laptop dengan PostgreSQL di VPS lewat tunnel SSH (Docker tidak dijalankan di laptop; lihat CLAUDE.md).
// Urutan: tunnel (IPv4, lalu IPv6 bila gagal) -> database dev -> migrasi + install modul -> air (live-reload).
// Verifikasi host key SSH tidak pernah dinonaktifkan; bila kedua alias gagal, skrip berhenti dan melaporkannya.
import { join } from "node:path";
import { resolveSecretEnv } from "./tools/envfile";

const ROOT = join(import.meta.dir, "..");
const ALIASES = (process.env.ERP_DEV_VPS_ALIASES ?? "rhd_vps_ipv4,rhd_vps_ipv6").split(",");
const LOCAL_PORT = Number(process.env.ERP_DEV_TUNNEL_PORT ?? 15432);
const PG_CONTAINER = process.env.ERP_DEV_PG_CONTAINER ?? "erpora-test-pg";
const DB_NAME = process.env.ERP_DEV_DB_NAME ?? "erpora_dev";
const API_ADDR = "127.0.0.1:8080";

const apiEnv = {
  ...process.env,
  ERP_DATABASE_URL: `postgres://erp:erp@127.0.0.1:${LOCAL_PORT}/${DB_NAME}?sslmode=disable`,
  ERP_ENV: "development",
  ERP_HTTP_ADDR: API_ADDR,
  ERP_TENANCY_MODE: "single",
};

const SSH_OPTS = [
  "-o",
  "BatchMode=yes",
  "-o",
  "ExitOnForwardFailure=yes",
  "-o",
  "ServerAliveInterval=15",
  "-o",
  "ServerAliveCountMax=8",
  "-o",
  "TCPKeepAlive=yes",
];

type Child = ReturnType<typeof Bun.spawn>;

let tunnel: Child | undefined;
let air: Child | undefined;
let stopping = false;
let activeAlias = ALIASES[0] ?? "";

function log(msg: string) {
  console.log(`[dev-api] ${msg}`);
}

async function portOpen(): Promise<boolean> {
  try {
    const sock = await Bun.connect({
      hostname: "127.0.0.1",
      port: LOCAL_PORT,
      socket: { data() {} },
    });
    sock.end();
    return true;
  } catch {
    return false;
  }
}

// Membuka tunnel lewat satu alias; true bila port lokal menerima koneksi sebelum proses ssh berhenti.
async function openTunnel(alias: string): Promise<boolean> {
  const proc = Bun.spawn(
    ["ssh", ...SSH_OPTS, "-N", "-L", `127.0.0.1:${LOCAL_PORT}:127.0.0.1:5432`, alias],
    {
      stdin: "ignore",
      stdout: "inherit",
      stderr: "inherit",
    },
  );
  let exited = false;
  void proc.exited.then(() => {
    exited = true;
  });
  for (let i = 0; i < 60 && !exited; i++) {
    if (await portOpen()) {
      tunnel = proc;
      activeAlias = alias;
      return true;
    }
    await Bun.sleep(500);
  }
  proc.kill();
  return false;
}

// Mencoba semua alias berurutan (dua putaran, karena tunnel sering putus sesaat); gagal total = lapor ke pengguna.
async function connectTunnel(): Promise<void> {
  for (let round = 0; round < 2; round++) {
    for (const alias of ALIASES) {
      log(`membuka tunnel lewat ${alias}`);
      if (await openTunnel(alias)) {
        log(`tunnel aktif: 127.0.0.1:${LOCAL_PORT} -> ${alias}:5432`);
        return;
      }
      log(`tunnel lewat ${alias} gagal`);
    }
  }
  throw new Error(
    `tunnel gagal lewat semua alias (${ALIASES.join(", ")}); laporkan ke pemilik proyek`,
  );
}

// Menjaga tunnel: bila ssh putus saat API berjalan, buka lagi (alias lain bila perlu).
async function superviseTunnel(): Promise<void> {
  while (!stopping) {
    await tunnel?.exited;
    if (stopping) return;
    log("tunnel putus, membuka kembali");
    try {
      await connectTunnel();
    } catch (err) {
      console.error(`[dev-api] ${(err as Error).message}`);
      await shutdown(1);
    }
  }
}

async function run(
  cmd: string[],
  opts: { env?: Record<string, string | undefined>; quiet?: boolean } = {},
) {
  const proc = Bun.spawn(cmd, {
    cwd: ROOT,
    env: opts.env ?? process.env,
    stdin: "ignore",
    stdout: opts.quiet ? "pipe" : "inherit",
    stderr: "inherit",
  });
  const out = opts.quiet ? await new Response(proc.stdout as ReadableStream).text() : "";
  const code = await proc.exited;
  return { code, out: out.trim() };
}

// Menunggu tunnel (yang dijaga superviseTunnel) pulih; tunnel sering putus sesaat (`Connection reset by peer`).
async function waitForTunnel(timeoutMs = 90_000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (!(await portOpen())) {
    if (Date.now() > deadline) throw new Error("tunnel tidak pulih; laporkan ke pemilik proyek");
    await Bun.sleep(1000);
  }
}

// Perintah di sini idempoten (migrate up, modules install, cek/buat database), jadi aman diulang saat tunnel putus.
async function runRetry(
  label: string,
  build: () => string[],
  opts: { env?: Record<string, string | undefined>; quiet?: boolean } = {},
) {
  const attempts = 4;
  for (let i = 1; ; i++) {
    await waitForTunnel();
    const res = await run(build(), opts);
    if (res.code === 0) return res;
    if (i >= attempts) throw new Error(`${label} gagal setelah ${attempts} percobaan`);
    log(`${label} gagal (percobaan ${i}/${attempts}); mengulang setelah tunnel siap`);
    await Bun.sleep(2000);
  }
}

async function ensureDatabase(): Promise<void> {
  const psql = (sql: string) => [
    "ssh",
    ...SSH_OPTS,
    activeAlias,
    "docker",
    "exec",
    PG_CONTAINER,
    "psql",
    "-U",
    "erp",
    "-d",
    "postgres",
    "-tAc",
    `"${sql}"`,
  ];
  const probe = await runRetry(
    `cek database ${DB_NAME} (container ${PG_CONTAINER} hidup di VPS?)`,
    () => psql(`SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'`),
    { quiet: true },
  );
  if (probe.out === "1") return;
  log(`membuat database ${DB_NAME}`);
  await runRetry(`membuat database ${DB_NAME}`, () => psql(`CREATE DATABASE ${DB_NAME}`), {
    quiet: true,
  });
}

async function shutdown(code: number): Promise<never> {
  stopping = true;
  air?.kill();
  tunnel?.kill();
  process.exit(code);
}

// serve butuh ERP_SECRET_KEY di semua mode (tanpa kunci sementara). Ambil dari environment proses atau .env (diurai seperti
// godotenv: komentar inline, entri ganda terakhir menang); ERP_SECRET_KEY_PREVIOUS (rotasi) ikut diteruskan. air untuk VPS
// menjalankan serve tanpa --env-file, jadi nilainya dimasukkan lewat apiEnv. Nilainya tidak pernah dicetak.
async function ensureSecretKey(): Promise<void> {
  const envFile = Bun.file(join(ROOT, ".env"));
  const text = (await envFile.exists()) ? await envFile.text() : undefined;
  Object.assign(apiEnv, resolveSecretEnv(process.env, text));
}

async function main(): Promise<void> {
  process.on("SIGINT", () => void shutdown(0));
  process.on("SIGTERM", () => void shutdown(0));

  await ensureSecretKey();

  await connectTunnel();
  void superviseTunnel();
  await ensureDatabase();

  const cli = ["go", "run", "-tags", "devtools", "./cmd/erpora"];
  log("migrate up");
  await runRetry("migrate up", () => [...cli, "migrate", "up"], { env: apiEnv });
  log("modules install --all --enable");
  await runRetry("modules install", () => [...cli, "modules", "install", "--all", "--enable"], {
    env: apiEnv,
  });

  log(`menjalankan air; API di http://${API_ADDR}`);
  air = Bun.spawn(["go", "tool", "air", "-c", ".air.vps.toml"], {
    cwd: ROOT,
    env: apiEnv,
    stdin: "ignore",
    stdout: "inherit",
    stderr: "inherit",
  });
  await shutdown((await air.exited) ?? 0);
}

main().catch(async (err: Error) => {
  console.error(`[dev-api] ${err.message}`);
  await shutdown(1);
});
