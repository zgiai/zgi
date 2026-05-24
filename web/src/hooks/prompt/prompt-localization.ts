import type { PromptDetail, PromptSummary } from '@/services/types/prompt';

type PromptCopy = {
  name: string;
  description: string;
};

const ZH_HANS_OFFICIAL_PROMPT_COPY_BY_ID: Record<string, PromptCopy> = {
  '9c6ff0a8-c53f-42b7-87c2-1d2f9f7f1d08': {
    name: '通用工作流助手模板',
    description: '适用于任务型工作流执行节点的官方默认提示词。',
  },
  '2d35f08e-5c52-43ef-bf63-bca3d5ae86ab': {
    name: '通用工作流助手模板',
    description: '适用于任务型工作流执行节点的官方默认提示词。',
  },
  '9fa2ec04-0672-4e4f-9af6-ac4630f542ff': {
    name: '企业助手回复模板',
    description: '适用于内部制度问答、服务指引和企业问答的官方助手提示词。',
  },
  '3db60d44-f6a7-4892-8e65-9b7f95f69ab1': {
    name: '企业助手回复模板',
    description: '适用于内部制度问答、服务指引和企业问答的官方助手提示词。',
  },
  'ca975ea8-8105-40d0-a9fd-b5865280f906': {
    name: '服务请求分诊分类器',
    description: '按紧急程度和处理路径路由服务请求的官方分类提示词。',
  },
  'c2b3cc59-36f7-4401-9e0f-c775c478a0f3': {
    name: '服务请求分诊分类器',
    description: '按紧急程度和处理路径路由服务请求的官方分类提示词。',
  },
  '09f473c0-39a0-49bb-bf77-5d7b2c0baee2': {
    name: '客服回复模板',
    description: '适用于常见服务对话的友好、合规客服回复模板。',
  },
  '8e0ca70d-42b9-4727-8ab3-985886fc2a31': {
    name: '客服回复模板',
    description: '适用于常见服务对话的友好、合规客服回复模板。',
  },
  '3756bb26-239b-4ad0-a056-bd4147cb6187': {
    name: '会议纪要行动项模板',
    description: '把长篇会议记录整理成简明摘要、决策、风险和下一步行动。',
  },
  '14f4b8e4-92d3-494f-b61c-36419fc51f8a': {
    name: '会议纪要行动项模板',
    description: '把长篇会议记录整理成简明摘要、决策、风险和下一步行动。',
  },
};

const ZH_HANS_OFFICIAL_PROMPT_COPY_BY_NAME: Record<string, PromptCopy> = {
  'Workflow Task Assistant': ZH_HANS_OFFICIAL_PROMPT_COPY_BY_ID['9c6ff0a8-c53f-42b7-87c2-1d2f9f7f1d08'],
  'Enterprise Assistant Answer': ZH_HANS_OFFICIAL_PROMPT_COPY_BY_ID['9fa2ec04-0672-4e4f-9af6-ac4630f542ff'],
  'Service Request Triage Classifier': ZH_HANS_OFFICIAL_PROMPT_COPY_BY_ID['ca975ea8-8105-40d0-a9fd-b5865280f906'],
  'Customer Support Reply': ZH_HANS_OFFICIAL_PROMPT_COPY_BY_ID['09f473c0-39a0-49bb-bf77-5d7b2c0baee2'],
  'Meeting Summary Action Items': ZH_HANS_OFFICIAL_PROMPT_COPY_BY_ID['3756bb26-239b-4ad0-a056-bd4147cb6187'],
};

function shouldLocalizeOfficialPrompt(locale: string): boolean {
  return locale.trim().toLowerCase().startsWith('zh');
}

export function localizePromptSummary<T extends PromptSummary>(prompt: T, locale: string): T {
  if (prompt.source !== 'official' || !shouldLocalizeOfficialPrompt(locale)) {
    return prompt;
  }

  const copy = ZH_HANS_OFFICIAL_PROMPT_COPY_BY_ID[prompt.id] ?? ZH_HANS_OFFICIAL_PROMPT_COPY_BY_NAME[prompt.name];
  if (!copy) {
    return prompt;
  }

  return {
    ...prompt,
    name: copy.name,
    description: copy.description,
  };
}

export function localizePromptDetail(prompt: PromptDetail, locale: string): PromptDetail {
  return localizePromptSummary(prompt, locale);
}
