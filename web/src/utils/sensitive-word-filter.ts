import { sensitiveWordAutomatonData } from '@/generated/sensitive-word-automaton';

export interface SensitiveWordAutomatonData {
  transitions: Array<Record<string, number>>;
  failures: number[];
  outputs: number[];
  wordCount: number;
  maxWordLength: number;
}

export interface SensitiveWordStreamResult {
  checked: boolean;
  matched: boolean;
}

export interface SensitiveWordStreamSession {
  append: (text: string) => SensitiveWordStreamResult;
  finish: () => SensitiveWordStreamResult;
}

export interface SensitiveWordStreamSessionOptions {
  chunkSize?: number;
  lookbehindSize?: number;
}

const DEFAULT_CHUNK_SIZE = 50;
const DEFAULT_LOOKBEHIND_SIZE = 50;
const STRIPPED_CHARACTERS_RE = /[\p{White_Space}\p{Punctuation}\p{Symbol}]+/gu;
const sensitiveWordAutomaton: SensitiveWordAutomatonData = sensitiveWordAutomatonData;

export function isSensitiveWordFilterEnabled(): boolean {
  return process.env.NEXT_PUBLIC_SENSITIVE_WORD_FILTER_ENABLED === 'true';
}

function isSensitiveWordFilterReady(): boolean {
  return isSensitiveWordFilterEnabled() && sensitiveWordAutomaton.wordCount > 0;
}

function normalizeSensitiveText(value: string): string {
  return value.normalize('NFKC').toLowerCase().replace(STRIPPED_CHARACTERS_RE, '');
}

function takeLastCharacters(value: string, count: number): string {
  const chars = Array.from(value);
  return chars.slice(Math.max(0, chars.length - count)).join('');
}

export const SensitiveWordMatcher = {
  contains(text: string): boolean {
    if (!isSensitiveWordFilterReady() || text.length === 0) {
      return false;
    }

    const normalized = normalizeSensitiveText(text);
    if (normalized.length === 0) {
      return false;
    }

    const { transitions, failures, outputs } = sensitiveWordAutomaton;
    let state = 0;

    for (const char of normalized) {
      let next = transitions[state]?.[char];
      while (typeof next !== 'number' && state !== 0) {
        state = failures[state] ?? 0;
        next = transitions[state]?.[char];
      }
      state = typeof next === 'number' ? next : 0;
      if ((outputs[state] ?? 0) > 0) {
        return true;
      }
    }

    return false;
  },
};

export function createSensitiveWordStreamSession(
  options: SensitiveWordStreamSessionOptions = {}
): SensitiveWordStreamSession {
  const chunkSize = Math.max(1, options.chunkSize ?? DEFAULT_CHUNK_SIZE);
  const lookbehindSize = Math.max(0, options.lookbehindSize ?? DEFAULT_LOOKBEHIND_SIZE);

  let fullText = '';
  let previousWindow = '';
  let currentWindow = '';
  let currentWindowLength = 0;
  let blocked = false;

  const result = (checked: boolean, matched = false): SensitiveWordStreamResult => ({
    checked,
    matched,
  });

  const markBlocked = (): SensitiveWordStreamResult => {
    blocked = true;
    return result(true, true);
  };

  return {
    append(text: string): SensitiveWordStreamResult {
      if (text.length === 0) {
        return result(false);
      }

      fullText += text;

      if (!isSensitiveWordFilterReady() || blocked) {
        return result(false, blocked);
      }

      let checked = false;
      for (const char of text) {
        currentWindow += char;
        currentWindowLength += 1;
        if (currentWindowLength < chunkSize) {
          continue;
        }

        checked = true;
        if (SensitiveWordMatcher.contains(`${previousWindow}${currentWindow}`)) {
          return markBlocked();
        }
        previousWindow = takeLastCharacters(currentWindow, lookbehindSize);
        currentWindow = '';
        currentWindowLength = 0;
      }

      return result(checked);
    },

    finish(): SensitiveWordStreamResult {
      if (!isSensitiveWordFilterReady() || blocked) {
        return result(false, blocked);
      }
      if (SensitiveWordMatcher.contains(fullText)) {
        return markBlocked();
      }
      return result(true);
    },

  };
}
