import assert from 'node:assert/strict';

import enUS from '../src/i18n/modules/dashboard/en-US.ts';
import zhHans from '../src/i18n/modules/dashboard/zh-Hans.ts';
import {
  MODEL_USAGE_APP_TYPES,
  normalizeModelUsageAppType,
} from '../src/utils/model-usage-app-type.ts';

const expectedAppTypes = [
  'workflow',
  'dataset',
  'agent',
  'aichat',
  'image-runtime',
  'data_library_file',
  'prompt_optimizer',
  'prompt_playground',
  'automation_task_draft',
  'unknown',
];

const expectedLabels = {
  en: {
    workflow: 'Workflow',
    dataset: 'Knowledge Base',
    agent: 'Agent',
    aichat: 'General Chat',
    'image-runtime': 'Image Generation',
    data_library_file: 'Knowledge Base File Processing',
    prompt_optimizer: 'Prompt Optimization',
    prompt_playground: 'Prompt Playground',
    automation_task_draft: 'Scheduled Task Draft Generation',
    unknown: 'Other',
  },
  zh: {
    workflow: '工作流',
    dataset: '知识库',
    agent: '智能体',
    aichat: '通用对话',
    'image-runtime': '绘图',
    data_library_file: '知识库文件处理',
    prompt_optimizer: '提示词优化',
    prompt_playground: '提示词调试台',
    automation_task_draft: '定时任务草稿生成',
    unknown: '其他',
  },
};

assert.deepEqual(
  MODEL_USAGE_APP_TYPES,
  expectedAppTypes,
  'the frontend app type contract must match the statistics API contract'
);

for (const appType of expectedAppTypes) {
  assert.equal(
    normalizeModelUsageAppType(appType),
    appType,
    `known app type ${appType} must remain distinct`
  );
  assert.equal(
    enUS.usage.appTypes[appType],
    expectedLabels.en[appType],
    `English label must match for ${appType}`
  );
  assert.equal(
    zhHans.usage.appTypes[appType],
    expectedLabels.zh[appType],
    `Chinese label must match for ${appType}`
  );
}

for (const appType of [undefined, null, '', 'web-app', 'future_app_type']) {
  assert.equal(
    normalizeModelUsageAppType(appType),
    'unknown',
    `unsupported app type ${String(appType)} must use the unknown label`
  );
}

console.log('Model usage app type checks passed.');
