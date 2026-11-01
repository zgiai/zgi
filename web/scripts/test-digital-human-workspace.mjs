import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { loadTsModule } from './lib/load-ts-module.mjs';

// DigitalHumans follow the current workspace: switching workspaces refreshes
// the list and leaves a detail page, and the detail shows only what the
// person's workspace role allows.

const read = path => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8');

const { getWorkspaceSwitchRedirect } = await loadTsModule('utils/workspace-route-reset.ts');
for (const path of [
  '/console/digital-humans/dh-1',
  '/console/digital-humans/dh-1/runs',
  '/console/digital-humans/dh-1/external/keys',
]) {
  assert.equal(getWorkspaceSwitchRedirect(path), '/console/digital-humans', path);
}
assert.equal(getWorkspaceSwitchRedirect('/console/digital-humans'), null);

const switchWorkspace = read('hooks/workspace/use-update-current-workspace.ts');
assert.match(switchWorkspace, /removeQueries\(\{ queryKey: DIGITAL_HUMAN_KEYS\.details\(\) \}\)/);
assert.match(switchWorkspace, /invalidateQueries\(\{ queryKey: DIGITAL_HUMAN_KEYS\.all \}\)/);

const list = read('components/digital-human/digital-human-list.tsx');
assert.match(
  list,
  /hasPermission\('digital_human\.delete'\)/,
  'deleting needs the delete permission'
);
assert.match(list, /\{canDeleteDigitalHuman \? \(\s*<DropdownMenuItem/);

const sidebar = read('components/digital-human/digital-human-sidebar.tsx');
assert.match(
  sidebar,
  /hasPermission\('digital_human\.logs\.view'\)[\s\S]{0,200}tabs\.runs/,
  'the runs tab needs the logs permission'
);
assert.match(
  sidebar,
  /hasPermission\('digital_human\.publish'\)[\s\S]{0,200}tabs\.apiKeys/,
  'the API keys tab needs the publish permission'
);

const layout = read('app/console/digital-humans/[digitalHumanId]/layout.tsx');
assert.match(layout, /detailQuery\.isError && !detailQuery\.data/);
assert.match(layout, /detailUnavailable\.backToList/);
assert.match(
  read('app/console/digital-humans/[digitalHumanId]/external/keys/page.tsx'),
  /permission="digital_human\.publish"/
);

for (const locale of ['zh-Hans', 'en-US']) {
  assert.match(read(`i18n/modules/digitalHumans/${locale}.ts`), /detailUnavailable: \{/, locale);
}

console.log('DigitalHuman workspace switching and permissions: ok');
