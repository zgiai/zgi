import assert from 'node:assert/strict';

import { modelProcessingStateFromEvent } from '../src/components/chat/controllers/aichat/model-processing.ts';

const base = {
  conversation_id: 'conversation-1',
  message_id: 'message-1',
  phase: 'model_processing',
  progress_id: 'message-1:1',
  status: 'running',
};

const initial = modelProcessingStateFromEvent(
  undefined,
  { ...base, stage: 'initial', elapsed_ms: 800 },
  '1-0'
);
assert.equal(initial?.stage, 'initial');
assert.equal(initial?.activity, 'awaiting_response');
assert.equal(initial?.event_id, '1-0');

const reasoning = modelProcessingStateFromEvent(
  initial,
  {
    ...base,
    stage: 'initial',
    activity: 'reasoning',
    source: 'provider_signal',
    elapsed_ms: 1800,
  },
  '1-1'
);
assert.equal(reasoning?.activity, 'reasoning');
assert.equal(reasoning?.source, 'provider_signal');

const extended = modelProcessingStateFromEvent(
  reasoning,
  {
    ...base,
    stage: 'extended',
    activity: 'reasoning',
    source: 'provider_signal',
    elapsed_ms: 15000,
  },
  '2-0'
);
assert.equal(extended?.stage, 'extended');
assert.equal(extended?.activity, 'reasoning');
assert.equal(extended?.elapsed_ms, 15000);

const replay = modelProcessingStateFromEvent(
  extended,
  { ...base, stage: 'extended', elapsed_ms: 15000 },
  '2-0'
);
assert.strictEqual(replay, extended);

const olderStage = modelProcessingStateFromEvent(
  extended,
  { ...base, stage: 'initial', elapsed_ms: 800 },
  '3-0'
);
assert.strictEqual(olderStage, extended);

const preparingAction = modelProcessingStateFromEvent(
  extended,
  {
    ...base,
    stage: 'extended',
    activity: 'preparing_action',
    source: 'provider_signal',
  },
  '3-1'
);
assert.equal(preparingAction?.activity, 'preparing_action');

const activityRegression = modelProcessingStateFromEvent(
  preparingAction,
  {
    ...base,
    stage: 'extended',
    activity: 'reasoning',
    source: 'provider_signal',
  },
  '3-2'
);
assert.strictEqual(activityRegression, preparingAction);

const nextRound = modelProcessingStateFromEvent(
  preparingAction,
  {
    ...base,
    progress_id: 'message-1:2',
    stage: 'initial',
    activity: 'reviewing_tool_result',
    source: 'runtime',
    round: 2,
  },
  '4-0'
);
assert.equal(nextRound?.progress_id, 'message-1:2');
assert.equal(nextRound?.stage, 'initial');
assert.equal(nextRound?.activity, 'reviewing_tool_result');

const reconnect = modelProcessingStateFromEvent(
  undefined,
  {
    ...base,
    stage: 'long_running',
    activity: 'reasoning',
    source: 'provider_signal',
    elapsed_ms: 45000,
  },
  '5-0'
);
assert.equal(reconnect?.stage, 'long_running');
assert.equal(reconnect?.activity, 'reasoning');

const invalid = modelProcessingStateFromEvent(
  reconnect,
  {
    ...base,
    stage: 'unknown',
  },
  '6-0'
);
assert.strictEqual(invalid, reconnect);

console.log('native model processing tests passed');
