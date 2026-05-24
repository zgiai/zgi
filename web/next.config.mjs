import createNextIntlPlugin from 'next-intl/plugin';
import { withSentryConfig } from '@sentry/nextjs';
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

function readPositiveIntegerEnv(name) {
  const raw = process.env[name];
  if (!raw) return undefined;
  const value = Number.parseInt(raw, 10);
  return Number.isFinite(value) && value > 0 ? value : undefined;
}

function readBooleanEnv(name) {
  const raw = process.env[name];
  if (raw === undefined) return undefined;
  const value = raw.trim().toLowerCase();
  if (['1', 'true', 'yes', 'on'].includes(value)) return true;
  if (['0', 'false', 'no', 'off'].includes(value)) return false;
  return undefined;
}

const basePath = normalizeBasePath(rawBasePath);
const staticAppName = (process.env.NEXT_PUBLIC_APP_NAME ?? 'ZGI').trim() || 'ZGI';
const staticBrandName = (process.env.NEXT_PUBLIC_BRAND_NAME ?? staticAppName).trim() || staticAppName;
const buildCpus = readPositiveIntegerEnv('NEXT_BUILD_CPUS');
const staticGenerationMaxConcurrency = readPositiveIntegerEnv(
  'NEXT_STATIC_GENERATION_MAX_CONCURRENCY'
);
const turbopackMemoryLimitMb = readPositiveIntegerEnv('NEXT_TURBOPACK_MEMORY_LIMIT_MB');
const webpackMemoryOptimizations = readBooleanEnv('NEXT_WEBPACK_MEMORY_OPTIMIZATIONS');

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  env: {
    APP_NAME_STATIC: staticAppName,
    APP_BRAND_STATIC: staticBrandName,
  },

  typescript: { ignoreBuildErrors: true },
  serverExternalPackages: ['@antv/g6'],

  experimental: {
    optimizePackageImports: ['@radix-ui/react-icons', 'lucide-react', 'antd'],
    serverSourceMaps: false,
    ...(buildCpus
      ? {
          cpus: buildCpus,
          memoryBasedWorkersCount: false,
        }
      : {}),
    ...(staticGenerationMaxConcurrency
      ? { staticGenerationMaxConcurrency }
      : {}),
    ...(turbopackMemoryLimitMb
      ? { turbopackMemoryLimit: turbopackMemoryLimitMb * 1024 * 1024 }
      : {}),
    ...(webpackMemoryOptimizations === undefined ? {} : { webpackMemoryOptimizations }),
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

const withPlugins = withNextIntl(nextConfig);

export default withSentryConfig(withPlugins, {
  org: process.env.SENTRY_ORG,
  project: process.env.SENTRY_PROJECT,
  authToken: process.env.SENTRY_AUTH_TOKEN,
  silent: !process.env.CI,
  widenClientFileUpload: Boolean(process.env.SENTRY_AUTH_TOKEN),
  webpack: {
    treeshake: {
      removeDebugLogging: true,
    },
  },
});
