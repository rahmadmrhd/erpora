import { describe, expect, test } from "bun:test";
import { createAuthController } from "./controller";
import { apiError, expectNoBrowserStorage, fakeServer, json, session } from "./test-support";

const REFRESH = "/api/core/auth/refresh";
const LOGOUT = "/api/core/auth/logout";
const PROTECTED = "/api/example/notes";

/** Server palsu: /notes menjawab 401 token_expired selama token bukan `valid`; refresh menerbitkan `valid`. */
function expiredThenOk(valid = "token-baru") {
  return fakeServer({
    [PROTECTED]: (c) =>
      c.authorization === `Bearer ${valid}` ? json({ data: [] }) : apiError(401, "token_expired"),
    [REFRESH]: () => session(valid),
  });
}

describe("token hanya di memori", () => {
  test("access token tidak pernah ditulis ke localStorage, sessionStorage, document.cookie, atau IndexedDB", async () => {
    const secret = "rahasia-access-token-abc123";
    const next = "rahasia-token-hasil-refresh-xyz789";
    const server = fakeServer({
      [PROTECTED]: (c) =>
        c.authorization === `Bearer ${next}` ? json({ data: [] }) : apiError(401, "token_expired"),
      [REFRESH]: () => session(next),
      [LOGOUT]: () => new Response(null, { status: 204 }),
    });
    await expectNoBrowserStorage([secret, next], async () => {
      const auth = createAuthController({ fetch: server.fetch });
      auth.acceptSession({ accessToken: secret, expiresIn: 600, mfaState: "none" });
      expect(auth.getAccessToken()).toBe(secret);
      await auth.fetch(PROTECTED); // 401 -> refresh -> ulang
      expect(auth.getAccessToken()).toBe(next);
      await auth.logout();
      expect(auth.getAccessToken()).toBeUndefined();
    });
  });
});

describe("fetch berautentikasi", () => {
  test("menambahkan Bearer hanya bila ada token; tanpa token tidak ada header dan 401 tidak memicu refresh", async () => {
    const server = fakeServer({
      [PROTECTED]: () => apiError(401, "unauthenticated"),
      [REFRESH]: () => session("x"),
    });
    const auth = createAuthController({ fetch: server.fetch, initial: "anonymous" });
    expect(auth.headers()).toEqual({});
    const res = await auth.fetch(PROTECTED);
    expect(res.status).toBe(401);
    expect(server.calls[0]?.authorization).toBeNull();
    expect(server.count(REFRESH)).toBe(0);

    auth.acceptSession({ accessToken: "t1", expiresIn: 600, mfaState: "none" });
    expect(auth.headers()).toEqual({ Authorization: "Bearer t1" });
  });

  test("tiga 401 token_expired serentak memicu SATU refresh; tiap request diulang sekali dengan token baru", async () => {
    const server = expiredThenOk();
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "lama", expiresIn: 600, mfaState: "none" });

    const results = await Promise.all([
      auth.fetch(PROTECTED),
      auth.fetch(PROTECTED),
      auth.fetch(PROTECTED),
    ]);
    expect(results.map((r) => r.status)).toEqual([200, 200, 200]);
    expect(server.count(REFRESH)).toBe(1);
    // 3 percobaan pertama (token lama) + 3 ulangan (token baru): masing-masing diulang tepat sekali.
    const notes = server.calls.filter((c) => c.path === PROTECTED);
    expect(notes.map((c) => c.authorization)).toEqual([
      "Bearer lama",
      "Bearer lama",
      "Bearer lama",
      "Bearer token-baru",
      "Bearer token-baru",
      "Bearer token-baru",
    ]);
    expect(auth.getState()).toEqual({ status: "authenticated" });
  });

  test("hanya diulang sekali: 401 lagi setelah refresh dikembalikan apa adanya (tanpa putaran tak berujung)", async () => {
    const server = fakeServer({
      [PROTECTED]: () => apiError(401, "token_expired"),
      [REFRESH]: () => session("token-baru"),
    });
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "lama", expiresIn: 600, mfaState: "none" });
    const res = await auth.fetch(PROTECTED);
    expect(res.status).toBe(401);
    expect(server.count(PROTECTED)).toBe(2);
    expect(server.count(REFRESH)).toBe(1);
  });

  test("credentials include HANYA untuk POST /api/core/auth/refresh", async () => {
    const server = expiredThenOk();
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "lama", expiresIn: 600, mfaState: "none" });
    await auth.fetch(PROTECTED);
    for (const c of server.calls) {
      if (c.path === REFRESH) {
        expect(c.method).toBe("POST");
        expect(c.credentials).toBe("include");
        expect(c.authorization).toBeNull(); // refresh memakai cookie, bukan access token
      } else {
        expect(c.credentials).not.toBe("include");
      }
    }
  });

  test("401 unauthenticated (sesi dicabut) saat memegang token juga mencoba refresh", async () => {
    const server = fakeServer({
      [PROTECTED]: () => apiError(401, "unauthenticated"),
      [REFRESH]: () => apiError(401, "unauthenticated"),
    });
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "none" });
    await auth.fetch(PROTECTED);
    expect(server.count(REFRESH)).toBe(1);
    expect(auth.getState()).toEqual({ status: "anonymous" });
  });
});

