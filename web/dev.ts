// Catatan: halaman disajikan Bun apa adanya; skrip tema pra-paint (src/theme-init.ts) hanya disuntikkan oleh build produksi
// (build.ts), sehingga di dev tema eksplisit yang berbeda dari OS dapat berkedip sesaat (lihat README, "Batas dev server").
import index from "./src/index.html";

const API = "http://127.0.0.1:8080";

Bun.serve({
  hostname: "127.0.0.1", // jangan buka proxy API loopback ke LAN
  port: 5173,
  routes: {
    "/*": index,
    "/api/*": (req) => {
      const url = new URL(req.url);
      return fetch(API + url.pathname + url.search, {
        method: req.method,
        headers: req.headers,
        body: req.body,
      });
    },
  },
  development: true,
});
console.log("web dev server: http://127.0.0.1:5173");
