import index from "./src/index.html";

const API = "http://127.0.0.1:8080";

Bun.serve({
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
