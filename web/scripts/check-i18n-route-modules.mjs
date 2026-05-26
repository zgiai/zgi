import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(__dirname, '..');
const srcDir = path.join(rootDir, 'src');
const routeModulesPath = path.join(srcDir, 'i18n', 'route-modules.ts');
const loaderPath = path.join(srcDir, 'i18n', 'loader.ts');

const GROUP_SOURCES = {
  public: 'PUBLIC_MODULES',
  auth: 'AUTH_MODULES',
  console: 'CONSOLE_MODULES',
  dashboard: 'DASHBOARD_MODULES',
  webapp: 'WEBAPP_MODULES',
  appToken: 'APP_TOKEN_MODULES',
  announcementToken: 'ANNOUNCEMENT_TOKEN_MODULES',
  profile: 'PROFILE_MODULES',
};

const GROUP_OWNERS = [
  {
    group: 'auth',
    prefixes: [
      'src/app/(auth)/',
      'src/components/auth/',
      'src/hooks/auth/',
      'src/hooks/use-setup.ts',
      'src/components/activate-form.tsx',
    ],
  },
  {
    group: 'console',
    prefixes: [
      'src/app/console/',
      'src/components/console/',
      'src/hooks/workspace/',
      'src/hooks/workspace-quota/',
    ],
  },
  {
    group: 'dashboard',
    prefixes: [
      'src/app/dashboard/',
      'src/components/dashboard/',
      'src/components/market/',
      'src/hooks/aichat/',
      'src/hooks/channel/',
      'src/hooks/organization/',
      'src/hooks/pay/',
    ],
  },
  {
    group: 'webapp',
    prefixes: [
      'src/app/webapp/',
      'src/components/webapp/',
      'src/hooks/webapp/',
      'src/components/workflow/approval/',
      'src/components/workflow/question-answer/',
      'src/components/workflow/ui/workflow-chat-panel/',
      'src/components/workflow/ui/workflow-run-panel/',
    ],
  },
  {
    group: 'appToken',
    prefixes: ['src/app/a/'],
  },
  {
    group: 'announcementToken',
    prefixes: ['src/app/n/', 'src/components/common/markdown-viewer.tsx'],
  },
  {
    group: 'profile',
    prefixes: ['src/app/profile/', 'src/hooks/use-profile.ts'],
  },
  {
    group: 'public',
    prefixes: [
      'src/app/error.tsx',
      'src/app/global-error.tsx',
      'src/app/privacy/',
      'src/app/terms/',
    ],
  },
];

function read(filePath) {
  return fs.readFileSync(filePath, 'utf8');
}

function normalizePath(filePath) {
  return path.relative(rootDir, filePath).replaceAll(path.sep, '/');
}

function parseStringArray(source, constName) {
  const pattern = new RegExp(`const\\s+${constName}\\s*:[^=]+=[\\s\\S]*?\\];`, 'm');
  const match = source.match(pattern);
  if (!match) {
    throw new Error(`Could not find ${constName} in src/i18n/route-modules.ts`);
  }

  return Array.from(match[0].matchAll(/'([^']+)'/g), item => item[1]);
}

function parseAvailableModules() {
  const source = read(loaderPath);
  const match = source.match(/export\s+const\s+AVAILABLE_MODULES\s*=\s*\[([\s\S]*?)\]\s+as\s+const/);
  if (!match) {
    throw new Error('Could not find AVAILABLE_MODULES in src/i18n/loader.ts');
  }

  return new Set(Array.from(match[1].matchAll(/'([^']+)'/g), item => item[1]));
}

function parseRouteGroups() {
  const source = read(routeModulesPath);
  return Object.fromEntries(
    Object.entries(GROUP_SOURCES).map(([group, constName]) => [
      group,
      new Set(parseStringArray(source, constName)),
    ])
  );
}

function walkFiles(dir) {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (['.next', 'node_modules'].includes(entry.name)) continue;
      files.push(...walkFiles(fullPath));
    } else if (/\.(ts|tsx)$/.test(entry.name) && !entry.name.endsWith('.d.ts')) {
      files.push(fullPath);
    }
  }

  return files;
}

function inferGroups(relativePath) {
  return GROUP_OWNERS.filter(owner =>
    owner.prefixes.some(prefix => relativePath === prefix || relativePath.startsWith(prefix))
  ).map(owner => owner.group);
}

function findScopedTranslationUses(source) {
  const scopes = [];
  const pattern = /\b(?:useT|getT)\(\s*(['"])([^'"]+)\1\s*\)/g;
  let match;

  while ((match = pattern.exec(source))) {
    scopes.push(match[2]);
  }

  return scopes;
}

const availableModules = parseAvailableModules();
const routeGroups = parseRouteGroups();
const findings = [];

for (const filePath of walkFiles(srcDir)) {
  const relativePath = normalizePath(filePath);
  const groups = inferGroups(relativePath);
  if (groups.length === 0) continue;

  const scopes = findScopedTranslationUses(read(filePath));
  for (const scope of scopes) {
    const moduleName = scope.split('.')[0];
    if (!availableModules.has(moduleName)) continue;

    for (const group of groups) {
      if (!routeGroups[group]?.has(moduleName)) {
        findings.push({ relativePath, group, moduleName, scope });
      }
    }
  }
}

if (findings.length > 0) {
  console.error('i18n route module coverage check failed:');
  for (const finding of findings) {
    console.error(
      `- ${finding.relativePath}: useT('${finding.scope}') requires '${finding.moduleName}' in ${finding.group}`
    );
  }
  process.exit(1);
}

console.log('i18n route module coverage check passed.');
