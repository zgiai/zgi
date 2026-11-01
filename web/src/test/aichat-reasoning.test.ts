import assert from 'node:assert/strict';

import {
  carryReasoningPreference,
  modelSupportsReasoningToggle,
  reasoningDurationSeconds,
  reasoningEffortForPreference,
  reasoningModelKey,
  reasoningPreferenceForModel,
  withReasoningEffortParam,
  // @ts-ignore Node's direct TypeScript runner requires an explicit extension.
} from '../components/chat/variants/aichat/reasoning.ts';

// The switch is offered only for platform models that can reason.
assert.equal(modelSupportsReasoningToggle(null), false);
assert.equal(modelSupportsReasoningToggle({ features: { reasoning: true } }), true);
assert.equal(modelSupportsReasoningToggle({ features: { reasoning_effort: true } }), true);
assert.equal(modelSupportsReasoningToggle({ features: { vision: true } }), false);
assert.equal(
  modelSupportsReasoningToggle({ is_custom: true, features: { reasoning: true } }),
  false
);

// Untouched = follow the model default; on/off map to the unified levels.
assert.equal(reasoningEffortForPreference(undefined), undefined);
assert.equal(reasoningEffortForPreference(true), 'medium');
assert.equal(reasoningEffortForPreference(false), 'none');

assert.deepEqual(withReasoningEffortParam({ temperature: 0.2 }, true), {
  temperature: 0.2,
  reasoning_effort: 'medium',
});
assert.deepEqual(withReasoningEffortParam(undefined, false), { reasoning_effort: 'none' });
assert.equal(withReasoningEffortParam(undefined, undefined), undefined);
const untouched = { temperature: 0.2 };
assert.equal(withReasoningEffortParam(untouched, undefined), untouched);
assert.deepEqual(
  withReasoningEffortParam({ temperature: 0.2, reasoning_effort: 'none' }, undefined),
  {
    temperature: 0.2,
  }
);
assert.equal(withReasoningEffortParam({ reasoning_effort: 'none' }, undefined), undefined);

assert.equal(reasoningDurationSeconds(undefined), undefined);
assert.equal(reasoningDurationSeconds(-1), undefined);
assert.equal(reasoningDurationSeconds(0), 1);
assert.equal(reasoningDurationSeconds(12_400), 12);

// A draft's choice moves to the conversation it becomes, once.
assert.deepEqual(carryReasoningPreference({ draft: true }, 'draft', 'c1'), {
  draft: true,
  c1: true,
});
const kept = { draft: true, c1: false };
assert.equal(carryReasoningPreference(kept, 'draft', 'c1'), kept);
const none = {};
assert.equal(carryReasoningPreference(none, 'draft', 'c1'), none);
assert.equal(carryReasoningPreference(kept, 'c1', 'c1'), kept);

// Switching the model resets the conversation to the model default.
const qwen = reasoningModelKey('zgi_cloud', 'qwen3.8-27b');
assert.equal(reasoningPreferenceForModel(undefined, qwen), undefined);
assert.equal(reasoningPreferenceForModel({ enabled: false, modelKey: qwen }, qwen), false);
assert.equal(
  reasoningPreferenceForModel(
    { enabled: true, modelKey: qwen },
    reasoningModelKey('zgi_cloud', 'deepseek-v4-flash')
  ),
  undefined
);

console.log('aichat reasoning helpers: ok');
