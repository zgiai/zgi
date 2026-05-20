import type {
  QuestionAnswerChoice,
  QuestionAnswerRequestedSseData,
  QuestionAnswerSubmittedSseData,
} from '@/services/types/workflow';

export interface ParsedQuestionAnswerRequested {
  workflowRunId?: string;
  nodeId?: string;
  nodeTitle?: string;
  question: string;
  choices: QuestionAnswerChoice[];
  round?: number;
}

export interface ParsedQuestionAnswerSubmitted {
  workflowRunId?: string;
  nodeId?: string;
  answer: string;
  round?: number;
  choiceId?: string;
  choiceLabel?: string;
  choiceValue?: string;
}

export interface QuestionAnswerTranscriptItem {
  key: string;
  nodeId?: string;
  round?: number;
  question: string;
  answer?: string;
}

export interface ParsedQuestionAnswerPaused {
  isQuestionAnswer: boolean;
  workflowRunId?: string;
  elapsedTime?: number;
  nodeIds: string[];
  prompt?: ParsedQuestionAnswerRequested;
}

function unwrap(payload: unknown): Record<string, unknown> {
  const record = payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
  const data = record.data;
  return data && typeof data === 'object' ? (data as Record<string, unknown>) : record;
}

function normalizeChoices(value: unknown): QuestionAnswerChoice[] {
  if (!Array.isArray(value)) return [];
  const choices: QuestionAnswerChoice[] = [];
  value.forEach(item => {
    if (!item || typeof item !== 'object') return null;
    const record = item as Record<string, unknown>;
    const id = typeof record.id === 'string' ? record.id.trim() : '';
    if (!id) return null;
    choices.push({
      ...record,
      id,
      label: typeof record.label === 'string' ? record.label : undefined,
      value: typeof record.value === 'string' ? record.value : undefined,
    });
  });
  return choices;
}

function stringField(data: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    const value = data[key];
    if (typeof value !== 'string') continue;
    const text = value.trim();
    if (text) return text;
  }
  return '';
}

function numberField(data: Record<string, unknown>, key: string): number | undefined {
  const value = data[key];
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value !== 'string') return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

export function parseQuestionAnswerRequestedEvent(
  payload: unknown
): ParsedQuestionAnswerRequested | null {
  const data = unwrap(payload) as QuestionAnswerRequestedSseData & Record<string, unknown>;
  const question = stringField(data, 'question');
  if (!question) return null;

  return {
    workflowRunId: stringField(data, 'workflow_run_id', 'workflowRunId', 'id') || undefined,
    nodeId: stringField(data, 'node_id', 'nodeId') || undefined,
    nodeTitle: stringField(data, 'node_title', 'nodeTitle') || undefined,
    question,
    choices: normalizeChoices(data.choices),
    round: numberField(data, 'round'),
  };
}

export function parseQuestionAnswerSubmittedEvent(
  payload: unknown
): ParsedQuestionAnswerSubmitted | null {
  const data = unwrap(payload) as QuestionAnswerSubmittedSseData & Record<string, unknown>;
  const answer = stringField(data, 'answer');
  if (!answer) return null;

  return {
    workflowRunId: stringField(data, 'workflow_run_id', 'workflowRunId', 'id') || undefined,
    nodeId: stringField(data, 'node_id', 'nodeId') || undefined,
    answer,
    round: numberField(data, 'round'),
    choiceId: stringField(data, 'choice_id', 'choiceId') || undefined,
    choiceLabel: stringField(data, 'choice_label', 'choiceLabel') || undefined,
    choiceValue: stringField(data, 'choice_value', 'choiceValue') || undefined,
  };
}

export function isQuestionAnswerPromptMessage(payload: unknown): boolean {
  const data = unwrap(payload);
  return data.message_kind === 'question_answer_prompt';
}

export function appendQuestionAnswerTranscriptQuestion(
  items: QuestionAnswerTranscriptItem[],
  event: ParsedQuestionAnswerRequested
): QuestionAnswerTranscriptItem[] {
  const key = questionAnswerTranscriptKey(event.nodeId, event.round, event.question);
  const nextItem: QuestionAnswerTranscriptItem = {
    key,
    nodeId: event.nodeId,
    round: event.round,
    question: event.question,
  };
  const index = items.findIndex(item => item.key === key);
  if (index < 0) {
    const duplicateIndex = items.findIndex(item => isSamePendingQuestion(item, event));
    if (duplicateIndex < 0) return [...items, nextItem];
    const next = items.slice();
    next[duplicateIndex] = { ...next[duplicateIndex], ...nextItem };
    return next;
  }
  const next = items.slice();
  next[index] = { ...next[index], ...nextItem };
  return next;
}

