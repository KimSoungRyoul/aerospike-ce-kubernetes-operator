// @ts-check

import {themes as prismThemes} from 'prism-react-renderer';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'ACKO',
  tagline: 'Aerospike CE Kubernetes Operator',
  favicon: 'img/favicon.ico',
  url: process.env.DOCS_URL || 'https://aerospike-ce-ecosystem.github.io',
  baseUrl: '/aerospike-ce-kubernetes-operator/',

  organizationName: process.env.ORG_NAME || 'aerospike-ce-ecosystem',
  projectName: 'aerospike-ce-kubernetes-operator',

  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'throw',

  i18n: {
    defaultLocale: 'en',
    locales: ['en', 'ko'],
    localeConfigs: {
      en: { label: 'English' },
      ko: { label: '한국어' },
    },
  },

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          path: './content',
          routeBasePath: '/',
          sidebarPath: './sidebars.js',
          editUrl:
            'https://github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/tree/main/docs/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      image: 'img/social-preview.png',
      colorMode: {
        respectPrefersColorScheme: true,
      },
      navbar: {
        logo: {
          alt: 'ACKO — Aerospike CE Kubernetes Operator',
          src: 'img/logo.svg',
          srcDark: 'img/logo-dark.svg',
        },
        items: [
          {
            type: 'docSidebar',
            sidebarId: 'docsSidebar',
            position: 'left',
            label: 'Docs',
          },
          {
            type: 'localeDropdown',
            position: 'right',
          },
          {
            href: 'https://github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Docs',
            items: [
              { label: 'Quick Start', to: '/' },
              { label: 'Installation', to: '/getting-started/install' },
              { label: 'API Reference', to: '/reference/api-reference/aerospikecluster' },
            ],
          },
          {
            title: 'More',
            items: [
              {
                label: 'GitHub',
                href: 'https://github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator',
              },
              {
                label: 'Aerospike',
                href: 'https://aerospike.com/',
              },
            ],
          },
        ],
        copyright: `Copyright \u00a9 ${new Date().getFullYear()} ACKO Project. Licensed under Apache 2.0.`,
      },
      prism: {
        theme: prismThemes.github,
        darkTheme: prismThemes.dracula,
        additionalLanguages: ['bash', 'yaml', 'go'],
      },
    }),
};

export default config;
