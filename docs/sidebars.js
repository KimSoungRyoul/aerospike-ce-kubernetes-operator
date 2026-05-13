// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docsSidebar: [
    'quickstart',
    {
      type: 'category',
      label: 'Getting Started',
      collapsed: false,
      items: [
        'getting-started/install',
        'getting-started/create-cluster',
        'getting-started/cluster-manager-ui',
        'getting-started/ackoctl-cli',
      ],
    },
    {
      type: 'category',
      label: 'Cluster Operations',
      items: [
        'operations/manage-cluster',
        'operations/operations',
        'operations/monitoring',
        'operations/troubleshooting',
      ],
    },
    {
      type: 'category',
      label: 'Configuration',
      items: [
        'configuration/storage',
        'configuration/networking',
        'configuration/access-control',
        'configuration/advanced-configuration',
        'configuration/templates',
      ],
    },
    {
      type: 'category',
      label: 'Reference',
      items: [
        'reference/helm-values',
        'reference/glossary',
        {
          type: 'category',
          label: 'API Reference',
          items: [
            'reference/api-reference/aerospikecluster',
            'reference/api-reference/aerospikeclustertemplate',
          ],
        },
      ],
    },
  ],
};

export default sidebars;
