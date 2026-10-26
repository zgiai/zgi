import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

import {
  backgroundGrantQueryKey,
  buildBackgroundGrantIssueRequest,
  grantsForEmployee,
} from '../src/components/digital-human/digital-human-background-grants-model.ts';

const scope = {
  organizationId: 'org-1',
  workspaceId: 'workspace-1',
  accountId: 'account-1',
  employeeId: 'employee-1',
  versionId: 'version-2',
};
assert.deepEqual(backgroundGrantQueryKey(scope), [
  'connector-background-grants',
  'org-1',
  'workspace-1',
  'account-1',
  'employee-1',
  'version-2',
]);

const now = new Date('2026-10-20T00:00:00.000Z');
const valid = buildBackgroundGrantIssueRequest(
  {
    operations: ['x-social.post.create'],
    destinationRows: [{ field: 'account_id', value: 'page-1' }],
    durationDays: 7,
    maxInvocations: 30,
  },
  now
);
assert.deepEqual(valid, {
  request: {
    operations: ['x-social.post.create'],
    destinations: { account_id: 'page-1' },
    expires_at: '2026-10-27T00:00:00.000Z',
    max_invocations: 30,
  },
});

for (const [input, error] of [
  [{ operations: [], destinationRows: [], durationDays: 7, maxInvocations: 30 }, 'operations'],
  [
    {
      operations: ['x-social.post.create'],
      destinationRows: [],
      durationDays: 91,
      maxInvocations: 30,
    },
    'duration',
  ],
  [
    {
      operations: ['x-social.post.create'],
      destinationRows: [{ field: 'account_id', value: '*' }],
      durationDays: 7,
      maxInvocations: 30,
    },
    'destinations',
  ],
  [
    {
      operations: ['x-social.post.create'],
      destinationRows: [],
      durationDays: 7,
      maxInvocations: 100001,
    },
    'maxInvocations',
  ],
]) {
  assert.deepEqual(buildBackgroundGrantIssueRequest(input, now), { error });
}

const grant = {
  id: 'grant-current',
  employee_id: 'employee-1',
  version_id: 'version-2',
  binding_id: 'binding-1',
  connection_id: 'connection-1',
  operations: ['x-social.post.create'],
  destinations: {},
  expires_at: '2026-10-21T00:00:00.000Z',
  max_invocations: 30,
  used_invocations: 3,
  created_at: '2026-10-19T00:00:00.000Z',
  binding_revision: 4,
  connection_revision: 5,
  credential_version: 6,
  contract_digest: 'a'.repeat(64),
};
const oldVersion = { ...grant, id: 'grant-old', version_id: 'version-1' };
const foreignEmployee = { ...grant, id: 'grant-foreign', employee_id: 'employee-2' };
const rows = grantsForEmployee(
  [foreignEmployee, oldVersion, grant],
  scope,
  [{ id: 'binding-1', connection_id: 'connection-1', revision: 4, status: 'active' }],
  [{ id: 'connection-1', revision: 5, credential_version: 6 }],
  now
);
assert.deepEqual(
  rows.map(row => [row.grant.id, row.stale, row.expired, row.revoked]),
  [
    ['grant-current', false, false, false],
    ['grant-old', true, false, false],
  ]
);
assert.equal(
  grantsForEmployee(
    [grant],
    scope,
    [{ id: 'binding-1', connection_id: 'connection-1', revision: 5, status: 'active' }],
    [{ id: 'connection-1', revision: 5, credential_version: 6 }],
    now
  )[0].stale,
  true
);

const componentSource = await readFile(
  new URL('../src/components/digital-human/digital-human-background-grants.tsx', import.meta.url),
  'utf8'
);
const hookSource = await readFile(
  new URL('../src/hooks/digital-human/use-digital-human-background-grants.ts', import.meta.url),
  'utf8'
);
const sectionSource = await readFile(
  new URL('../src/components/digital-human/digital-human-connector-section.tsx', import.meta.url),
  'utf8'
);
assert.match(sectionSource, /bindings=\{publishedBindings\}/);
assert.match(componentSource, /const \[operations, setOperations\] = useState<string\[]>\(\[\]\)/);
assert.match(componentSource, /canCreate \? \(/);
assert.doesNotMatch(componentSource, /<textarea|JSON\.stringify/);
assert.equal((hookSource.match(/retry: false/g) ?? []).length, 3);
assert.match(hookSource, /backgroundGrantQueryKey\(scope\)/);

console.log('Digital human background grant model and scope contracts passed');
