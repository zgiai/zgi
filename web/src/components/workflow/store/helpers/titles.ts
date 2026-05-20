// Utility: Title helpers for workflow nodes
import { isContainerStartNode } from '../type';
// Keep UI-facing default titles here to avoid coupling inside the store

export const getDefaultTitleByType = (nodeType?: string): string => {
  // Server-side safe best-effort i18n; falls back to zh label when i18n not available
  const fallback = (t: string | undefined) => t || '节点';
  switch (nodeType) {
    case 'start':
      return fallback('开始');
    case 'knowledge-retrieval':
      return fallback('知识检索');
    case 'llm':
      return fallback('LLM');
    case 'http-request':
      return fallback('HTTP 请求');
    case 'code':
      return fallback('代码执行');
    case 'iteration':
      return fallback('迭代');
    case 'assigner':
      return fallback('变量赋值');
    case 'answer':
      return fallback('直接回复');
    case 'if-else':
      return fallback('条件分支');
    case 'iteration-start':
      return fallback('迭代');
    case 'end':
      return fallback('结束');
    default:
      if (isContainerStartNode(nodeType)) {
        return fallback('迭代');
      }
      return fallback(nodeType);
  }
};

// Find next available title by appending -N (N starts from 2)
// Example: base "LLM" -> "LLM-2", "LLM-3", ...
export const getNextNodeTitle = (baseTitle: string, existingTitles: Set<string>): string => {
  if (!existingTitles.has(baseTitle)) return baseTitle;
  let n = 2;
  while (existingTitles.has(`${baseTitle}-${n}`)) n += 1;
  return `${baseTitle}-${n}`;
};
