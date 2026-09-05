import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';
import {
  AI_CREDITS_SCALE,
  DEVELOPER_ACCESS_QUOTA_STEP,
  denormalizeDeveloperAccessPolicy,
  denormalizeDeveloperAccessRequest,
  denormalizeDeveloperAccessReview,
  isValidDeveloperAccessQuota,
  normalizeDeveloperAccessAudit,
  normalizeDeveloperAccessMe,
  normalizeDeveloperAccessPolicy,
  normalizeDeveloperAccessRequest,
  normalizeDeveloperAccessRequests,
} from '../src/utils/ai-credits.ts';

function freeze(value) {
  if (value && typeof value === 'object') {
    Object.values(value).forEach(freeze);
    Object.freeze(value);
  }
  return value;
}

const policy = freeze({
  workspace_id: 'workspace-a',
  organization_id: 'org-a',
  mode: 'approval_required',
  default_quota: 100000,
  max_quota: 200001,
  max_keys: 2,
  default_ttl_seconds: 3600,
  max_ttl_seconds: 7200,
  allowed_models: ['model-a'],
  version: 3,
});
const request = freeze({
  id: 'request-a',
  requester_account_id: 'member-a',
  purpose: 'experiment',
  environment: 'development',
  requested_quota: 100000,
  requested_models: ['model-a'],
  requested_ttl_seconds: 3600,
  status: 'pending',
  created_at: '2026-09-06T00:00:00Z',
});
const raw = freeze({
  workspace_id: 'workspace-a',
  organization_id: 'org-a',
  principal_id: 'member-a',
  principal_type: 'user',
  role: 'member',
  mode: 'approval_required',
  can_manage: false,
  can_create_key: true,
  can_request_access: false,
  active_key_count: 1,
  policy,
  pending_request: request,
  grant: {
    id: 'grant-a',
    status: 'active',
    quota_limit: 100000,
    used_quota: 1234,
    remain_quota: 98766,
    max_keys: 2,
    allowed_models: ['model-a'],
  },
});
const view = normalizeDeveloperAccessMe(raw);
assert.equal(AI_CREDITS_SCALE, 1000);
assert.equal(DEVELOPER_ACCESS_QUOTA_STEP, 0.001);
assert.equal(view.policy.default_quota, 100);
assert.equal(view.policy.max_quota, 200.001);
assert.equal(view.pending_request.requested_quota, 100);
assert.equal(view.grant.quota_limit, 100);
assert.equal(view.grant.used_quota, 1.234);
assert.equal(view.grant.remain_quota, 98.766);
assert.equal(view.active_key_count, 1);
assert.equal(view.policy.default_ttl_seconds, 3600);
assert.equal(view.grant.max_keys, 2);
assert.equal(raw.grant.used_quota, 1234, 'must not normalize the cached API object in place');
assert.deepEqual(normalizeDeveloperAccessMe(raw), view, 'repeated selection must not scale twice');
assert.deepEqual(denormalizeDeveloperAccessPolicy(view.policy), policy);

const page = freeze({ items: [request], total: 61, pending_total: 21, page: 2, page_size: 20 });
assert.deepEqual(normalizeDeveloperAccessRequests(page), {
  ...page,
  items: [{ ...request, requested_quota: 100 }],
});
const audit = freeze({
  items: [
    {
      request_id: 'req-a',
      attempt_id: 'attempt-a',
      total_tokens: 1001,
      prompt_tokens: 1000,
      completion_tokens: 1,
      total_points: 1234,
      quota_charged_points: 1233,
      quota_overage_points: 1,
    },
  ],
  total: 1,
  page: 1,
  page_size: 50,
});
assert.deepEqual(normalizeDeveloperAccessAudit(audit), {
  ...audit,
  items: [
    {
      ...audit.items[0],
      total_points: 1.234,
      quota_charged_points: 1.233,
      quota_overage_points: 0.001,
    },
  ],
});

for (const internal of [0, 1, 1001, 100000, 99999999000]) {
  const points = internal / AI_CREDITS_SCALE;
  assert.equal(isValidDeveloperAccessQuota(points), true);
  assert.equal(
    denormalizeDeveloperAccessRequest({
      purpose: 'test',
      environment: 'development',
      requested_quota: points,
    }).requested_quota,
    internal
  );
  assert.equal(denormalizeDeveloperAccessReview({ quota_limit: points }).quota_limit, internal);
  assert.equal(
    normalizeDeveloperAccessRequest({ ...request, requested_quota: internal }).requested_quota,
    points
  );
}
for (const absent of [undefined, null]) {
  const unlimited = normalizeDeveloperAccessMe({
    ...raw,
    grant: absent,
    pending_request: absent,
    policy: { ...policy, default_quota: absent, max_quota: absent },
  });
  assert.equal(unlimited.grant, absent);
  assert.equal(unlimited.pending_request, absent);
  assert.equal(unlimited.policy.default_quota, absent);
  assert.equal(denormalizeDeveloperAccessPolicy(unlimited.policy).default_quota, absent);
}
assert.equal(
  normalizeDeveloperAccessMe({ ...raw, grant: { ...raw.grant, quota_limit: null } }).grant
    .quota_limit,
  null
);
assert.equal(normalizeDeveloperAccessPolicy({ ...policy, default_quota: 0 }).default_quota, 0);
assert.equal(denormalizeDeveloperAccessReview({}).quota_limit, undefined);
for (const unsafe of [Number.MAX_SAFE_INTEGER, Number.MAX_SAFE_INTEGER + 1, Infinity, 0.1, -1]) {
  assert.throws(
    () => normalizeDeveloperAccessPolicy({ ...policy, default_quota: unsafe }),
    RangeError
  );
}
for (const invalid of [-1, NaN, Infinity, 0.0001, 1.0001, Number.MAX_SAFE_INTEGER]) {
  assert.equal(isValidDeveloperAccessQuota(invalid), false);
  assert.throws(() => denormalizeDeveloperAccessReview({ quota_limit: invalid }), RangeError);
  assert.throws(() => denormalizeDeveloperAccessRequest({ requested_quota: invalid }), RangeError);
  assert.throws(
    () => denormalizeDeveloperAccessPolicy({ ...policy, default_quota: invalid }),
    RangeError
  );
}

// Lock integration at the API/view boundary; pure arithmetic alone cannot catch
// a new query or form accidentally bypassing the conversion.
const hook = await readFile(
  new URL('../src/hooks/developer-access/use-developer-access.ts', import.meta.url),
  'utf8'
);
for (const selector of ['normalizeDeveloperAccessMe', 'normalizeDeveloperAccessAudit']) {
  assert.match(hook, new RegExp(`select: ${selector}`));
}
assert.equal((hook.match(/select: normalizeDeveloperAccessRequests/g) || []).length, 2);
for (const input of ['Policy(input)', 'Request(input)', 'Review(input.review)']) {
  assert.ok(hook.includes(`denormalizeDeveloperAccess${input}`));
}
const form = await readFile(
  new URL('../src/features/developer-access/developer-access-page.tsx', import.meta.url),
  'utf8'
);
assert.equal((form.match(/step=\{DEVELOPER_ACCESS_QUOTA_STEP\}/g) || []).length, 4);
assert.equal((form.match(/!isValidDeveloperAccessQuota\(/g) || []).length, 4);
console.log('Developer access quota conversion, precision, isolation, and wiring checks passed.');
