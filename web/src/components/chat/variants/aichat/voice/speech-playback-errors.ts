export type SpeechPlaybackErrorKey = 'timeout' | 'balance' | 'quota' | 'unavailable' | 'failed';

function readString(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

export function getSpeechPlaybackErrorKey(error: unknown): SpeechPlaybackErrorKey {
  if (!error || typeof error !== 'object') return 'failed';

  const candidate = error as {
    code?: unknown;
    businessError?: { code?: unknown };
    response?: { status?: unknown; data?: { code?: unknown; errorCode?: unknown } };
  };
  const code =
    readString(candidate.response?.data?.code) ||
    readString(candidate.response?.data?.errorCode) ||
    readString(candidate.businessError?.code) ||
    readString(candidate.code);
  const status = candidate.response?.status;

  if (code === 'SPEECH_TIMEOUT' || status === 504) return 'timeout';
  if (code === 'INSUFFICIENT_BALANCE' || status === 402) return 'balance';
  if (code === 'INSUFFICIENT_QUOTA' || status === 429) return 'quota';
  if (code === 'SPEECH_UNAVAILABLE' || code === 'SPEECH_NOT_CONFIGURED' || status === 503) {
    return 'unavailable';
  }
  return 'failed';
}