describe("refresh gagal", () => {
  test("ditolak (401): token dibuang, sesi berakhir, pendengar diberi tahu, respons 401 asal dikembalikan", async () => {
    const server = fakeServer({
      [PROTECTED]: () => apiError(401, "token_expired"),
      [REFRESH]: () => apiError(401, "unauthenticated"),
    });
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "lama", expiresIn: 600, mfaState: "none" });
    const seen: string[] = [];
    auth.subscribe(() => seen.push(auth.getState().status));

    const res = await auth.fetch(PROTECTED);
    expect(res.status).toBe(401);
    expect(auth.getAccessToken()).toBeUndefined();
    expect(auth.getState()).toEqual({ status: "anonymous" });
    expect(seen).toEqual(["anonymous"]);
    expect(server.count(PROTECTED)).toBe(1); // tidak diulang
  });

  test("galat sementara (500 atau jaringan putus): sesi TIDAK diakhiri", async () => {
    for (const refresh of [
      () => apiError(500, "internal_error"),
      () => apiError(429, "rate_limited"),
      () => {
        throw new TypeError("network");
      },
    ]) {
      const server = fakeServer({
        [PROTECTED]: () => apiError(401, "token_expired"),
        [REFRESH]: refresh,
      });
      const auth = createAuthController({ fetch: server.fetch });
      auth.acceptSession({ accessToken: "lama", expiresIn: 600, mfaState: "none" });
      const res = await auth.fetch(PROTECTED);
      expect(res.status).toBe(401);
      expect(auth.getAccessToken()).toBe("lama");
      expect(auth.getState()).toEqual({ status: "authenticated" });
    }
  });

  test("respons refresh yang bentuknya rusak dianggap galat sementara", async () => {
    const server = fakeServer({
      [PROTECTED]: () => apiError(401, "token_expired"),
      [REFRESH]: () => json({ data: { accessToken: 5 } }),
    });
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "lama", expiresIn: 600, mfaState: "none" });
    await auth.fetch(PROTECTED);
    expect(auth.getAccessToken()).toBe("lama");
  });
});

