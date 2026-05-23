export const DEFAULT_CONVERSATIONAL_PROMPTS = {
  zh: {
    promptId: '9fa2ec04-0672-4e4f-9af6-ac4630f542ff',
    promptName: '企业助手回复模板',
    locale: 'zh-Hans',
    source: 'official' as const,
    text: '你是一名企业助手。请清晰回答问题，在必要时追问缺失上下文，并保持回复可执行、简洁、符合业务表达习惯。严格遵守政策边界。',
  },
  en: {
    promptId: '3db60d44-f6a7-4892-8e65-9b7f95f69ab1',
    promptName: 'Enterprise Assistant Answer',
    locale: 'en-US',
    source: 'official' as const,
    text: 'You are an enterprise assistant. Answer clearly, ask for missing context when needed, and keep the response actionable. Prefer concise business language and preserve policy boundaries.',
  },
};

export const DEFAULT_STANDARD_PROMPTS = {
  zh: {
    promptId: '9c6ff0a8-c53f-42b7-87c2-1d2f9f7f1d08',
    promptName: '通用工作流助手模板',
    locale: 'zh-Hans',
    source: 'official' as const,
    text: '你是一名工作流任务助手。请严格根据提供的输入完成任务，输出清晰、可执行、可复用的结果；只有在确实缺少必要上下文时才提出澄清问题，不要虚构事实、结论或承诺。',
  },
  en: {
    promptId: '2d35f08e-5c52-43ef-bf63-bca3d5ae86ab',
    promptName: 'Workflow Task Assistant',
    locale: 'en-US',
    source: 'official' as const,
    text: 'You are a workflow task assistant. Follow the provided input carefully, produce clear and actionable output, ask for missing context only when absolutely necessary, and avoid fabricating facts or decisions.',
  },
};

export const getDefaultWorkflowPrompts = (locale: string) => {
  const isZh = locale.toLowerCase().startsWith('zh');
  return {
    conversational: isZh ? DEFAULT_CONVERSATIONAL_PROMPTS.zh : DEFAULT_CONVERSATIONAL_PROMPTS.en,
    standard: isZh ? DEFAULT_STANDARD_PROMPTS.zh : DEFAULT_STANDARD_PROMPTS.en,
  };
};
