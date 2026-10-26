import assert from 'node:assert/strict';

// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { joinFacts, memoryTopicTitle, splitFacts } from '../components/memory/memory-topics.ts';

assert.deepEqual(
  splitFacts('- 姓名：林川\n\n* 常住地：杭州\n1. 职业：产品经理\n喜欢跑步'),
  ['姓名：林川', '常住地：杭州', '职业：产品经理', '喜欢跑步'],
  'facts are the non-empty lines with list markers removed'
);
assert.equal(joinFacts(['Prefers short answers.']), 'Prefers short answers.', 'one fact stays plain');
assert.equal(joinFacts(['A', ' ', 'B']), '- A\n- B', 'several facts become a list');
assert.deepEqual(splitFacts(joinFacts(['A', 'B'])), ['A', 'B'], 'joining and splitting round-trips');

const titles = { profile: '个人资料', preferences: '偏好' } as Record<string, string>;
const knownTitle = (topic: string) => titles[topic] ?? topic;
assert.equal(memoryTopicTitle('profile', 'profile', knownTitle), '个人资料', 'a known key gets its title');
assert.equal(memoryTopicTitle('preference', '', knownTitle), '偏好', 'aliases resolve to a known topic');
assert.equal(memoryTopicTitle('travel_plans', undefined, knownTitle), 'Travel plans', 'other keys are made readable');
assert.equal(memoryTopicTitle('note_1a2b', '旅行计划', knownTitle), '旅行计划', 'a display name wins');

console.log('memory topics: ok');
