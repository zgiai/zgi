import assert from 'node:assert/strict';
import { getDefaultVoiceOptions } from '../src/app/dashboard/settings/model/default-voice.ts';

const rules = [
  {
    name: 'default_voice',
    options: [' voice-1 ', 'voice-2', 'voice-1', ''],
    option_labels: {
      'voice-1': { zh_Hans: '音色一', en_US: 'Voice One' },
      'voice-2': { zh_Hans: '音色二' },
    },
  },
];

assert.deepEqual(getDefaultVoiceOptions(rules, 'zh-Hans'), [
  { value: 'voice-1', label: '音色一' },
  { value: 'voice-2', label: '音色二' },
]);

assert.deepEqual(getDefaultVoiceOptions(rules, 'en-US'), [
  { value: 'voice-1', label: 'Voice One' },
  { value: 'voice-2', label: '音色二' },
]);

assert.deepEqual(
  getDefaultVoiceOptions([{ name: 'default_voice', options: ['voice-3'] }], 'zh-Hans'),
  [{ value: 'voice-3', label: 'voice-3' }]
);

console.log('default voice option tests passed');
