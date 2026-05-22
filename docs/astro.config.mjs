// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
//
// Deployed to GitHub Pages at https://elvonpiko.github.io/tunnd/.
// To switch to a custom domain (e.g. tunnd.sh), change `site` to the new
// origin, drop `base` (or set it to '/'), and put the domain into
// public/CNAME.
export default defineConfig({
  site: 'https://elvonpiko.github.io',
  base: '/tunnd',
  integrations: [
    starlight({
      title: 'Tunnd',
      description: 'Self-hosted tunnel server — expose localhost to the internet.',
      logo: {
        light: './src/assets/logo.svg',
        dark: './src/assets/logo.svg',
        replacesTitle: false,
      },
      favicon: '/favicon.svg',
      customCss: ['./src/styles/tunnd.css'],
      head: [
        {
          tag: 'meta',
          attrs: { property: 'og:image', content: 'https://elvonpiko.github.io/tunnd/og.png' },
        },
        {
          tag: 'link',
          attrs: {
            rel: 'preconnect',
            href: 'https://fonts.googleapis.com',
          },
        },
        {
          tag: 'link',
          attrs: {
            rel: 'preconnect',
            href: 'https://fonts.gstatic.com',
            crossorigin: '',
          },
        },
        {
          tag: 'link',
          attrs: {
            rel: 'stylesheet',
            href: 'https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&family=Geist+Mono:wght@400;500;600;700&display=swap',
          },
        },
      ],
      social: {
        github: 'https://github.com/elvonpiko/tunnd',
      },
      editLink: {
        baseUrl: 'https://github.com/elvonpiko/tunnd/edit/main/docs/',
      },
      lastUpdated: true,
      pagination: true,
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'Quick Start', slug: 'getting-started/quick-start' },
            { label: 'Client Installation', slug: 'getting-started/client-installation' },
            { label: 'Server Deployment', slug: 'getting-started/server-deployment' },
          ],
        },
        {
          label: 'Deployment',
          items: [
            { label: 'Docker', slug: 'deployment/docker' },
            { label: 'TLS Certificates', slug: 'deployment/tls-certificates' },
            { label: 'Caddy Reverse Proxy', slug: 'deployment/reverse-proxy/caddy' },
          ],
        },
        {
          label: 'Configuration',
          items: [
            { label: 'Server Config', slug: 'configuration/server-config' },
            { label: 'CLI Reference', slug: 'configuration/cli-reference' },
          ],
        },
        {
          label: 'Guides',
          items: [
            { label: 'Use Cases', slug: 'guides/use-cases' },
            { label: 'Custom Subdomains', slug: 'guides/custom-subdomains' },
            { label: 'Multiple Tunnels', slug: 'guides/multiple-tunnels' },
            { label: 'Security', slug: 'guides/security-best-practices' },
            { label: 'Troubleshooting', slug: 'guides/troubleshooting' },
          ],
        },
        {
          label: 'API Reference',
          collapsed: true,
          items: [
            { label: 'Admin API', slug: 'api/admin-api' },
            { label: 'WebSocket Protocol', slug: 'api/websocket-protocol' },
          ],
        },
        {
          label: 'Architecture',
          collapsed: true,
          items: [
            { label: 'Overview', slug: 'architecture/overview' },
            { label: 'Data Flow', slug: 'architecture/data-flow' },
          ],
        },
      ],
      components: {
        // we'll override these to match Tunnd branding
      },
    }),
  ],
});