describe("boot dan MFA", () => {
  test("boot: refresh senyap berhasil -> authenticated; gagal -> anonymous", async () => {
    const ok = createAuthController({ fetch: fakeServer({ [REFRESH]: () => session("t") }).fetch });
    expect(ok.getState()).toEqual({ status: "booting" });
    await ok.boot();
    expect(ok.getState()).toEqual({ status: "authenticated" });
    expect(ok.getAccessToken()).toBe("t");

    const bad = createAuthController({
      fetch: fakeServer({ [REFRESH]: () => apiError(401, "unauthenticated") }).fetch,
    });
    await bad.boot();
    expect(bad.getState()).toEqual({ status: "anonymous" });

    const down = createAuthController({
      fetch: fakeServer({ [REFRESH]: () => apiError(503, "not_ready") }).fetch,
    });
    await down.boot();
    expect(down.getState()).toEqual({ status: "anonymous" });
  });

  test("boot dua kali (StrictMode) hanya satu refresh", async () => {
    const server = fakeServer({ [REFRESH]: () => session("t") });
    const auth = createAuthController({ fetch: server.fetch });
    await Promise.all([auth.boot(), auth.boot()]);
    expect(server.count(REFRESH)).toBe(1);
  });

  test("mfaState dari sesi menentukan layar MFA (enroll_required dan verify_required)", () => {
    const auth = createAuthController({ initial: "anonymous" });
    auth.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "verify_required" });
    expect(auth.getState()).toEqual({ status: "mfa", mfaState: "verify_required" });
    auth.acceptSession({ accessToken: "t2", expiresIn: 600, mfaState: "enroll_required" });
    expect(auth.getState()).toEqual({ status: "mfa", mfaState: "enroll_required" });
    auth.acceptSession({ accessToken: "t3", expiresIn: 600, mfaState: "none" });
    expect(auth.getState()).toEqual({ status: "authenticated" });
  });

  test("403 mfa_required pada request biasa memunculkan layar MFA sesuai keadaan dari refresh", async () => {
    const server = fakeServer({
      [PROTECTED]: () => apiError(403, "mfa_required"),
      [REFRESH]: () => session("t-pra-mfa", "verify_required"),
    });
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "none" });
    const res = await auth.fetch(PROTECTED);
    expect(res.status).toBe(403); // respons asli tetap dikembalikan
    expect(auth.getState()).toEqual({ status: "mfa", mfaState: "verify_required" });
  });

  test("403 mfa_required tetapi refresh melaporkan mfaState none: ikuti server, keadaan tetap authenticated, 403 dikembalikan apa adanya", async () => {
    const server = fakeServer({
      [PROTECTED]: () => apiError(403, "mfa_required"),
      [REFRESH]: () => session("t2", "none"),
    });
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "none" });
    const res = await auth.fetch(PROTECTED);
    expect(res.status).toBe(403);
    expect((await res.json()).error.code).toBe("mfa_required");
    expect(auth.getState()).toEqual({ status: "authenticated" });
    expect(auth.getAccessToken()).toBe("t2");
  });

  test("403 mfa_required dengan refresh gagal (5xx): keadaan tidak berubah dan 403 dikembalikan", async () => {
    const server = fakeServer({
      [PROTECTED]: () => apiError(403, "mfa_required"),
      [REFRESH]: () => apiError(500, "internal_error"),
    });
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "none" });
    const res = await auth.fetch(PROTECTED);
    expect(res.status).toBe(403);
    expect(auth.getState()).toEqual({ status: "authenticated" });
  });

  test("403 selain mfa_required tidak mengubah keadaan", async () => {
    const server = fakeServer({ [PROTECTED]: () => apiError(403, "permission_denied") });
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "none" });
    await auth.fetch(PROTECTED);
    expect(auth.getState()).toEqual({ status: "authenticated" });
  });
});

