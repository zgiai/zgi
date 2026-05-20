import { cp, mkdir, rm, stat, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const generatedCustomerDir = path.join(repoRoot, '.customer');
const generatedCustomerRoutesDir = path.join(repoRoot, 'src/app/console/(customer)');
const generatedDashboardCustomerRoutesDir = path.join(repoRoot, 'src/app/dashboard/(customer)');

function readOverlayDirArg() {
  const overlayArgIndex = process.argv.findIndex(arg => arg === '--overlay');
  if (overlayArgIndex >= 0) {
    return process.argv[overlayArgIndex + 1] || null;
  }

  const overlayArg = process.argv.find(arg => arg.startsWith('--overlay='));
  return overlayArg ? overlayArg.slice('--overlay='.length) : null;
}

const configuredOverlayDir = readOverlayDirArg() || process.env.CUSTOMER_OVERLAY_DIR || null;
const overlayRoot = configuredOverlayDir ? path.resolve(repoRoot, configuredOverlayDir) : null;

async function pathExists(targetPath) {
  try {
    await stat(targetPath);
    return true;
  } catch (error) {
    if (error?.code === 'ENOENT') {
      return false;
    }
    throw error;
  }
}

async function copyIfExists(sourcePath, targetPath) {
  if (!(await pathExists(sourcePath))) {
    return false;
  }

  await mkdir(path.dirname(targetPath), { recursive: true });
  await cp(sourcePath, targetPath, { recursive: true });
  return true;
}

async function resetGeneratedTargets() {
  await rm(generatedCustomerDir, { recursive: true, force: true });
  await rm(generatedCustomerRoutesDir, { recursive: true, force: true });
  await rm(generatedDashboardCustomerRoutesDir, { recursive: true, force: true });
  await mkdir(generatedCustomerDir, { recursive: true });
}

async function prepareDefaultCustomer() {
  await writeFile(
    path.join(generatedCustomerDir, 'active.ts'),
    "export { defaultCustomerAdapter as customerAdapter } from '../src/customer/default';\n",
    'utf8'
  );
  console.log('[customer-overlay] prepared default customer adapter');
}

async function prepareOverlayCustomer() {
  if (!(await pathExists(overlayRoot))) {
    throw new Error(`CUSTOMER_OVERLAY_DIR does not exist: ${overlayRoot}`);
  }

  await cp(overlayRoot, path.join(generatedCustomerDir, 'src'), {
    recursive: true,
    filter(sourcePath) {
      const basename = path.basename(sourcePath);
      return basename !== 'node_modules' && basename !== '.git' && basename !== '.next';
    },
  });

  const hasOverlayActive =
    (await pathExists(path.join(generatedCustomerDir, 'src/active.ts'))) ||
    (await pathExists(path.join(generatedCustomerDir, 'src/active.tsx')));
  if (!hasOverlayActive) {
    throw new Error('Customer overlay must provide active.ts or active.tsx exporting customerAdapter');
  }

  await writeFile(
    path.join(generatedCustomerDir, 'active.ts'),
    "export { customerAdapter } from './src/active';\n",
    'utf8'
  );

  const routeSources = [
    path.join(overlayRoot, 'src/app/console/(customer)'),
    path.join(overlayRoot, 'app/console/(customer)'),
  ];

  for (const routeSource of routeSources) {
    if (await copyIfExists(routeSource, generatedCustomerRoutesDir)) {
      break;
    }
  }

  const dashboardRouteSources = [
    path.join(overlayRoot, 'src/app/dashboard/(customer)'),
    path.join(overlayRoot, 'app/dashboard/(customer)'),
  ];

  for (const routeSource of dashboardRouteSources) {
    if (await copyIfExists(routeSource, generatedDashboardCustomerRoutesDir)) {
      break;
    }
  }

  console.log(`[customer-overlay] prepared overlay customer adapter from ${overlayRoot}`);
}

await resetGeneratedTargets();

if (overlayRoot) {
  await prepareOverlayCustomer();
} else {
  await prepareDefaultCustomer();
}
