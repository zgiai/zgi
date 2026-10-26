import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

import {
  buildSocialOperationsSettings,
  createSocialOperationsDraft,
  draftFromSocialOperationsConfiguration,
  initialVerifiedAccountId,
  isSupportedSocialOperationsHandoff,
  researchConnectionChoices,
  socialOperationsQueryKey,
  socialPlatformChoices,
} from '../src/components/digital-human/digital-human-social-operations-model.ts';

const scope = {
  organizationId: 'org-a',
  workspaceId: 'workspace-a',
  accountId: 'account-a',
  employeeId: 'employee-a',
  versionId: 'version-a',
};
assert.deepEqual(socialOperationsQueryKey(scope), [
  'digital-humans',
  'detail',
  'employee-a',
  'social-operations',
  'org-a',
  'workspace-a',
  'account-a',
  'version-a',
]);

const choices = socialPlatformChoices(
  'employee-a',
  'version-a',
  [
    {
      id: 'binding-live',
      connection_id: 'connection-x',
      connector_id: 'x-social',
      status: 'active',
      target: {
        type: 'digital_human',
        id: 'employee-a',
        scope: 'published',
        version_id: 'version-a',
      },
      revision: 1,
    },
    {
      id: 'binding-wrong-version',
      connection_id: 'connection-linkedin',
      connector_id: 'linkedin-social',
      status: 'active',
      target: {
        type: 'digital_human',
        id: 'employee-a',
        scope: 'published',
        version_id: 'version-old',
      },
      revision: 1,
    },
  ],
  [
    {
      id: 'connection-x',
      connector_id: 'x-social',
      name: 'Launch account',
      external_account_id: 'x-owner-1',
      state: 'ready',
      credential_configured: true,
      revision: 1,
      created_at: '',
      updated_at: '',
    },
    {
      id: 'connection-linkedin',
      connector_id: 'linkedin-social',
      name: 'Company',
      state: 'ready',
      credential_configured: true,
      revision: 1,
      created_at: '',
      updated_at: '',
    },
  ]
);
assert.deepEqual(
  choices.map(choice => choice.connectionId),
  ['connection-x']
);
assert.equal(choices[0].accountId, 'x-owner-1');
assert.equal(initialVerifiedAccountId('x', [{ id: 'x-owner-1' }]), 'x-owner-1');
assert.equal(initialVerifiedAccountId('facebook', [{ id: 'page-1' }]), 'page-1');
assert.equal(initialVerifiedAccountId('linkedin', [{ id: 'org-1' }]), '');
assert.equal(initialVerifiedAccountId('x', [{ id: 'x-1' }, { id: 'x-2' }]), '');
assert.equal(isSupportedSocialOperationsHandoff('feishu', 'feishu.message.send'), true);
assert.equal(
  isSupportedSocialOperationsHandoff('dingtalk', 'dingtalk.messages.send_group'),
  true
);
assert.equal(isSupportedSocialOperationsHandoff('gmail', 'gmail.message.send'), true);
assert.equal(isSupportedSocialOperationsHandoff('x-social', 'x-social.post.create'), false);
assert.equal(isSupportedSocialOperationsHandoff('slack', 'slack.message.send'), false);
assert.equal(isSupportedSocialOperationsHandoff('gmail', 'qqmail.message.send'), false);

const researchChoices = researchConnectionChoices(
  'employee-a',
  'version-a',
  [
    {
      id: 'binding-research',
      connection_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
      connector_id: 'web-search',
      status: 'active',
      target: {
        type: 'digital_human',
        id: 'employee-a',
        scope: 'published',
        version_id: 'version-a',
      },
      revision: 1,
    },
    {
      id: 'binding-stale-research',
      connection_id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
      connector_id: 'web-search',
      status: 'active',
      target: {
        type: 'digital_human',
        id: 'employee-a',
        scope: 'published',
        version_id: 'version-old',
      },
      revision: 1,
    },
  ],
  [
    {
      id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
      connector_id: 'web-search',
      name: 'Approved research',
      state: 'ready',
      usable_operation_ids: ['web.search', 'web.fetch'],
      credential_configured: true,
      revision: 1,
      created_at: '',
      updated_at: '',
    },
    {
      id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
      connector_id: 'web-search',
      name: 'Stale research',
      state: 'ready',
      usable_operation_ids: ['web.search', 'web.fetch'],
      credential_configured: true,
      revision: 1,
      created_at: '',
      updated_at: '',
    },
  ],
  [
    {
      id: 'web-search',
      plugin_id: 'web-search',
      name: 'Web search',
      description: '',
      key_fields: [],
      capabilities: [
        { id: 'web.search', name: 'Search', effect: 'external_read', usable: true },
        { id: 'web.fetch', name: 'Fetch', effect: 'external_read', usable: true },
      ],
      connection_summary: { count: 1 },
    },
  ]
);
assert.deepEqual(researchChoices, [
  {
    connectorId: 'web-search',
    connectionId: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
    label: 'Approved research',
  },
]);

