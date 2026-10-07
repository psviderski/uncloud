import { existsSync, statSync } from "node:fs";
import { readFile } from "node:fs/promises";
import path from "node:path";

const TYPES: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".webp": "image/webp",
};

/**
 * Dev-only: serve the static landing page (./landing) at `/` and `/hub` like the production Caddy does.
 * In production the Dockerfile copies the landing files over the Blume build output, so this changes nothing there.
 */
export function landingDev() {
  const dir = path.resolve("landing");
  return {
    name: "landing-dev",
    hooks: {
      "astro:server:setup": ({ server }: { server: any }) => {
        server.middlewares.use(async (req: any, res: any, next: () => void) => {
          const url = new URL(req.url ?? "/", "http://localhost").pathname;
          let rel: string | null = null;
          if (url === "/") rel = "index.html";
          else if (url === "/hub") rel = "hub.html";
          else if (url === "/style.css" || /^\/(images|js)\//.test(url)) rel = url.slice(1);
          if (!rel) return next();
          const file = path.join(dir, rel);
          if (!file.startsWith(dir) || !existsSync(file) || !statSync(file).isFile()) return next();
          res.setHeader("Content-Type", TYPES[path.extname(file)] ?? "application/octet-stream");
          res.end(await readFile(file));
        });
      },
    },
  };
}
