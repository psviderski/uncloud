import { defineConfig } from "blume";
import { posthog } from "blume/analytics";
import { landingDev } from "./integrations/landing-dev.ts";

export default defineConfig({
  title: "Uncloud",
  description: "Self-host and scale web apps without Kubernetes complexity",
  logo: "/img/logo.svg",
  theme: {
    accent: { light: "#006be6", dark: "#3391ee" },
    fonts: { body: "inter", display: "inter" },
  },
  content: {
    // Docs and blog sit directly under the website dir, so scope the scan to them.
    root: ".",
    include: ["docs/**/*.{md,mdx}", "blog/**/*.{md,mdx}"],
  },
  navigation: {
    sidebar: { display: "group" },
    tabs: [
      { label: "Docs", path: "/docs" },
      { label: "Blog", path: "/blog" },
    ],
  },
  markdown: {
    code: {
      theme: { light: "material-theme-palenight", dark: "material-theme-palenight" },
    },
  },
  github: { owner: "psviderski", repo: "uncloud", branch: "main", dir: "website" },
  footer: {
    links: [
      { label: "Discord", href: "https://discord.gg/eR35KQJhPu" },
      { label: "X", href: "https://x.com/psviderski" },
    ],
  },
  lastModified: "git",
  deployment: { site: "https://uncloud.run" },
  analytics: [
    posthog({
      key: "phc_nsuhPtAsiYAFiSYmc2KwA5Homz6miXWjf3Hy4J4H3QMV",
      // Send events through our own domain (see Caddyfile) so ad blockers do not drop them.
      host: "https://uncloud.run/phproxy",
      ui_host: "https://us.posthog.com",
      defaults: "2026-05-30",
      person_profiles: "identified_only",
    }),
  ],
  integrations: [landingDev()],
  redirects: [
    // Docusaurus generated-index category pages are now regular folder index pages.
    { from: "/docs/category/cli-reference", to: "/docs/cli-reference" },
    { from: "/docs/category/clusters", to: "/docs/concepts/clusters" },
    { from: "/docs/category/compose-file-reference", to: "/docs/compose-file-reference" },
    { from: "/docs/category/concepts", to: "/docs/concepts" },
    { from: "/docs/category/deployments", to: "/docs/guides/deployments" },
    { from: "/docs/category/getting-started", to: "/docs/getting-started" },
    { from: "/docs/category/guides", to: "/docs/guides" },
    { from: "/docs/category/ingress--https", to: "/docs/concepts/ingress" },
    { from: "/docs/category/services", to: "/docs/concepts/services" },
    // Docusaurus blog tag, author and archive pages have no Blume equivalent.
    { from: "/blog/archive", to: "/blog" },
    { from: "/blog/authors", to: "/blog" },
    { from: "/blog/tags", to: "/blog" },
    { from: "/blog/tags/:tag", to: "/blog" },
  ],
});