const draft = createSocialOperationsDraft('Asia/Shanghai');
assert.equal(draft.internalReviewLanguage, 'zh-Hans');
assert.deepEqual(draft.research, { connectorId: '', connectionId: '' });
assert.deepEqual(draft.cadence, {
  dailyContentCron: '0 9 * * *',
  inboxCron: '*/20 * * * *',
  sevenDaySampleCron: '15 10 * * *',
  weeklyReviewCron: '0 16 * * 5',
});
draft.productName = 'Launchpad';
draft.productDescription = 'A focused release workflow.';
draft.audience = 'Product teams preparing a launch';
draft.sourceUrls = ['https://example.com/docs'];
draft.research = {
  connectorId: 'web-search',
  connectionId: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
};
draft.platforms = [
  {
    platform: 'linkedin',
    connectorId: 'linkedin-social',
    connectionId: 'connection-linkedin',
    accountId: 'urn:li:organization:123',
    maxDailyPublications: 2,
    capabilities: {
      account: 'required',
      post: 'required',
      readback: 'required',
      comment: 'required',
      inbox: 'required',
      metrics: 'required',
      images: 'required',
    },
  },
];
draft.handoff = {
  connectorId: 'slack',
  connectionId: 'connection-slack',
  operationId: 'slack.message.send',
  destination: 'channel-123',
};

const built = buildSocialOperationsSettings(draft);
assert.equal('error' in built, false);
assert.equal(built.settings.product.name, 'Launchpad');
assert.equal(built.settings.timezone, 'Asia/Shanghai');
assert.deepEqual(built.settings.research, {
  connector_id: 'web-search',
  connection_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
});
assert.equal(
  built.settings.platforms[0].capabilities.find(item => item.capability === 'inbox').mode,
  'optional',
  'LinkedIn company messaging must never be submitted as required'
);
assert.equal(built.settings.platforms[0].capabilities.length, 7);
assert.deepEqual(built.settings.sources, [{ url: 'https://example.com/docs' }]);

const restoredDraft = draftFromSocialOperationsConfiguration({
  ...built.settings,
  scope: {
    organization_id: 'org-a',
    workspace_id: 'workspace-a',
    owner_id: 'account-a',
    employee_id: 'employee-a',
  },
  employee_version_id: 'version-a',
  revision: 3,
  sources: [
    { url: 'https://example.com/docs' },
    { artifact_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' },
  ],
});
assert.deepEqual(restoredDraft.sourceArtifactIds, ['aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa']);
assert.deepEqual(restoredDraft.research, {
  connectorId: 'web-search',
  connectionId: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
});

const substituted = structuredClone(draft);
substituted.productName = 'Beacon';
assert.equal(buildSocialOperationsSettings(substituted).settings.product.name, 'Beacon');
assert.equal(buildSocialOperationsSettings(draft).settings.product.name, 'Launchpad');

const invalid = structuredClone(draft);
invalid.sourceUrls = ['http://example.com/private'];
assert.deepEqual(buildSocialOperationsSettings(invalid), { error: 'sources' });

const noResearch = structuredClone(draft);
noResearch.research = { connectorId: '', connectionId: '' };
assert.deepEqual(buildSocialOperationsSettings(noResearch), { error: 'research' });

const componentSource = await readFile(
  new URL('../src/components/digital-human/digital-human-social-operations.tsx', import.meta.url),
  'utf8'
);
const hookSource = await readFile(
  new URL('../src/hooks/digital-human/use-social-operations.ts', import.meta.url),
  'utf8'
);
const serviceSource = await readFile(
  new URL('../src/services/digital-human.service.ts', import.meta.url),
  'utf8'
);
assert.doesNotMatch(componentSource, /token|secret|JSON\.stringify\(draft/gi);
assert.match(componentSource, /linkedinCompanyInboxUnavailable/);
assert.match(componentSource, /researchConnectionChoices/);
assert.match(componentSource, /hasPermission\('digital_human\.update'\)/);
assert.equal((hookSource.match(/retry: false/g) ?? []).length, 6);
assert.ok((hookSource.match(/if \(!scopeIsCurrent\(\)\)/g) ?? []).length >= 3);
assert.match(hookSource, /socialOperationsQueryKey\(scope\)/);
assert.match(componentSource, /preflight\.published_employee_version_id/);
assert.match(componentSource, /preflight\.prerequisites/);
assert.match(serviceSource, /social-operations\/configuration/);
assert.match(serviceSource, /social-operations\/accounts/);
assert.match(serviceSource, /social-operations\/preflight/);
assert.match(serviceSource, /social-operations\/setup/);
assert.match(serviceSource, /social-operations\/assets\/images/);
assert.match(serviceSource, /const formData = new FormData\(\)/);
assert.doesNotMatch(
  serviceSource.match(/uploadSocialOperationsImage[\s\S]*?\n {2}}/)?.[0] ?? '',
  /multipart\/form-data/
);

console.log('Digital human social operations form and scope contracts passed');
