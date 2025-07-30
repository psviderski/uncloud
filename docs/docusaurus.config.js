// @ts-check
// `@type` JSDoc annotations allow editor autocompletion and type checking
// (when paired with `@ts-check`).
// There are various equivalent ways to declare your Docusaurus config.
// See: https://docusaurus.io/docs/api/docusaurus-config

import {themes as prismThemes} from 'prism-react-renderer';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Uncloud',
  tagline: 'Self-host and scale web apps without Kubernetes complexity',
  favicon: 'img/favicon.png',

  // Set the production url of your site here
  url: 'https://uncloud.run',
  // Set the /<baseUrl>/ pathname under which your site is served
  // For GitHub pages deployment, it is often '/<projectName>/'
  baseUrl: '/',

  // GitHub pages deployment config.
  // If you aren't using GitHub pages, you don't need these.
  organizationName: '', // Usually your GitHub org/user name.
  projectName: '', // Usually your repo name.

  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'warn',

  // Even if you don't use internationalization, you can use this field to set
  // useful metadata like html lang. For example, if your site is Chinese, you
  // may want to replace "en" with "zh-Hans".
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  themes: [
    [
      '@easyops-cn/docusaurus-search-local',
      /** @type {import("@easyops-cn/docusaurus-search-local").PluginOptions} */
      ({
        docsRouteBasePath: '/',
        hashed: true,
        highlightSearchTermsOnTargetPage: true,
      }),
    ],
  ],
  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          // Remove this to remove the "edit this page" links.
          editUrl: 'https://github.com/psviderski/uncloud/edit/main/docs/',
          // Serve the docs at the site's root.
          // routeBasePath: '/',
          showLastUpdateTime: true,
          sidebarPath: './sidebars.js',
        },
        // blog: {
        //   showReadingTime: true,
        //   feedOptions: {
        //     type: ['rss', 'atom'],
        //     xslt: true,
        //   },
        //   // Please change this to your repo.
        //   // Remove this to remove the "edit this page" links.
        //   // editUrl:
        //   //   'https://github.com/facebook/docusaurus/tree/main/packages/create-docusaurus/templates/shared/',
        //   // Useful options to enforce blogging best practices
        //   onInlineTags: 'warn',
        //   onInlineAuthors: 'warn',
        //   onUntruncatedBlogPosts: 'warn',
        // },
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      colorMode: {
        // The color mode when user first visits the site.
        defaultMode: "light",
        // Whether to use the prefers-color-scheme media-query, using user system preferences,
        // instead of the hardcoded defaultMode.
        respectPrefersColorScheme: true,
      },
      // The meta image URL for the site (og:image and twitter:image meta tags).
      // Relative to your site's "static" directory. Cannot be SVGs. Can be external URLs too.
      image: 'img/social-card.png',
      navbar: {
        title: 'Uncloud',
        logo: {
          alt: 'Uncloud Logo',
          src: 'img/logo.svg',
        },
        items: [
          {
            type: 'doc',
            docId: 'overview',
            label: 'Docs',
            position: 'left',
          },
          {
            to: 'blog',
            label: 'Blog',
            position: 'left'},
          {
            type: 'search',
            position: 'right',
          },
          {
            href: 'https://github.com/psviderski/uncloud',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'light',
        links: [
          {
            title: 'Uncloud',
            items: [
              {
                html: 'Made with ❤️ by <a href="https://github.com/psviderski">@psviderski</a> in Australia'
              },
            ],
          },
          {
            title: 'Community',
            items: [
              {
                label: 'Discord',
                href: 'https://discord.gg/eR35KQJhPu',
              },
              {
                label: 'X',
                href: 'https://x.com/psviderski',
              },
              {
                label: 'GitHub',
                href: 'https://github.com/psviderski/uncloud',
              },
            ],
          }
        ]
      },
      prism: {
        theme: prismThemes.palenight,
        //darkTheme: prismThemes.dracula,
      },
    }),

    plugins: [
        [
            '@docusaurus/plugin-client-redirects',
            {
                redirects: [
                    {
                        from: '/',
                        to: '/new-index.html',
                    },
                ],
            },
        ],
    ],
};

export default config;