export function applyQuestionAnswerTranscriptSubmission(
  items: QuestionAnswerTranscriptItem[],
  event: ParsedQuestionAnswerSubmitted
): QuestionAnswerTranscriptItem[] {
  const answer = event.choiceLabel || event.choiceValue || event.answer;
  const index = findQuestionAnswerTranscriptItem(items, event);
  if (index < 0) {
    if (hasRecentQuestionAnswerTranscriptAnswer(items, event, answer)) return items;
    return [
      ...items,
      {
        key: questionAnswerTranscriptKey(event.nodeId, event.round, answer),
        nodeId: event.nodeId,
        round: event.round,
        question: '',
        answer,
      },
    ];
  }
  const next = items.slice();
  next[index] = { ...next[index], answer };
  return next;
}

export function applyQuestionAnswerTranscriptLocalAnswer(
  items: QuestionAnswerTranscriptItem[],
  rawAnswer: string
): QuestionAnswerTranscriptItem[] {
  const answer = rawAnswer.trim();
  if (!answer) return items;

  for (let i = items.length - 1; i >= 0; i -= 1) {
    if (!items[i].answer) {
      const next = items.slice();
      next[i] = { ...next[i], answer };
      return next;
    }
  }

  return [
    ...items,
    {
      key: `question-answer-answer:${items.length}:${answer}`,
      question: '',
      answer,
    },
  ];
}

function questionAnswerTranscriptKey(
  nodeId: string | undefined,
  round: number | undefined,
  fallback: string
): string {
  if (nodeId && typeof round === 'number') return `${nodeId}:${round}`;
  if (nodeId) return `${nodeId}:${fallback}`;
  return fallback;
}

function isSamePendingQuestion(
  item: QuestionAnswerTranscriptItem,
  event: ParsedQuestionAnswerRequested
): boolean {
  if (item.answer || item.question !== event.question) return false;
  if (item.nodeId && event.nodeId && item.nodeId !== event.nodeId) return false;
  return true;
}

function findQuestionAnswerTranscriptItem(
  items: QuestionAnswerTranscriptItem[],
  event: ParsedQuestionAnswerSubmitted
): number {
  if (event.nodeId && typeof event.round === 'number') {
    const index = items.findIndex(
      item => item.nodeId === event.nodeId && item.round === event.round
    );
    if (index >= 0) return index;
  }
  if (event.nodeId) {
    for (let i = items.length - 1; i >= 0; i -= 1) {
      if (items[i].nodeId === event.nodeId && !items[i].answer) return i;
    }
  }
  for (let i = items.length - 1; i >= 0; i -= 1) {
    if (!items[i].answer) return i;
  }
  return -1;
}

function hasRecentQuestionAnswerTranscriptAnswer(
  items: QuestionAnswerTranscriptItem[],
  event: ParsedQuestionAnswerSubmitted,
  answer: string
): boolean {
  for (let i = items.length - 1; i >= 0; i -= 1) {
    const item = items[i];
    if (item.answer !== answer) continue;
    if (event.nodeId && item.nodeId && item.nodeId !== event.nodeId) continue;
    if (typeof event.round === 'number' && typeof item.round === 'number' && item.round !== event.round) {
      continue;
    }
    return true;
  }
  return false;
}

export function parseQuestionAnswerPausedEvent(payload: unknown): ParsedQuestionAnswerPaused {
  const data = unwrap(payload);
  const reasons = Array.isArray(data.reasons) ? data.reasons : [];
  const nodeIds: string[] = [];
  let prompt: ParsedQuestionAnswerRequested | undefined;
  const isQuestionAnswer = reasons.some(reason => {
    if (!reason || typeof reason !== 'object') return false;
    const record = reason as Record<string, unknown>;
    const matched = record.type === 'question_answer_required';
    const nodeId = typeof record.node_id === 'string' ? record.node_id : '';
    if (matched && nodeId) nodeIds.push(nodeId);
    if (matched && !prompt) {
      const parsedPrompt = parseQuestionAnswerRequestedEvent(record);
      if (parsedPrompt) prompt = parsedPrompt;
    }
    return matched;
  });

  if (!isQuestionAnswer && data.reason === 'question_answer_required') {
    const nodeId = typeof data.node_id === 'string' ? data.node_id : '';
    if (nodeId) nodeIds.push(nodeId);
    const parsedPrompt = parseQuestionAnswerRequestedEvent(data);
    if (parsedPrompt) prompt = parsedPrompt;
  }

  return {
    isQuestionAnswer: isQuestionAnswer || data.reason === 'question_answer_required',
    workflowRunId:
      (typeof data.id === 'string' ? data.id : '') ||
      (typeof data.workflow_run_id === 'string' ? data.workflow_run_id : '') ||
      undefined,
    elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
    nodeIds: Array.from(new Set(nodeIds)),
    prompt,
  };
}
