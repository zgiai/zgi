import type { SseEventCallbacks } from '@/lib/http/types';
import {
  SensitiveWordMatcher,
  createSensitiveWordStreamSession,
  isSensitiveWordFilterEnabled,
} from '@/utils/sensitive-word-filter';

export const SENSITIVE_OUTPUT_BLOCKED_TOKEN = '__ZGI_SENSITIVE_OUTPUT_BLOCKED__';
export const SENSITIVE_OUTPUT_BLOCKED_FLAG = '__sensitiveOutputBlocked';

type RecordLike = Record<string, unknown>;

const TEXT_KEYS = ['answer', 'answer_delta', 'text', 'content', 'delta'] as const;

function isRecord(value: unknown): value is RecordLike {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function getPayloadData(payload: unknown): RecordLike | null {
  if (!isRecord(payload)) {
    return null;
  }
  const nested = payload.data;
  return isRecord(nested) ? nested : payload;
}

function getOutputText(payload: unknown): string {
  if (typeof payload === 'string') {
    return payload;
  }

  const data = getPayloadData(payload);
  if (!data) {
    return '';
  }

  for (const key of TEXT_KEYS) {
    const value = data[key];
    if (typeof value === 'string') {
      return value;
    }
  }

  const outputs = data.outputs;
  if (isRecord(outputs)) {
    for (const key of ['answer', 'text', 'content', 'output']) {
      const value = outputs[key];
      if (typeof value === 'string') {
        return value;
      }
    }
  }

  return '';
}

function getWorkflowSnapshotAnswer(payload: unknown): string {
  const data = getPayloadData(payload);
  if (!data) {
    return '';
  }
  const message = data.message;
  if (!isRecord(message)) {
    return '';
  }
  return typeof message.answer === 'string' ? message.answer : '';
}

function takeLastCharacters(value: string, count: number): string {
  const characters = Array.from(value);
  return characters.slice(Math.max(0, characters.length - count)).join('');
}

export function isSensitiveOutputBlockedValue(value: unknown): boolean {
  if (value === SENSITIVE_OUTPUT_BLOCKED_TOKEN) {
    return true;
  }
  return isRecord(value) && value[SENSITIVE_OUTPUT_BLOCKED_FLAG] === true;
}

export function createSensitiveOutputBlockedPayload(): RecordLike {
  return {
    event: 'text_replace',
    [SENSITIVE_OUTPUT_BLOCKED_FLAG]: true,
    answer: SENSITIVE_OUTPUT_BLOCKED_TOKEN,
    text: SENSITIVE_OUTPUT_BLOCKED_TOKEN,
    content: SENSITIVE_OUTPUT_BLOCKED_TOKEN,
  };
}

export function sanitizeModelOutputValue(value: unknown): unknown {
  if (!isSensitiveWordFilterEnabled()) {
    return value;
  }

  if (typeof value === 'string') {
    return SensitiveWordMatcher.contains(value) ? SENSITIVE_OUTPUT_BLOCKED_TOKEN : value;
  }

  if (Array.isArray(value)) {
    let changed = false;
    const next = value.map(item => {
      const sanitized = sanitizeModelOutputValue(item);
      changed = changed || sanitized !== item;
      return sanitized;
    });
    return changed ? next : value;
  }

  if (!isRecord(value)) {
    return value;
  }

  let changed = false;
  const next: RecordLike = {};
  for (const [key, item] of Object.entries(value)) {
    const sanitized = sanitizeModelOutputValue(item);
    changed = changed || sanitized !== item;
    next[key] = sanitized;
  }
  return changed ? next : value;
}

function withSanitizedOutputFields(payload: unknown): unknown {
  if (!isSensitiveWordFilterEnabled()) {
    return payload;
  }

  if (typeof payload === 'string') {
    return SensitiveWordMatcher.contains(payload) ? SENSITIVE_OUTPUT_BLOCKED_TOKEN : payload;
  }

  if (!isRecord(payload)) {
    return payload;
  }

  const data = getPayloadData(payload);
  if (!data) {
    return payload;
  }

  let changed = false;
  const nextData: RecordLike = { ...data };

  for (const key of TEXT_KEYS) {
    if (typeof data[key] === 'string') {
      const sanitized = sanitizeModelOutputValue(data[key]);
      if (sanitized !== data[key]) {
        nextData[key] = sanitized;
        changed = true;
      }
    }
  }

  if ('outputs' in data) {
    const sanitized = sanitizeModelOutputValue(data.outputs);
    if (sanitized !== data.outputs) {
      nextData.outputs = sanitized;
      changed = true;
    }
  }

  if (!changed) {
    return payload;
  }

  if (isRecord(payload.data) && payload.data === data) {
    return { ...payload, data: nextData };
  }

  return nextData;
}

export function wrapModelOutputSseCallbacks<T extends SseEventCallbacks>(callbacks: T): T {
  if (!isSensitiveWordFilterEnabled()) {
    return callbacks;
  }

  const session = createSensitiveWordStreamSession({ chunkSize: 50, lookbehindSize: 50 });
  let blocked = false;
  let streamTail = '';

  const block = (): void => {
    blocked = true;
    callbacks.onTextReplace?.(createSensitiveOutputBlockedPayload());
  };

  const appendStreamText = (text: string): boolean => {
    if (!text) {
      return false;
    }
    if (SensitiveWordMatcher.contains(`${streamTail}${text}`) || session.append(text).matched) {
      return true;
    }
    streamTail = takeLastCharacters(`${streamTail}${text}`, 50);
    return false;
  };

  const finish = (): void => {
    if (!blocked && session.finish().matched) {
      block();
    }
  };

  return {
    ...callbacks,
    onWorkflowSnapshot: payload => {
      const answer = getWorkflowSnapshotAnswer(payload);
      const sanitizedPayload = sanitizeModelOutputValue(payload);
      const sanitizedAnswer = getWorkflowSnapshotAnswer(sanitizedPayload);
      const snapshotBlocked =
        isSensitiveOutputBlockedValue(answer) ||
        isSensitiveOutputBlockedValue(sanitizedAnswer) ||
        (answer.length > 0 && SensitiveWordMatcher.contains(answer));

      if (snapshotBlocked) {
        blocked = true;
        streamTail = '';
      } else if (!blocked && answer.length > 0 && appendStreamText(answer)) {
        blocked = true;
      }
      callbacks.onWorkflowSnapshot?.(sanitizedPayload);
    },
    onTextChunk: payload => {
      if (blocked) return;
      const text = getOutputText(payload);
      if (appendStreamText(text)) {
        block();
        return;
      }
      callbacks.onTextChunk?.(payload);
    },
    onTextReplace: payload => {
      if (blocked) return;
      const text = getOutputText(payload);
      if (text && SensitiveWordMatcher.contains(text)) {
        block();
        return;
      }
      callbacks.onTextReplace?.(withSanitizedOutputFields(payload));
    },
    onMessage: payload => {
      if (blocked) return;
      const text = getOutputText(payload);
      if (appendStreamText(text)) {
        block();
        return;
      }
      callbacks.onMessage?.(withSanitizedOutputFields(payload));
    },
    onMessageEnd: payload => {
      finish();
      callbacks.onMessageEnd?.(withSanitizedOutputFields(payload));
    },
    onNodeFinished: payload => {
      callbacks.onNodeFinished?.(withSanitizedOutputFields(payload));
    },
    onWorkflowFinished: payload => {
      finish();
      callbacks.onWorkflowFinished?.(withSanitizedOutputFields(payload));
    },
    onError: payload => {
      callbacks.onError?.(payload);
    },
  };
}

export function isSensitiveOutputBlockedPayload(payload: unknown): boolean {
  if (isSensitiveOutputBlockedValue(payload)) {
    return true;
  }
  const data = getPayloadData(payload);
  if (!data) {
    return false;
  }
  if (data[SENSITIVE_OUTPUT_BLOCKED_FLAG] === true) {
    return true;
  }
  return TEXT_KEYS.some(key => isSensitiveOutputBlockedValue(data[key]));
}

export function getSensitiveOutputTextFromPayload(payload: unknown): string | null {
  if (isSensitiveOutputBlockedPayload(payload)) {
    return SENSITIVE_OUTPUT_BLOCKED_TOKEN;
  }
  const text = getOutputText(payload);
  return text.length > 0 ? text : null;
}
