import { existsSync, mkdirSync, rmSync, cpSync, writeFileSync } from 'node:fs';
import { basename, dirname, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(__dirname, '../..');
const generatedRoot = resolve(webRoot, 'src/customer/generated');
const generatedActive = resolve(generatedRoot, 'active.ts');
const generatedCustomization = resolve(generatedRoot, 'customization');
const publicCustomerRoot = resolve(webRoot, 'public/customer');
const tmpRoot = resolve(webRoot, '.customer-tmp');

const repo = process.env.CUSTOMER_REPO?.trim();
const ref = process.env.CUSTOMER_REF?.trim() || 'main';
const configuredPath = process.env.CUSTOMER_PATH?.trim();
const required = ['1', 'true', 'yes'].includes(
  (process.env.CUSTOMER_REQUIRED || '').trim().toLowerCase()
);
const preserveGenerated = ['1', 'true', 'yes'].includes(
  (process.env.CUSTOMER_PRESERVE_GENERATED || '').trim().toLowerCase()
);

function fail(message) {
  if (required) {
    throw new Error(message);
  }
  console.warn(`[customer:prepare] ${message}`);
  writeDefaultActive();
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    stdio: 'inherit',
    shell: false,
    ...options,
  });
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(' ')} exited with code ${result.status}`);
  }
}

function writeDefaultActive() {
  rmSync(generatedRoot, { recursive: true, force: true });
  mkdirSync(generatedRoot, { recursive: true });
  rmSync(publicCustomerRoot, { recursive: true, force: true });
  writeFileSync(
    generatedActive,
    "export { defaultCustomerAdapter as customerAdapter } from '../default';\n"
  );
}

function shouldCopySource(source) {
  const name = basename(source);
  return !['.git', 'node_modules', '.DS_Store'].includes(name);
}

function copyAssets(sourceDir, slug) {
  const targetDir = resolve(publicCustomerRoot, slug);
  rmSync(targetDir, { recursive: true, force: true });

  const assetsDir = resolve(sourceDir, 'assets');
  if (!existsSync(assetsDir)) return;

  mkdirSync(targetDir, { recursive: true });
  cpSync(assetsDir, targetDir, { recursive: true, filter: shouldCopySource });
}

function resolveSourceDir() {
  if (configuredPath) {
    return resolve(configuredPath);
  }

  if (!repo) {
    return null;
  }

  rmSync(tmpRoot, { recursive: true, force: true });
  run('git', ['clone', '--depth', '1', '--branch', ref, repo, tmpRoot], { cwd: webRoot });

  const repoPath = process.env.CUSTOMER_REPO_PATH?.trim();
  return repoPath ? resolve(tmpRoot, repoPath) : tmpRoot;
}

const sourceDir = resolveSourceDir();
if (!sourceDir) {
  if (preserveGenerated && existsSync(generatedActive)) {
    console.log(
      '[customer:prepare] No customer customization configured, preserving existing generated adapter.'
    );
    process.exit(0);
  }
  fail('No customer customization configured');
  console.log('[customer:prepare] Using default adapter.');
  writeDefaultActive();
  process.exit(0);
}

if (!existsSync(sourceDir)) {
  fail(`Customization path does not exist: ${sourceDir}`);
  process.exit(0);
}

const sourceActive = resolve(sourceDir, 'active.tsx');
const sourceActiveTs = resolve(sourceDir, 'active.ts');
if (!existsSync(sourceActive) && !existsSync(sourceActiveTs)) {
  fail(`Customization must provide active.ts or active.tsx: ${sourceDir}`);
  process.exit(0);
}

const slug = (process.env.CUSTOMER_SLUG?.trim() || basename(sourceDir)).replace(
  /[^a-zA-Z0-9_-]/g,
  '-'
);

rmSync(generatedRoot, { recursive: true, force: true });
mkdirSync(generatedRoot, { recursive: true });
cpSync(sourceDir, generatedCustomization, { recursive: true, filter: shouldCopySource });
copyAssets(sourceDir, slug);

writeFileSync(
  generatedActive,
  "export { customerAdapter } from './customization/active';\n"
);

console.log(`[customer:prepare] Loaded customization from ${sourceDir}`);
console.log(`[customer:prepare] Public assets copied to /customer/${slug}`);