describe("logout dan keadaan", () => {
  test("logout 204: server mengakui, hasil true, token dibuang, keadaan anonymous", async () => {
    const server = fakeServer({ [LOGOUT]: () => new Response(null, { status: 204 }) });
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "none" });
    expect(await auth.logout()).toBe(true);
    expect(server.calls).toHaveLength(1);
    expect(server.calls[0]).toMatchObject({
      path: LOGOUT,
      method: "POST",
      authorization: "Bearer t",
    });
    expect(auth.getAccessToken()).toBeUndefined();
    expect(auth.getState()).toEqual({ status: "anonymous" });
  });

  test("logout 401: sesi memang sudah tidak ada di server, dianggap keluar", async () => {
    const server = fakeServer({ [LOGOUT]: () => apiError(401, "unauthenticated") });
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "none" });
    expect(await auth.logout()).toBe(true);
    expect(auth.getAccessToken()).toBeUndefined();
    expect(auth.getState()).toEqual({ status: "anonymous" });
  });

  test("logout gagal (429, 500, 403, atau jaringan putus): TIDAK keluar, token dan keadaan tetap", async () => {
    const failures: Array<[string, () => Response | Promise<Response>]> = [
      [
        "429",
        () => json({ error: { code: "rate_limited", message: "x" } }, 429, { "Retry-After": "30" }),
      ],
      ["500", () => apiError(500, "internal_error")],
      ["503", () => new Response("down", { status: 503 })],
      ["403", () => apiError(403, "permission_denied")],
      [
        "jaringan putus",
        () => {
          throw new TypeError("network");
        },
      ],
    ];
    for (const [name, handler] of failures) {
      const server = fakeServer({ [LOGOUT]: handler });
      const auth = createAuthController({ fetch: server.fetch });
      auth.acceptSession({ accessToken: "t-" + name, expiresIn: 600, mfaState: "none" });
      expect(await auth.logout(), name).toBe(false);
      expect(auth.getAccessToken(), name).toBe("t-" + name);
      expect(auth.getState(), name).toEqual({ status: "authenticated" });
    }
  });

  test("logout gagal lalu dicoba lagi dan berhasil: baru keluar", async () => {
    const server = fakeServer({
      [LOGOUT]: (_c, n) =>
        n === 1 ? apiError(500, "internal_error") : new Response(null, { status: 204 }),
    });
    const auth = createAuthController({ fetch: server.fetch });
    auth.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "none" });
    expect(await auth.logout()).toBe(false);
    expect(auth.getState()).toEqual({ status: "authenticated" });
    expect(await auth.logout()).toBe(true);
    expect(auth.getState()).toEqual({ status: "anonymous" });
  });

  test("logout tanpa token tidak memanggil jaringan dan berhasil (tidak ada sesi lokal untuk dipertahankan)", async () => {
    const server = fakeServer({ [LOGOUT]: () => apiError(500, "internal_error") });
    const auth = createAuthController({ fetch: server.fetch, initial: "anonymous" });
    expect(await auth.logout()).toBe(true);
    expect(server.calls).toHaveLength(0);
  });

  test("snapshot stabil: refresh berulang pada keadaan sama tidak membuat objek keadaan baru", async () => {
    const auth = createAuthController({ initial: "anonymous" });
    auth.acceptSession({ accessToken: "a", expiresIn: 600, mfaState: "none" });
    const first = auth.getState();
    let notified = 0;
    auth.subscribe(() => notified++);
    auth.acceptSession({ accessToken: "b", expiresIn: 600, mfaState: "none" });
    expect(auth.getState()).toBe(first);
    expect(notified).toBe(0);
    expect(auth.getAccessToken()).toBe("b");
  });

  test("skipBoot langsung anonymous tanpa jaringan", async () => {
    const server = fakeServer({});
    const auth = createAuthController({ fetch: server.fetch });
    auth.skipBoot();
    expect(auth.getState()).toEqual({ status: "anonymous" });
    expect(server.calls).toHaveLength(0);
  });
});

describe("refresh berbatas waktu (kunci Web Locks tidak boleh tertahan)", () => {
  /** fetch yang tidak pernah selesai kecuali `signal` dibatalkan. */
  const hanging = ((_input: RequestInfo | URL, init?: RequestInit) =>
    new Promise<Response>((_resolve, reject) => {
      init?.signal?.addEventListener("abort", () =>
        reject(new DOMException("dibatalkan", "AbortError")),
      );
    })) as typeof fetch;

  test("request refresh membawa AbortSignal; timeout dianggap error (sesi tidak berakhir) dan tidak menahan refresh berikutnya", async () => {
    let signalSeen = false;
    let n = 0;
    const fetchFn = ((input: RequestInfo | URL, init?: RequestInit) => {
      n++;
      if (n === 1) {
        signalSeen = init?.signal instanceof AbortSignal;
        return hanging(input, init);
      }
      return Promise.resolve(session("t-setelah-timeout"));
    }) as typeof fetch;
    const auth = createAuthController({ fetch: fetchFn, refreshTimeoutMs: 20 });
    auth.acceptSession({ accessToken: "lama", expiresIn: 600, mfaState: "none" });
    expect(await auth.refresh()).toBe("error");
    expect(signalSeen).toBe(true);
    expect(auth.getState()).toEqual({ status: "authenticated" });
    expect(auth.getAccessToken()).toBe("lama");
    // single-flight dilepas: refresh berikutnya berjalan dan berhasil.
    expect(await auth.refresh()).toBe("ok");
    expect(auth.getAccessToken()).toBe("t-setelah-timeout");
  });

  test("boot dengan refresh menggantung berakhir sebagai anonymous setelah timeout", async () => {
    const auth = createAuthController({ fetch: hanging, refreshTimeoutMs: 20 });
    await auth.boot();
    expect(auth.getState()).toEqual({ status: "anonymous" });
  });
});
