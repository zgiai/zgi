import createNextIntlPlugin from 'next-intl/plugin';
import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const withNextIntl = createNextIntlPlugin('./src/i18n/request.ts');

function readBasePathFromEnvFiles() {
  const envFiles = ['.env.local', '.env'];
  for (const filename of envFiles) {
    const fullPath = resolve(process.cwd(), filename);
    if (!existsSync(fullPath)) continue;
    const content = readFileSync(fullPath, 'utf8');
    const lines = content.split(/\r?\n/);
    for (const line of lines) {
      const match = line.match(/^\s*(NEXT_PUBLIC_BASE_PATH|BASE_PATH)\s*=\s*(.*)\s*$/);
      if (!match) continue;
      const rawValue = match[2]?.trim() ?? '';
      const unquoted = rawValue.replace(/^['"]|['"]$/g, '');
      if (unquoted) return unquoted;
    }
  }
  return '';
}

const rawBasePath =
  process.env.NEXT_PUBLIC_BASE_PATH ?? process.env.BASE_PATH ?? readBasePathFromEnvFiles();

function normalizeBasePath(basePath) {
  if (!basePath) return '';
  const trimmed = String(basePath).trim();
  if (!trimmed || trimmed === '/') return '';
  const prefixed = trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
  return prefixed.replace(/\/+$/, '');
}

const basePath = normalizeBasePath(rawBasePath);
const staticAppName = (process.env.NEXT_PUBLIC_APP_NAME ?? 'ZGI').trim() || 'ZGI';
const staticBrandName = (process.env.NEXT_PUBLIC_BRAND_NAME ?? staticAppName).trim() || staticAppName;

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  async rewrites() {
    return [
      {
        source: '/market-api/:path*',
        destination: 'http://localhost:8025/:path*',
      },
    ];
  },
  env: {
    APP_NAME_STATIC: staticAppName,
    APP_BRAND_STATIC: staticBrandName,
  },

  typescript: { ignoreBuildErrors: true },
  serverExternalPackages: ['@antv/g6'],

  experimental: {
    optimizePackageImports: ['@radix-ui/react-icons', 'lucide-react', 'antd'],
    serverSourceMaps: false,
  },
  productionBrowserSourceMaps: false,
  pageExtensions: ['js', 'jsx', 'md', 'mdx', 'ts', 'tsx'],
  turbopack: {
    rules: {
      '*.mdx': {
        loaders: ['@mdx-js/loader'],
        as: '*.js',
      },
    },
    resolveExtensions: [
      '.mdx',
      '.tsx',
      '.ts',
      '.jsx',
      '.js',
      '.mjs',
      '.json',
    ],
  },
  ...(basePath
    ? {
        basePath,
        assetPrefix: basePath,
      }
    : {}),
};

export default withNextIntl(nextConfig)
