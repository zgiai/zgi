import { http, webappHttp } from '@/lib/http';

interface VoiceSpeechClientError extends Error {
  code: 'INVALID_SPEECH_TARGET' | 'INVALID_SPEECH_INPUT';
}

function speechClientError(
  code: VoiceSpeechClientError['code'],
  message: string
): VoiceSpeechClientError {
  return Object.assign(new Error(message), { code });
}

function validateSpeechRequest(targetID: string, input: string) {
  const normalizedTargetID = targetID.trim();
  if (!normalizedTargetID) {
    throw speechClientError('INVALID_SPEECH_TARGET', 'A speech generation target is required.');
  }
  if (!input.trim()) {
    throw speechClientError('INVALID_SPEECH_INPUT', 'Speech input is required.');
  }
  return {
    targetID: encodeURIComponent(normalizedTargetID),
    body: { input },
  };
}

const streamConfig = (signal: AbortSignal) => ({
  adapter: 'fetch' as const,
  responseType: 'stream' as const,
  headers: { Accept: 'audio/mpeg' },
  retryAttemptsOverride: 0,
  signal,
  skipErrorHandling: true,
});

export async function generateAgentDraftSpeech(
  agentID: string,
  input: string,
  signal: AbortSignal
): Promise<ReadableStream<Uint8Array>> {
  const request = validateSpeechRequest(agentID, input);
  return http.post<ReadableStream<Uint8Array>>(
    `/console/api/agents/${request.targetID}/runtime/audio/speech`,
    request.body,
    streamConfig(signal)
  );
}

export async function generateAgentWebAppSpeech(
  webAppID: string,
  input: string,
  signal: AbortSignal
): Promise<ReadableStream<Uint8Array>> {
  const request = validateSpeechRequest(webAppID, input);
  return webappHttp.post<ReadableStream<Uint8Array>>(
    `/console/api/webapps/${request.targetID}/runtime/audio/speech`,
    request.body,
    streamConfig(signal)
  );
}
