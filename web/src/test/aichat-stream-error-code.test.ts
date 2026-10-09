import assert from 'node:assert/strict';

// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { resolveAIChatErrorCode, isAIChatApplicationErrorCode } from '../components/chat/variants/aichat/stream-error-code.ts';

// The generic system code used to win, so an unknown model rendered as a server
// error even though the frame said llm.model.not_found.
assert.equal(resolveAIChatErrorCode(399001, 'llm.model.not_found'), 'llm.model.not_found');
assert.equal(resolveAIChatErrorCode('399001', 'llm.provider.none_available'), 'llm.provider.none_available');
assert.equal(resolveAIChatErrorCode(undefined, 'llm.model.not_found'), 'llm.model.not_found');

// Product codes keep winning: billing numbers and runtime words are what the
// existing billing and runtime copy is keyed on.
assert.equal(resolveAIChatErrorCode(310021, 'billing.organization.balance_insufficient'), 310021);
assert.equal(resolveAIChatErrorCode('model_service_timeout', 'llm.provider.timeout'), 'model_service_timeout');

// Without a catalog identity nothing changes.
assert.equal(resolveAIChatErrorCode(399001, undefined), 399001);
assert.equal(resolveAIChatErrorCode(399001, 'not-a-catalog-code'), 399001);
assert.equal(resolveAIChatErrorCode(undefined, undefined), undefined);

assert.equal(isAIChatApplicationErrorCode('llm.model.not_found'), true);
assert.equal(isAIChatApplicationErrorCode('399001'), false);
assert.equal(isAIChatApplicationErrorCode(399001), false);

console.log('aichat stream error code: ok');
