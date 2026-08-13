export type VoiceInputErrorKey =
  | 'cancelled'
  | 'permissionDenied'
  | 'unsupported'
  | 'noSpeech'
  | 'timeout'
  | 'balance'
  | 'quota'
  | 'unavailable'
  | 'failed';

function readString(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function readErrorCode(error: unknown): string {
  if (!error || typeof error !== 'object') return '';
  const candidate = error as {
    code?: unknown;
    businessError?: { code?: unknown };
    response?: { data?: { code?: unknown; errorCode?: unknown } };
  };
  return (
    readString(candidate.response?.data?.code) ||
    readString(candidate.response?.data?.errorCode) ||
    readString(candidate.businessError?.code) ||
    readString(candidate.code)
  );
}

export function getVoiceInputErrorKey(error: unknown): VoiceInputErrorKey {
  const name =
    error && typeof error === 'object' ? readString((error as { name?: unknown }).name) : '';
  const code = readErrorCode(error);

  if (
    name === 'AbortError' ||
    name === 'CanceledError' ||
    code === 'ERR_CANCELED' ||
    code === 'RECORDING_CANCELLED' ||
    code === 'REQUEST_CANCELLED'
  ) {
    return 'cancelled';
  }
  if (name === 'NotAllowedError' || name === 'SecurityError') return 'permissionDenied';
  if (
    code === 'MICROPHONE_UNSUPPORTED' ||
    name === 'NotFoundError' ||
    name === 'NotSupportedError'
  ) {
    return 'unsupported';
  }
  if (code === 'NO_SPEECH_DETECTED' || code === 'EMPTY_AUDIO') return 'noSpeech';
  if (code === 'TRANSCRIPTION_TIMEOUT' || code === 'ECONNABORTED' || code === 'ETIMEDOUT') {
    return 'timeout';
  }
  if (code === 'INSUFFICIENT_BALANCE') return 'balance';
  if (code === 'INSUFFICIENT_QUOTA') return 'quota';
  if (code === 'VOICE_UNAVAILABLE') return 'unavailable';
  return 'failed';
}
